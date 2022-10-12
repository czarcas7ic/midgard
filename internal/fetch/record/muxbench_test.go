package record_test

import (
	"testing"

	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
)

type FakeDemux struct {
	reuse struct {
		record.ActiveVault
		record.Add
		record.PendingLiquidity
		record.AsgardFundYggdrasil
		record.Bond
		record.Errata
		record.Fee
		record.Gas
		record.InactiveVault
		record.Message
		record.NewNode
		record.Outbound
		record.Pool
		record.Refund
		record.Reserve
		record.Rewards
		record.SetIPAddress
		record.SetMimir
		record.SetNodeKeys
		record.SetVersion
		record.Slash
		record.Desposit
		record.Swap
		record.Transfer
		record.Withdraw
		record.UpdateNodeAccountStatus
		record.ValidatorRequestLeave
		record.PoolBalanceChange
		record.Switch
		record.THORNameChange
		record.SlashPoints
		record.SetNodeMimir
	}
}

var GlobalFakeDemux FakeDemux

func (d *FakeDemux) processDemux(event abci.Event) int64 {
	attrs := event.Attributes

	switch event.Type {
	case "swap":
		if err := d.reuse.Swap.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return d.reuse.Swap.LiqFeeInRuneE8
	case "switch":
		if err := d.reuse.Switch.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return d.reuse.Switch.BurnE8
	case "transfer":
		if err := d.reuse.Transfer.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return d.reuse.Transfer.AmountE8
	default:
		panic("unknown event type")
	}
}

// Note: this presents a worse picture than it should, because without the
// Demux.reuse the `LoadTendermint` functions would not need to clear the structures they are
// filling in.
func processDirect(event abci.Event) int64 {
	attrs := event.Attributes

	switch event.Type {
	case "swap":
		var x record.Swap
		if err := x.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return x.LiqFeeInRuneE8
	case "switch":
		var x record.Switch
		if err := x.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return x.BurnE8
	case "transfer":
		var x record.Transfer
		if err := x.LoadTendermint(attrs); err != nil {
			panic(err)
		}
		return x.AmountE8
	default:
		panic("unknown event type")
	}
}

var total int64

var events = []abci.Event{
	testdb.Swap{
		Pool:               "BTC.BTC",
		Coin:               "1 BTC.BTC",
		EmitAsset:          "9 THOR.RUNE",
		LiquidityFeeInRune: 1,
		LiquidityFee:       1,
		Slip:               10,
	}.ToTendermint(),
	testdb.Switch{
		FromAddress: "b2",
		ToAddress:   "thor2",
		Burn:        "42 BNB.RUNE-B1A",
	}.ToTendermint(),
	testdb.Transfer{
		FromAddr:    "thorAddr2",
		ToAddr:      "thorAddr1",
		AssetAmount: "1 THOR.RUNE",
	}.ToTendermint(),
}

func BenchmarkLoadDemux(b *testing.B) {
	d := &GlobalFakeDemux

	for i := 0; i < b.N; i++ {
		for _, event := range events {
			total += d.processDemux(event)
		}
	}
}

func BenchmarkLoadDirect(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, event := range events {
			total += processDirect(event)
		}
	}
}
