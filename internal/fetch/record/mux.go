package record

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/pascaldekloe/metrics"

	abci "github.com/tendermint/tendermint/abci/types"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/internal/util/timer"
)

// Package Metrics
var (
	blockProcTimer = timer.NewTimer("block_write_process")
	EventProcTime  = metrics.Must1LabelHistogram("midgard_chain_event_process_seconds", "type", 0.001, 0.01, 0.1)

	EventTotal            = metrics.Must1LabelCounter("midgard_chain_events_total", "group")
	DeliverTxEventsTotal  = EventTotal("deliver_tx")
	BeginBlockEventsTotal = EventTotal("begin_block")
	EndBlockEventsTotal   = EventTotal("end_block")
	IgnoresTotal          = metrics.MustCounter("midgard_chain_event_ignores_total", "Number of known types not in use seen.")
	UnknownsTotal         = metrics.MustCounter("midgard_chain_event_unknowns_total", "Number of unknown types discarded.")

	AttrPerEvent = metrics.MustHistogram("midgard_chain_event_attrs", "Number of attributes per event.", 0, 1, 7, 21, 144)

	PoolRewardsTotal = metrics.MustCounter("midgard_pool_rewards_total", "Number of asset amounts on rewards events seen.")
)

// Metadata has metadata for a block (from the chain).
type Metadata struct {
	BlockHeight    int64
	BlockTimestamp time.Time
	EventId        db.EventId
}

// Block invokes Listener for each transaction event in block.
func ProcessBlock(block *chain.Block) {
	defer blockProcTimer.One()()

	applyBlockCorrections(block)

	m := Metadata{
		BlockHeight:    block.Height,
		BlockTimestamp: block.Time,
		EventId:        db.EventId{BlockHeight: block.Height},
	}

	// “The BeginBlock ABCI message is sent from the underlying Tendermint
	// engine when a block proposal created by the correct proposer is
	// received, before DeliverTx is run for each transaction in the block.
	// It allows developers to have logic be executed at the beginning of
	// each block.”
	// — https://docs.cosmos.network/master/core/baseapp.html#beginblock
	BeginBlockEventsTotal.Add(uint64(len(block.Results.BeginBlockEvents)))
	m.EventId.Location = db.BeginBlockEvents
	m.EventId.EventIndex = 1
	for eventIndex, event := range block.Results.BeginBlockEvents {
		if err := processEvent(event, &m); err != nil {
			miderr.LogEventParseErrorF("block height %d begin event %d type %q skipped: %s",
				block.Height, eventIndex, event.Type, err)
		}
		m.EventId.EventIndex++
	}

	m.EventId.Location = db.TxsResults
	m.EventId.TxIndex = 1
	for txIndex, tx := range block.Results.TxsResults {
		DeliverTxEventsTotal.Add(uint64(len(tx.Events)))
		m.EventId.EventIndex = 1
		for eventIndex, event := range tx.Events {
			if err := processEvent(event, &m); err != nil {
				miderr.LogEventParseErrorF("block height %d tx %d event %d type %q skipped: %s",
					block.Height, txIndex, eventIndex, event.Type, err)
			}
			m.EventId.EventIndex++
		}
		m.EventId.TxIndex++
	}

	// “The EndBlock ABCI message is sent from the underlying Tendermint
	// engine after DeliverTx as been run for each transaction in the block.
	// It allows developers to have logic be executed at the end of each
	// block.”
	// — https://docs.cosmos.network/master/core/baseapp.html#endblock
	EndBlockEventsTotal.Add(uint64(len(block.Results.EndBlockEvents)))
	m.EventId.Location = db.EndBlockEvents
	m.EventId.EventIndex = 1
	for eventIndex, event := range block.Results.EndBlockEvents {
		if err := processEvent(event, &m); err != nil {
			miderr.LogEventParseErrorF("block height %d end event %d type %q skipped: %s",
				block.Height, eventIndex, event.Type, err)
		}
		m.EventId.EventIndex++
	}

	AddMissingEvents(&m)
}

var errEventType = errors.New("unknown event type")

