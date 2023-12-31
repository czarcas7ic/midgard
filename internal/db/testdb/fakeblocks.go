package testdb

import (
	"fmt"
	"testing"

	abci "github.com/tendermint/tendermint/abci/types"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/fetch/chain"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util"
)

type blockCreator struct {
	lastHeight int64
	demux      record.Demux
}

type FakeEvent interface {
	ToTendermint() abci.Event
}

func (bc *blockCreator) NewBlock(t *testing.T, timeStr string, events ...FakeEvent) {
	bc.lastHeight++

	block := chain.Block{
		Height:  bc.lastHeight,
		Time:    StrToSec(timeStr).ToTime(),
		Hash:    []byte(fmt.Sprintf("hash%d", bc.lastHeight)),
		Results: &coretypes.ResultBlockResults{}}

	for _, event := range events {
		block.Results.EndBlockEvents = append(block.Results.EndBlockEvents, event.ToTendermint())
	}

	bc.demux.Block(block)
	err := timeseries.CommitBlock(block.Height, block.Time, block.Hash)
	require.NoError(t, err)
}

func toAttributes(attrs map[string]string) (ret []abci.EventAttribute) {
	for k, v := range attrs {
		ret = append(ret, abci.EventAttribute{Index: true, Key: []byte(k), Value: []byte(v)})
	}
	return
}

func withDefaultStr(s string, def string) string {
	if s == "" {
		return def
	}
	return s
}

type Swap struct {
	Pool         string
	Coin         string
	EmitAsset    string
	LiquidityFee int64
	Slip         int64
	FromAddress  string
	ToAddress    string
	TxID         string
	PriceTarget  int64
}

func (x Swap) ToTendermint() abci.Event {
	return abci.Event{Type: "swap", Attributes: toAttributes(map[string]string{
		"pool":                  x.Pool,
		"memo":                  "doesntmatter",
		"coin":                  x.Coin,
		"emit_asset":            x.EmitAsset,
		"from":                  withDefaultStr(x.FromAddress, "addressfrom"),
		"to":                    withDefaultStr(x.ToAddress, "addressto"),
		"chain":                 "chain",
		"id":                    withDefaultStr(x.TxID, "txid"),
		"swap_target":           util.IntStr(x.PriceTarget),
		"swap_slip":             util.IntStr(x.Slip),
		"liquidity_fee":         "1",
		"liquidity_fee_in_rune": util.IntStr(x.LiquidityFee),
	})}
}

type AddLiquidity struct {
	Pool         string
	AssetAmount  int64
	RuneAmount   int64
	AssetAddress string
	RuneAddress  string
	RuneTxID     string
	AssetTxID    string
}

func assetTxIdKey(pool string) string {
	chainBytes, _, _ := record.ParseAsset([]byte(pool))
	chain := string(chainBytes)
	assetIdKey := "BNB_txid"
	if chain == "" {
		assetIdKey = chain + "_txid"
	}
	return assetIdKey
}

func (x AddLiquidity) ToTendermint() abci.Event {
	assetIdKey := assetTxIdKey(x.Pool)
	return abci.Event{Type: "add_liquidity", Attributes: toAttributes(map[string]string{
		"pool":                     x.Pool,
		"liquidity_provider_units": "1",
		"rune_address":             withDefaultStr(x.RuneAddress, "runeAddress"),
		"rune_amount":              util.IntStr(x.RuneAmount),
		"asset_amount":             util.IntStr(x.AssetAmount),
		"asset_address":            withDefaultStr(x.AssetAddress, "assetAddress"),
		"THOR_txid":                withDefaultStr(x.RuneTxID, "chainID"),
		assetIdKey:                 withDefaultStr(x.AssetTxID, "chainID"),
	})}
}

type PendingTypeEnum int

const (
	PendingAdd PendingTypeEnum = iota
	PendingWithdraw
)

// Note that this intentionally doesn't have a base class together with AddLiquidity.
// Unfortunately initializing fields of embedded structs is cumbersome, it would make writing the
// unit tests harder.
type PendingLiquidity struct {
	Pool         string
	AssetAmount  int64
	RuneAmount   int64
	AssetAddress string
	RuneAddress  string
	RuneTxID     string
	AssetTxID    string
	PendingType  PendingTypeEnum
}

func (x PendingLiquidity) ToTendermint() abci.Event {
	assetIdKey := assetTxIdKey(x.Pool)
	pendingTypeStr := "unkown"
	switch x.PendingType {
	case PendingAdd:
		pendingTypeStr = "add"
	case PendingWithdraw:
		pendingTypeStr = "withdraw"
	}
	return abci.Event{Type: "pending_liquidity", Attributes: toAttributes(map[string]string{
		"pool":          x.Pool,
		"rune_address":  withDefaultStr(x.RuneAddress, "runeAddress"),
		"rune_amount":   util.IntStr(x.RuneAmount),
		"asset_amount":  util.IntStr(x.AssetAmount),
		"asset_address": withDefaultStr(x.AssetAddress, "assetAddress"),
		"THOR_txid":     withDefaultStr(x.RuneTxID, "chainID"),
		assetIdKey:      withDefaultStr(x.AssetTxID, "chainID"),
		"type":          pendingTypeStr,
	})}
}

type Withdraw struct {
	Pool              string
	Coin              string
	EmitAsset         int64
	EmitRune          int64
	ImpLossProtection int64
	ToAddress         string
}

func (x Withdraw) ToTendermint() abci.Event {
	return abci.Event{Type: "withdraw", Attributes: toAttributes(map[string]string{
		"pool":                     x.Pool,
		"coin":                     withDefaultStr(x.Coin, "0 THOR.RUNE"),
		"liquidity_provider_units": "1",
		"basis_points":             "1",
		"asymmetry":                "0.000000000000000000",
		"emit_rune":                util.IntStr(x.EmitRune),
		"emit_asset":               util.IntStr(x.EmitAsset),
		"imp_loss_protection":      util.IntStr(x.ImpLossProtection),
		"id":                       "id",
		"chain":                    "THOR",
		"from":                     "fromaddr",
		"to":                       withDefaultStr(x.ToAddress, "toaddr"),
		"memo":                     "MEMO",
	})}
}

type Switch struct {
	FromAddress string
	ToAddress   string
	Burn        string
}

func (x Switch) ToTendermint() abci.Event {
	return abci.Event{Type: "switch", Attributes: toAttributes(map[string]string{
		"from": withDefaultStr(x.FromAddress, "addressfrom"),
		"to":   withDefaultStr(x.ToAddress, "addressto"),
		"burn": x.Burn,
	})}
}

type PoolActivate struct {
	Pool string
}

func (x PoolActivate) ToTendermint() abci.Event {
	return abci.Event{Type: "pool", Attributes: toAttributes(map[string]string{
		"pool":        x.Pool,
		"pool_status": "Available",
	})}
}

type Fee struct {
	TxID       string
	Asset      string
	AssetE8    int64
	PoolDeduct int64
}

func (x Fee) ToTendermint() abci.Event {
	return abci.Event{Type: "swap", Attributes: toAttributes(map[string]string{
		"tx_id":       x.TxID,
		"coins":       toCoin(x.Asset, x.AssetE8),
		"pool_deduct": fmt.Sprintf("%d", x.PoolDeduct),
	})}
}

func toCoin(asset string, assetE8 int64) string {
	return fmt.Sprintf("%d", assetE8) + " " + asset
}
