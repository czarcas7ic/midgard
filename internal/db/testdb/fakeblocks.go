package testdb

import (
	"fmt"
	"testing"

	abci "github.com/tendermint/tendermint/abci/types"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util"
)

type blockCreator struct {
	lastHeight    int64
	lastTimestamp db.Second
}

type FakeEvent interface {
	ToTendermint() abci.Event
}

func (bc *blockCreator) NewBlock(t *testing.T, timeStr string, events ...FakeEvent) {
	sec := db.StrToSec(timeStr)
	bc.newBlockSec(t, sec, events...)
}

func (bc *blockCreator) newBlockSec(t *testing.T, timestamp db.Second, events ...FakeEvent) {
	bc.lastHeight++
	bc.lastTimestamp = timestamp

	block := chain.Block{
		Height:  bc.lastHeight,
		Time:    timestamp.ToTime(),
		Hash:    []byte(fmt.Sprintf("hash%d", bc.lastHeight)),
		Results: &coretypes.ResultBlockResults{},
	}

	for _, event := range events {
		block.Results.EndBlockEvents = append(block.Results.EndBlockEvents, event.ToTendermint())
	}

	if block.Height == 1 {
		hash := string(block.Hash)
		db.InitializeChainVars("fakechain", 1, hash)
		db.SetAndCheckFirstBlock(hash, 1, db.TimeToNano(block.Time))
	}

	err := timeseries.ProcessBlock(&block, true)
	require.NoError(t, err)

	db.RefreshAggregatesForTests()
}

func (bc *blockCreator) EmptyBlocksBefore(t *testing.T, height int64) {
	for bc.lastHeight < height-1 {
		bc.newBlockSec(t, bc.lastTimestamp+1)
	}
}

const OMIT_FIELD = "OMIT_FIELD"

func toAttributes(attrs map[string]string) (ret []abci.EventAttribute) {
	for k, v := range attrs {
		if v == OMIT_FIELD {
			continue
		}
		var b []byte
		if v != "" {
			b = []byte(v)
		}
		ret = append(ret, abci.EventAttribute{Index: true, Key: []byte(k), Value: b})
	}
	return
}

func withDefaultStr(s string, def string) string {
	if s == "" {
		return def
	}
	return s
}

func intIfNotZero(i int64) string {
	if i != 0 {
		return util.IntStr(i)
	} else {
		return OMIT_FIELD
	}
}

type Swap struct {
	Pool               string
	Coin               string
	EmitAsset          string
	LiquidityFee       int64
	LiquidityFeeInRune int64
	Slip               int64
	FromAddress        string
	ToAddress          string
	TxID               string
	PriceTarget        int64
	Memo               string
}

func (x Swap) ToTendermint() abci.Event {

	memo := x.Memo
	if memo == "" {
		memo = "doesntmatter"
	}

	return abci.Event{Type: "swap", Attributes: toAttributes(map[string]string{
		"pool":                  x.Pool,
		"memo":                  memo,
		"coin":                  x.Coin,
		"emit_asset":            x.EmitAsset,
		"from":                  withDefaultStr(x.FromAddress, "addressfrom"),
		"to":                    withDefaultStr(x.ToAddress, "addressto"),
		"chain":                 "chain",
		"id":                    withDefaultStr(x.TxID, "txid"),
		"swap_target":           util.IntStr(x.PriceTarget),
		"swap_slip":             util.IntStr(x.Slip),
		"liquidity_fee":         util.IntStr(x.LiquidityFee),
		"liquidity_fee_in_rune": util.IntStr(x.LiquidityFeeInRune),
	})}
}

type Outbound struct {
	Chain       string
	Coin        string
	FromAddress string
	ToAddress   string
	TxID        string
	InTxID      string
	Memo        string
}