// Block notifies Listener for the transaction event.
// Errors do not include the event type in the message.
func processEvent(event abci.Event, meta *Metadata) error {
	defer EventProcTime(event.Type).AddSince(time.Now())

	attrs := event.Attributes
	AttrPerEvent.Add(float64(len(attrs)))

	switch event.Type {
	case "ActiveVault":
		var x ActiveVault
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnActiveVault(&x, meta)
	case "donate":
		// TODO(acsaba): rename add to donate
		var x Add
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnAdd(&x, meta)
	case "asgard_fund_yggdrasil":
		var x AsgardFundYggdrasil
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnAsgardFundYggdrasil(&x, meta)
	case "bond":
		var x Bond
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnBond(&x, meta)
	case "errata":
		var x Errata
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnErrata(&x, meta)
	case "fee":
		var x Fee
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		if CorrectionsFeeEventIsOK(&x, meta) {
			Recorder.OnFee(&x, meta)
		}
	case "InactiveVault":
		var x InactiveVault
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnInactiveVault(&x, meta)
	case "gas":
		var x Gas
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnGas(&x, meta)
	case "message":
		var x Message
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnMessage(&x, meta)
	case "new_node":
		var x NewNode
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnNewNode(&x, meta)
	case "outbound":
		var x Outbound
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnOutbound(&x, meta)
	case "pool":
		var x Pool
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPool(&x, meta)
	case "refund":
		var x Refund
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnRefund(&x, meta)
	case "reserve":
		var x Reserve
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnReserve(&x, meta)
	case "rewards":
		var x Rewards
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		PoolRewardsTotal.Add(uint64(len(x.PerPool)))
		Recorder.OnRewards(&x, meta)
	case "set_ip_address":
		var x SetIPAddress
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetIPAddress(&x, meta)
	case "set_mimir":
		var x SetMimir
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetMimir(&x, meta)
	case "set_node_keys":
		var x SetNodeKeys
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetNodeKeys(&x, meta)
	case "set_version":
		var x SetVersion
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetVersion(&x, meta)
	case "slash":
		var x Slash
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSlash(&x, meta)
	case "pending_liquidity":
		var x PendingLiquidity
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPendingLiquidity(&x, meta)
	case "add_liquidity":
		var x Stake
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnStake(&x, meta)
	case "swap":
		var x Swap
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSwap(&x, meta)
	case "transfer":
		var x Transfer
		if err := x.LoadTendermint(attrs); err != nil {
			if err.Error() == "empty amount" {
				// Ignore transfers with null amount.
				// TODO(huginn): investigate why this happens.
				return nil
			}
			return err
		}
		Recorder.OnTransfer(&x, meta)
	case "withdraw":
		var x Withdraw
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		if CorrectWithdraw(&x, meta) == Discard {
			break
		}
		Recorder.OnWithdraw(&x, meta)
	case "UpdateNodeAccountStatus":
		var x UpdateNodeAccountStatus
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnUpdateNodeAccountStatus(&x, meta)
	case "validator_request_leave":
		var x ValidatorRequestLeave
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnValidatorRequestLeave(&x, meta)
	case "pool_balance_change":
		var x PoolBalanceChange
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPoolBalanceChange(&x, meta)
	case "thorname":
		var x THORNameChange
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnTHORNameChange(&x, meta)
	case "switch":
		var x Switch
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSwitch(&x, meta)
	case "slash_points":
		var x SlashPoints
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSlashPoints(&x, meta)
	case "set_node_mimir":
		var x SetNodeMimir
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetNodeMimir(&x, meta)
	case "mint_burn":
		var x MintBurn
		if err := x.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnMintBurn(&x, meta)
	case "tx":
	case "coin_spent", "coin_received":
	case "coinbase":
	case "burn":
	case "tss_keygen", "tss_keysign":
	case "create_client", "update_client":
	case "connection_open_init":
	case "security":
	case "scheduled_outbound":
	default:
		miderr.LogEventParseErrorF("Unknown event type: %s, attributes: %s",
			event.Type, FormatAttributes(attrs))
		UnknownsTotal.Add(1)
		return errEventType
	}
	return nil
}

func FormatAttributes(attrs []abci.EventAttribute) string {
	buf := bytes.Buffer{}
	fmt.Fprint(&buf, "{")
	for _, attr := range attrs {
		fmt.Fprint(&buf, `"`, string(attr.Key), `": "`, string(attr.Value), `"`)
	}
	fmt.Fprint(&buf, "}")
	return buf.String()
}
