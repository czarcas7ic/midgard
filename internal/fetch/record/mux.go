package record

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	abci "github.com/tendermint/tendermint/abci/types"

	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

// Package Metrics
var (
	blockProcHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "midgard",
		Subsystem: "record",
		Name:      "block_process_duration",
		Help:      "block processing duration",
	})

	eventProcHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "midgard",
			Subsystem: "record",
			Name:      "event_process_duration",
			Help:      "event processing duration",
		},
		[]string{"group", "type"},
	)

	eventUnknownCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "midgard",
		Subsystem: "record",
		Name:      "event_unknown_count",
		Help:      "count of unknown events",
	})

	eventAttrCountHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "midgard",
		Subsystem: "record",
		Name:      "event_attr_count",
		Help:      "count of attributes per event",
	})

	poolRewardsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "midgard",
		Subsystem: "record",
		Name:      "pool_rewards_total",
		Help:      "number of asset amounts on rewards events seen",
	})
)

func init() {
	prometheus.MustRegister(blockProcHistogram)
	prometheus.MustRegister(eventProcHistogram)
	prometheus.MustRegister(eventUnknownCount)
	prometheus.MustRegister(eventAttrCountHistogram)
	prometheus.MustRegister(poolRewardsTotal)
}

// Metadata has metadata for a block (from the chain).
type Metadata struct {
	BlockHeight    int64     // Tendermint sequence identifier
	BlockTimestamp time.Time // official acceptance moment
}

// Demux is a demultiplexer for events from the blockchain.
type Demux struct {
	// TODO(huginn): this should be insignificant. Benchmark and remove!
	// prevent memory allocation
	reuse struct {
		ActiveVault
		Add
		PendingLiquidity
		AsgardFundYggdrasil
		Bond
		Errata
		Fee
		Gas
		InactiveVault
		Message
		NewNode
		Outbound
		Pool
		Refund
		Reserve
		Rewards
		SetIPAddress
		SetMimir
		SetNodeKeys
		SetVersion
		Slash
		Stake
		Swap
		Transfer
		Unstake
		UpdateNodeAccountStatus
		ValidatorRequestLeave
		PoolBalanceChange
		Switch
		THORNameChange
		SlashPoints
		SetNodeMimir
	}
}

var GlobalDemux Demux

// Block invokes Listener for each transaction event in block.
func (d *Demux) Block(block *chain.Block) {
	timer := prometheus.NewTimer(blockProcHistogram)
	defer timer.ObserveDuration()

	applyBlockCorrections(block)

	m := Metadata{
		BlockHeight:    block.Height,
		BlockTimestamp: block.Time,
	}

	// “The BeginBlock ABCI message is sent from the underlying Tendermint
	// engine when a block proposal created by the correct proposer is
	// received, before DeliverTx is run for each transaction in the block.
	// It allows developers to have logic be executed at the beginning of
	// each block.”
	// — https://docs.cosmos.network/master/core/baseapp.html#beginblock
	for eventIndex, event := range block.Results.BeginBlockEvents {
		if err := d.event("begin_block", event, &m); err != nil {
			miderr.LogEventParseErrorF("block height %d begin event %d type %q skipped: %s",
				block.Height, eventIndex, event.Type, err)
		}
	}

	for txIndex, tx := range block.Results.TxsResults {
		for eventIndex, event := range tx.Events {
			if err := d.event("deliver_tx", event, &m); err != nil {
				miderr.LogEventParseErrorF("block height %d tx %d event %d type %q skipped: %s",
					block.Height, txIndex, eventIndex, event.Type, err)
			}
		}
	}

	// “The EndBlock ABCI message is sent from the underlying Tendermint
	// engine after DeliverTx as been run for each transaction in the block.
	// It allows developers to have logic be executed at the end of each
	// block.”
	// — https://docs.cosmos.network/master/core/baseapp.html#endblock
	for eventIndex, event := range block.Results.EndBlockEvents {
		if err := d.event("end_block", event, &m); err != nil {
			miderr.LogEventParseErrorF("block height %d end event %d type %q skipped: %s",
				block.Height, eventIndex, event.Type, err)
		}
	}

	AddMissingEvents(d, &m)
}

var errEventType = errors.New("unknown event type")