func (x Outbound) ToTendermint() abci.Event {
	return abci.Event{Type: "outbound", Attributes: toAttributes(map[string]string{
		"chain":    "chain",
		"coin":     x.Coin,
		"from":     withDefaultStr(x.FromAddress, "addressfrom"),
		"to":       withDefaultStr(x.ToAddress, "addressto"),
		"id":       withDefaultStr(x.TxID, "00000000"),
		"in_tx_id": withDefaultStr(x.InTxID, "txid"),
		"memo":     withDefaultStr(x.Memo, "memo"),
	})}
}

type AddLiquidity struct {
	Pool                   string
	AssetAmount            int64
	RuneAmount             int64
	AssetAddress           string
	RuneAddress            string
	RuneTxID               string
	AssetTxID              string
	LiquidityProviderUnits int64 // If 0 defaults to 1
}

func assetTxIdKey(pool string) string {
	chainBytes, _, _ := record.ParseAsset([]byte(pool))
	chain := string(chainBytes)
	assetIdKey := "BNB_txid"
	if chain != "" {
		assetIdKey = chain + "_txid"
	}
	return assetIdKey
}

func (x AddLiquidity) ToTendermint() abci.Event {
	assetIdKey := assetTxIdKey(x.Pool)
	units := x.LiquidityProviderUnits
	if units == 0 {
		units = 1
	}
	return abci.Event{Type: "add_liquidity", Attributes: toAttributes(map[string]string{
		"pool":                     x.Pool,
		"liquidity_provider_units": util.IntStr(units),
		"rune_address":             x.RuneAddress,
		"rune_amount":              util.IntStr(x.RuneAmount),
		"asset_amount":             util.IntStr(x.AssetAmount),
		"asset_address":            x.AssetAddress,
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
		"THOR_txid":     x.RuneTxID,
		assetIdKey:      x.AssetTxID,
		"type":          pendingTypeStr,
	})}
}

type Withdraw struct {
	Pool                   string
	Coin                   string
	EmitAsset              int64
	EmitRune               int64
	LiquidityProviderUnits int64
	ImpLossProtection      int64
	ToAddress              string
	FromAddress            string
	ID                     string
	Assymetry              string
	BasisPoints            int64
}

func (x Withdraw) ToTendermint() abci.Event {
	if x.LiquidityProviderUnits == 0 {
		x.LiquidityProviderUnits = 1
	}
	if x.BasisPoints == 0 {
		x.BasisPoints = 1
	}
	return abci.Event{Type: "withdraw", Attributes: toAttributes(map[string]string{
		"pool":                     x.Pool,
		"coin":                     withDefaultStr(x.Coin, "0 THOR.RUNE"),
		"liquidity_provider_units": util.IntStr(x.LiquidityProviderUnits),
		"basis_points":             util.IntStr(x.BasisPoints),
		"asymmetry":                withDefaultStr(x.Assymetry, "0.000000000000000000"),
		"emit_rune":                util.IntStr(x.EmitRune),
		"emit_asset":               util.IntStr(x.EmitAsset),
		"imp_loss_protection":      util.IntStr(x.ImpLossProtection),
		"id":                       withDefaultStr(x.ID, "id"),
		"chain":                    "THOR",
		"from":                     withDefaultStr(x.FromAddress, "fromaddr"),
		"to":                       withDefaultStr(x.ToAddress, "toaddr"),
		"memo":                     "MEMO",
	})}
}

type Switch struct {
	FromAddress string
	ToAddress   string
	Burn        string
	Mint        int64 // Omitted if 0
	TxID        string
}

func (x Switch) ToTendermint() abci.Event {
	attributes := map[string]string{
		"from": withDefaultStr(x.FromAddress, "addressfrom"),
		"to":   withDefaultStr(x.ToAddress, "addressto"),
		"burn": x.Burn,
		"mint": intIfNotZero(x.Mint),
	}
	if x.TxID != "" {
		attributes["txid"] = x.TxID
	}
	return abci.Event{Type: "switch", Attributes: toAttributes(attributes)}
}

type StatusName string

const (
	StatusAvailable StatusName = "available"
	StatusSuspended StatusName = "suspended"
	StatusStaged    StatusName = "staged"
)