// Block notifies Listener for the transaction event.
// Errors do not include the event type in the message.
func (d *Demux) event(group string, event abci.Event, meta *Metadata) error {
	timer := prometheus.NewTimer(eventProcHistogram.WithLabelValues(group, event.Type))
	defer timer.ObserveDuration()

	attrs := event.Attributes
	eventAttrCountHistogram.Observe(float64(len(attrs)))

	switch event.Type {
	case "ActiveVault":
		if err := d.reuse.ActiveVault.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnActiveVault(&d.reuse.ActiveVault, meta)
	case "donate":
		// TODO(acsaba): rename add to donate
		if err := d.reuse.Add.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnAdd(&d.reuse.Add, meta)
	case "asgard_fund_yggdrasil":
		if err := d.reuse.AsgardFundYggdrasil.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnAsgardFundYggdrasil(&d.reuse.AsgardFundYggdrasil, meta)
	case "bond":
		if err := d.reuse.Bond.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnBond(&d.reuse.Bond, meta)
	case "errata":
		if err := d.reuse.Errata.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnErrata(&d.reuse.Errata, meta)
	case "fee":
		if err := d.reuse.Fee.LoadTendermint(attrs); err != nil {
			return err
		}
		if CorrectionsFeeEventIsOK(&d.reuse.Fee, meta) {
			Recorder.OnFee(&d.reuse.Fee, meta)
		}
	case "InactiveVault":
		if err := d.reuse.InactiveVault.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnInactiveVault(&d.reuse.InactiveVault, meta)
	case "gas":
		if err := d.reuse.Gas.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnGas(&d.reuse.Gas, meta)
	case "message":
		if err := d.reuse.Message.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnMessage(&d.reuse.Message, meta)
	case "new_node":
		if err := d.reuse.NewNode.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnNewNode(&d.reuse.NewNode, meta)
	case "outbound":
		if err := d.reuse.Outbound.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnOutbound(&d.reuse.Outbound, meta)
	case "pool":
		if err := d.reuse.Pool.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPool(&d.reuse.Pool, meta)
	case "refund":
		if err := d.reuse.Refund.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnRefund(&d.reuse.Refund, meta)
	case "reserve":
		if err := d.reuse.Reserve.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnReserve(&d.reuse.Reserve, meta)
	case "rewards":
		if err := d.reuse.Rewards.LoadTendermint(attrs); err != nil {
			return err
		}
		poolRewardsTotal.Add(float64(len(d.reuse.Rewards.PerPool)))
		Recorder.OnRewards(&d.reuse.Rewards, meta)
	case "set_ip_address":
		if err := d.reuse.SetIPAddress.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetIPAddress(&d.reuse.SetIPAddress, meta)
	case "set_mimir":
		if err := d.reuse.SetMimir.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetMimir(&d.reuse.SetMimir, meta)
	case "set_node_keys":
		if err := d.reuse.SetNodeKeys.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetNodeKeys(&d.reuse.SetNodeKeys, meta)
	case "set_version":
		if err := d.reuse.SetVersion.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetVersion(&d.reuse.SetVersion, meta)
	case "slash":
		if err := d.reuse.Slash.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSlash(&d.reuse.Slash, meta)
	case "pending_liquidity":
		if err := d.reuse.PendingLiquidity.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPendingLiquidity(&d.reuse.PendingLiquidity, meta)
	case "add_liquidity":
		if err := d.reuse.Stake.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnStake(&d.reuse.Stake, meta)
	case "swap":
		if err := d.reuse.Swap.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSwap(&d.reuse.Swap, meta)
	case "transfer":
		if err := d.reuse.Transfer.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnTransfer(&d.reuse.Transfer, meta)
	case "withdraw":
		// TODO(acsaba): rename unstake->withdraw.
		if err := d.reuse.Unstake.LoadTendermint(attrs); err != nil {
			return err
		}
		if CorrectWithdaw(&d.reuse.Unstake, meta) == Discard {
			break
		}
		Recorder.OnUnstake(&d.reuse.Unstake, meta)
	case "UpdateNodeAccountStatus":
		if err := d.reuse.UpdateNodeAccountStatus.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnUpdateNodeAccountStatus(&d.reuse.UpdateNodeAccountStatus, meta)
	case "validator_request_leave":
		if err := d.reuse.ValidatorRequestLeave.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnValidatorRequestLeave(&d.reuse.ValidatorRequestLeave, meta)
	case "pool_balance_change":
		if err := d.reuse.PoolBalanceChange.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnPoolBalanceChange(&d.reuse.PoolBalanceChange, meta)
	case "thorname":
		if err := d.reuse.THORNameChange.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnTHORNameChange(&d.reuse.THORNameChange, meta)
	case "switch":
		if err := d.reuse.Switch.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSwitch(&d.reuse.Switch, meta)
	case "slash_points":
		if err := d.reuse.SlashPoints.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSlashPoints(&d.reuse.SlashPoints, meta)
	case "set_node_mimir":
		if err := d.reuse.SetNodeMimir.LoadTendermint(attrs); err != nil {
			return err
		}
		Recorder.OnSetNodeMimir(&d.reuse.SetNodeMimir, meta)
	case "tx":
		//transfer event has it
	case "coin_spent":
		//transfer event has it
	case "coin_received":
		//transfer event has it
	case "coinbase":
		//transfer event has it
	case "burn":
		//transfer event has it
	case "tss_keygen", "tss_keysign", "create_client", "update_client", "connection_open_init":
		// TODO(acsaba): decide if we want to store these events.
	default:
		miderr.LogEventParseErrorF("Unkown event type: %s, attributes: %s",
			event.Type, FormatAttributes(attrs))
		eventUnknownCount.Add(1)
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