type PoolStatus struct {
	Pool   string
	Status StatusName
}

func (x PoolStatus) ToTendermint() abci.Event {
	return abci.Event{Type: "pool", Attributes: toAttributes(map[string]string{
		"pool":        x.Pool,
		"pool_status": string(x.Status),
	})}
}

func PoolActivate(pool string) PoolStatus {
	return PoolStatus{Pool: pool, Status: StatusAvailable}
}

type THORName struct {
	Name            string
	Chain           string
	Address         string
	RegistrationFee int64
	FundAmount      int64
	ExpireHeight    int64
	Owner           string
}

func (x THORName) ToTendermint() abci.Event {
	return abci.Event{Type: "thorname", Attributes: toAttributes(map[string]string{
		"name":             x.Name,
		"chain":            x.Chain,
		"address":          x.Address,
		"registration_fee": util.IntStr(x.RegistrationFee),
		"fund_amount":      util.IntStr(x.FundAmount),
		"expire":           util.IntStr(x.ExpireHeight),
		"owner":            x.Owner,
	})}
}

type SetMimir struct {
	Key   string
	Value int64
}

func (x SetMimir) ToTendermint() abci.Event {
	return abci.Event{Type: "set_mimir", Attributes: toAttributes(map[string]string{
		"key":   x.Key,
		"value": util.IntStr(x.Value),
	})}
}

type ActiveVault struct {
	AddVault string
}

func (x ActiveVault) ToTendermint() abci.Event {
	return abci.Event{Type: "ActiveVault", Attributes: toAttributes(map[string]string{
		"add new asgard vault": x.AddVault,
	})}
}

type Fee struct {
	TxID       string
	Coins      string
	PoolDeduct int64
}

func (x Fee) ToTendermint() abci.Event {
	return abci.Event{Type: "fee", Attributes: toAttributes(map[string]string{
		"tx_id":       withDefaultStr(x.TxID, "txid"),
		"coins":       x.Coins,
		"pool_deduct": util.IntStr(x.PoolDeduct),
	})}
}

type Donate struct {
	Chain       string
	Coin        string
	FromAddress string
	ToAddress   string
	TxID        string
	Memo        string
	Pool        string
}

func (x Donate) ToTendermint() abci.Event {
	return abci.Event{Type: "donate", Attributes: toAttributes(map[string]string{
		"chain": "chain",
		"coin":  x.Coin,
		"from":  withDefaultStr(x.FromAddress, "addressfrom"),
		"to":    withDefaultStr(x.ToAddress, "addressto"),
		"id":    withDefaultStr(x.TxID, "00000000"),
		"memo":  withDefaultStr(x.Memo, "memo"),
		"pool":  x.Pool,
	})}
}

type Refund struct {
	TxID        string
	Chain       string
	Coin        string
	FromAddress string
	ToAddress   string
	Reason      string
	Memo        string
}

func (x Refund) ToTendermint() abci.Event {
	return abci.Event{Type: "refund", Attributes: toAttributes(map[string]string{
		"chain":  "chain",
		"coin":   x.Coin,
		"from":   withDefaultStr(x.FromAddress, "addressfrom"),
		"to":     withDefaultStr(x.ToAddress, "addressto"),
		"id":     withDefaultStr(x.TxID, "00000000"),
		"reason": withDefaultStr(x.Reason, "reason"),
		"memo":   withDefaultStr(x.Memo, "memo"),
	})}
}

type Transfer struct {
	FromAddr    string
	ToAddr      string
	AssetAmount string
}

func (x Transfer) ToTendermint() abci.Event {
	return abci.Event{Type: "transfer", Attributes: toAttributes(map[string]string{
		"sender":    x.FromAddr,
		"recipient": x.ToAddr,
		"amount":    x.AssetAmount,
	})}
}

type Version struct {
	Version string
}

func (x Version) ToTendermint() abci.Event {
	return abci.Event{Type: "version", Attributes: toAttributes(map[string]string{
		"version": x.Version,
	})}
}
