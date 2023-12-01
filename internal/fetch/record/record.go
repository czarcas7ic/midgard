package record

import (
	"strings"

	"github.com/lib/pq"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

func AddressIsRune(address string) bool {
	return (strings.HasPrefix(address, "thor") ||
		strings.HasPrefix(address, "tthor") ||
		strings.HasPrefix(address, "sthor"))
}

// Empty prevents the SQL driver from writing NULL values.
var empty = []byte{}

// Recorder gets initialised by Setup.
var Recorder = &eventRecorder{
	runningTotals: *newRunningTotals(),
}

type eventRecorder struct {
	runningTotals
}

func InsertWithMeta(table string, meta *Metadata, cols []string, values ...interface{}) error {
	cols = append(cols, "event_id", "block_timestamp")
	values = append(values, meta.EventId.AsBigint(), meta.BlockTimestamp.UnixNano())
	return db.Inserter.Insert(table, cols, values...)
}

func (*eventRecorder) OnActiveVault(e *ActiveVault, meta *Metadata) {
	cols := []string{"add_asgard_addr"}
	err := InsertWithMeta("active_vault_events", meta, cols, e.AddAsgardAddr)
	if err != nil {
		miderr.LogEventParseErrorF("ActiveVault event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnAdd(e *Add, meta *Metadata) {
	if e.Memo == nil {
		e.Memo = empty
	}

	// donate events for saver yield mint synths
	if GetCoinType(e.Pool) == AssetSynth && string(e.Memo) == "THOR-SAVERS-YIELD" {
		r.AddPoolSynthE8Depth([]byte(util.ConvertSynthPoolToNative(string(e.Pool))), e.AssetE8)

		// On a donate event with `THOR-SAVERS-YIELD` we add a row to the `rewards_event_entries` table
		// which would be normally filled only from reward events.
		// This is unlike other cases where we have 1:1 mapping between events and tables,
		// but we do this to simplify serving.
		err := InsertWithMeta("rewards_event_entries", meta,
			[]string{"pool", "rune_e8", "saver_e8"},
			util.ConvertSynthPoolToNative(string(e.Pool)), 0, e.AssetE8)
		if err != nil {
			miderr.LogEventParseErrorF(
				"synth donate event from height %d lost on %s",
				meta.BlockHeight, err)
			return
		}
	} else {
		cols := []string{
			"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "rune_e8", "pool"}
		err := InsertWithMeta("add_events", meta, cols,
			e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset, e.AssetE8, e.Memo, e.RuneE8, e.Pool)
		if err != nil {
			miderr.LogEventParseErrorF("add event from height %d lost on %s", meta.BlockHeight, err)
			return
		}
	}

	r.AddPoolAssetE8Depth(e.Pool, e.AssetE8)
	r.AddPoolRuneE8Depth(e.Pool, e.RuneE8)
}

func (r *eventRecorder) OnAsgardFundYggdrasil(e *AsgardFundYggdrasil, meta *Metadata) {
	cols := []string{"tx", "asset", "asset_e8", "vault_key"}
	err := InsertWithMeta("asgard_fund_yggdrasil_events", meta, cols,
		e.Tx, e.Asset, e.AssetE8, e.VaultKey)
	if err != nil {
		miderr.LogEventParseErrorF(
			"asgard_fund_yggdrasil event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnBond(e *Bond, meta *Metadata) {
	cols := []string{
		"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "bond_type", "e8"}
	err := InsertWithMeta("bond_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset, e.AssetE8, e.Memo, e.BondType, e.E8)
	if err != nil {
		miderr.LogEventParseErrorF("bond event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnErrata(e *Errata, meta *Metadata) {
	cols := []string{"in_tx", "asset", "asset_e8", "rune_e8"}
	err := InsertWithMeta("errata_events", meta, cols,
		e.InTx, e.Asset, e.AssetE8, e.RuneE8)
	if err != nil {
		miderr.LogEventParseErrorF("errata event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	r.AddPoolAssetE8Depth(e.Asset, e.AssetE8)
	r.AddPoolRuneE8Depth(e.Asset, e.RuneE8)
}

func (r *eventRecorder) OnFee(e *Fee, meta *Metadata) {
	cols := []string{"tx", "asset", "asset_e8", "pool_deduct"}
	err := InsertWithMeta("fee_events", meta, cols,
		e.Tx, e.Asset, e.AssetE8, e.PoolDeduct)
	if err != nil {
		miderr.LogEventParseErrorF("fee event from height %d lost on %s", meta.BlockHeight, err)
	}

	// NOTE: Fee applies to an outbound transaction amount and
	// is then sent to the reserve, so the pool is not involved in principle.
	// However, when the outbound amount is not RUNE,
	// the asset amount correspoinding to the fee is left in its pool,
	// and the RUNE equivalent is deducted from the pool's RUNE and sent to the reserve
	coinType := GetCoinType(e.Asset)
	pool := GetNativeAsset(e.Asset)
	if !IsRune(e.Asset) {
		if coinType == AssetNative {
			r.AddPoolAssetE8Depth(pool, e.AssetE8)
		}
		if coinType == AssetSynth {
			r.AddPoolSynthE8Depth(pool, -e.AssetE8)
		}
		r.AddPoolRuneE8Depth(pool, -e.PoolDeduct)
	}
}

func (r *eventRecorder) OnGas(e *Gas, meta *Metadata) {
	cols := []string{"asset", "asset_e8", "rune_e8", "tx_count"}
	err := InsertWithMeta("gas_events", meta, cols,
		e.Asset, e.AssetE8, e.RuneE8, e.TxCount)
	if err != nil {
		miderr.LogEventParseErrorF("gas event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	r.AddPoolAssetE8Depth(e.Asset, -e.AssetE8)
	r.AddPoolRuneE8Depth(e.Asset, e.RuneE8)
}

func (*eventRecorder) OnInactiveVault(e *InactiveVault, meta *Metadata) {
	cols := []string{"add_asgard_addr"}
	err := InsertWithMeta("inactive_vault_events", meta, cols, e.AddAsgardAddr)
	if err != nil {
		miderr.LogEventParseErrorF(
			"InactiveVault event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnMessage(e *Message, meta *Metadata) {
	if !config.Global.EventRecorder.OnMessageEnabled {
		return
	}
	if e.FromAddr == nil {
		e.FromAddr = empty
	}
	if e.Action == nil {
		e.Action = empty
	}
	cols := []string{"from_addr", "action"}
	err := InsertWithMeta("message_events", meta, cols, e.FromAddr, e.Action)
	if err != nil {
		miderr.LogEventParseErrorF("message event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnNewNode(e *NewNode, meta *Metadata) {
	cols := []string{"node_addr"}
	err := InsertWithMeta("new_node_events", meta, cols, e.NodeAddr)
	if err != nil {
		miderr.LogEventParseErrorF("new_node event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnOutbound(e *Outbound, meta *Metadata) {
	var internal *bool
	if string(e.Memo) == "noop" {
		internal = new(bool)
		*internal = true
	}
	cols := []string{"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "in_tx", "internal"}
	err := InsertWithMeta("outbound_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset, e.AssetE8, e.Memo, e.InTx, internal)
	if err != nil {
		miderr.LogEventParseErrorF("outbound event from height %d lost on %s", meta.BlockHeight, err)
	}
}

// TODO: add tx_type to the table
func (*eventRecorder) OnScheduledOutbound(e *ScheduledOutbound, meta *Metadata) {
	cols := []string{"chain", "to_addr", "asset", "asset_e8", "asset_decimals", "gas_rate", "memo",
		"in_hash", "out_hash", "max_gas_amount", "max_gas_decimals", "max_gas_asset",
		"module_name", "vault_pub_key"}
	err := InsertWithMeta("scheduled_outbound", meta, cols,
		e.Chain, e.ToAddr, e.Asset, e.AssetE8, e.AssetDecimals, e.GasRate, e.Memo, e.InHash,
		e.OutHash, pq.Array(e.MaxGas), pq.Array(e.MaxGasDecimal), pq.Array(e.MaxGasAsset), e.ModuleName, e.VaultPubKey)
	if err != nil {
		miderr.LogEventParseErrorF("scheduled outbound event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnPool(e *Pool, meta *Metadata) {
	cols := []string{"asset", "status"}
	err := InsertWithMeta("pool_events", meta, cols, e.Asset, e.Status)
	if err != nil {
		miderr.LogEventParseErrorF("pool event from height %d lost on %s", meta.BlockHeight, err)
	}
	if strings.ToLower(string(e.Status)) == "suspended" {
		pool := string(e.Asset)
		r.SetAssetDepth(pool, 0)
		r.SetRuneDepth(pool, 0)
		r.SetSynthDepth(pool, 0)
	}
}

func (*eventRecorder) OnRefund(e *Refund, meta *Metadata) {
	cols := []string{
		"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "asset_2nd", "asset_2nd_e8",
		"memo", "code", "reason"}
	err := InsertWithMeta("refund_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset, e.AssetE8, e.Asset2nd, e.Asset2ndE8, e.Memo,
		e.Code, e.Reason)

	if err != nil {
		miderr.LogEventParseErrorF("refund event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnReserve(e *Reserve, meta *Metadata) {
	cols := []string{
		"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "addr", "e8"}
	err := InsertWithMeta("reserve_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset, e.AssetE8, e.Memo, e.Addr, e.E8)

	if err != nil {
		miderr.LogEventParseErrorF("reserve event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnRewards(e *Rewards, meta *Metadata) {
	cols := []string{"bond_e8"}
	err := InsertWithMeta("rewards_events", meta, cols, e.BondE8)
	if err != nil {
		miderr.LogEventParseErrorF("reserve event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	if len(e.PerPool) == 0 {
		return
	}

	cols2 := []string{"pool", "rune_e8", "saver_e8"}
	for _, p := range e.PerPool {
		err := InsertWithMeta("rewards_event_entries", meta, cols2, p.Asset, p.E8, 0)
		if err != nil {
			miderr.LogEventParseErrorF(
				"reserve event pools from height %d lost on %s",
				meta.BlockHeight, err)
			return
		}
	}

	for _, a := range e.PerPool {
		r.AddPoolRuneE8Depth(a.Asset, a.E8)
	}
}

func (*eventRecorder) OnSetIPAddress(e *SetIPAddress, meta *Metadata) {
	cols := []string{"node_addr", "ip_addr"}
	err := InsertWithMeta("set_ip_address_events", meta, cols, e.NodeAddr, e.IPAddr)

	if err != nil {
		miderr.LogEventParseErrorF(
			"set_ip_address event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSetMimir(e *SetMimir, meta *Metadata) {
	cols := []string{"key", "value"}
	err := InsertWithMeta("set_mimir_events", meta, cols,
		e.Key, e.Value)
	if err != nil {
		miderr.LogEventParseErrorF(
			"set_mimir event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSetNodeKeys(e *SetNodeKeys, meta *Metadata) {
	cols := []string{"node_addr", "secp256k1", "ed25519", "validator_consensus"}
	err := InsertWithMeta("set_node_keys_events", meta, cols,
		e.NodeAddr, string(e.Secp256k1), string(e.Ed25519), e.ValidatorConsensus)

	if err != nil {
		miderr.LogEventParseErrorF(
			"set_node_keys event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSetVersion(e *SetVersion, meta *Metadata) {
	cols := []string{"node_addr", "version"}
	err := InsertWithMeta("set_version_events", meta, cols, e.NodeAddr, e.Version)
	if err != nil {
		miderr.LogEventParseErrorF("set_version event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnSlash(e *Slash, meta *Metadata) {
	if len(e.Amounts) == 0 {
		miderr.LogEventParseErrorF("slash event on pool %q ignored: zero amounts", e.Pool)
	}
	for _, a := range e.Amounts {
		cols := []string{"pool", "asset", "asset_e8"}
		err := InsertWithMeta("slash_events", meta, cols, e.Pool, a.Asset, a.E8)
		if err != nil {
			miderr.LogEventParseErrorF("slash event from height %d lost on %s", meta.BlockHeight, err)
		}
		coinType := GetCoinType(a.Asset)
		switch coinType {
		case Rune:
			r.AddPoolRuneE8Depth(e.Pool, a.E8)
		case AssetNative:
			r.AddPoolAssetE8Depth(e.Pool, a.E8)
		default:
			miderr.LogEventParseErrorF("Unhandled slash coin type: %s", a.Asset)
		}
	}
}

func (*eventRecorder) OnPendingLiquidity(e *PendingLiquidity, meta *Metadata) {
	cols := []string{
		"pool", "asset_tx", "asset_chain", "asset_addr", "asset_e8",
		"rune_tx", "rune_addr", "rune_e8",
		"pending_type"}
	err := InsertWithMeta("pending_liquidity_events", meta, cols, e.Pool,
		e.AssetTx, e.AssetChain, e.AssetAddr, e.AssetE8,
		e.RuneTx, e.RuneAddr, e.RuneE8,
		e.PendingType)

	if err != nil {
		miderr.LogEventParseErrorF(
			"pending_liquidity event from height %d lost on %s",
			meta.BlockHeight, err)
		return
	}
}

func (r *eventRecorder) OnStake(e *Stake, meta *Metadata) {
	// TODO(muninn): Separate this side data calculation from the sync process.
	aE8, rE8, _ := r.CurrentDepths(e.Pool)
	aE8 += e.AssetE8
	rE8 += e.RuneE8
	var assetInRune int64
	if aE8 != 0 {
		assetInRune = int64(float64(e.AssetE8)*(float64(rE8)/float64(aE8)) + 0.5)
	}
	cols := []string{
		"pool", "asset_tx", "asset_chain",
		"asset_addr", "asset_e8", "stake_units", "rune_tx", "rune_addr", "rune_e8",
		"_asset_in_rune_e8"}
	err := InsertWithMeta(
		"stake_events", meta, cols,
		e.Pool, e.AssetTx, e.AssetChain,
		e.AssetAddr, e.AssetE8, e.StakeUnits, e.RuneTx, e.RuneAddr, e.RuneE8,
		assetInRune)
	if err != nil {
		miderr.LogEventParseErrorF("stake event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	r.AddPoolAssetE8Depth(e.Pool, e.AssetE8)
	r.AddPoolRuneE8Depth(e.Pool, e.RuneE8)
}

func (r *eventRecorder) OnSwap(e *Swap, meta *Metadata) {
	fromCoin := GetCoinType(e.FromAsset)
	toCoin := GetCoinType(e.ToAsset)
	if fromCoin == UnknownCoin {
		miderr.LogEventParseErrorF(
			"swap event from height %d lost - unknown from Coin %s",
			meta.BlockHeight, e.FromAsset)
		return
	}
	if toCoin == UnknownCoin {
		miderr.LogEventParseErrorF(
			"swap event from height %d lost - unknown to Coin %s",
			meta.BlockHeight, e.ToAsset)
		return
	}
	isStreaming := false
	if e.StreamingQuantity > 1 || util.CheckMemoIsStreamingSwap(string(e.Memo)) {
		isStreaming = true
	}
	var direction db.SwapDirection
	switch {
	case fromCoin == Rune && toCoin == AssetNative:
		direction = db.RuneToAsset
	case fromCoin == AssetNative && toCoin == Rune:
		direction = db.AssetToRune
	case fromCoin == Rune && toCoin == AssetSynth:
		direction = db.RuneToSynth
	case fromCoin == AssetSynth && toCoin == Rune:
		direction = db.SynthToRune
	case fromCoin == Rune && toCoin == AssetDerived:
		direction = db.RuneToDerived
	case fromCoin == AssetDerived && toCoin == Rune:
		direction = db.DerivedToRune
	default:
		miderr.LogEventParseErrorF(
			"swap event from height %d lost - exactly one side should be Rune. fromCoin: %s toCoin: %s",
			meta.BlockHeight, e.FromAsset, e.ToAsset)
		return
	}
	cols := []string{"tx", "chain", "from_addr", "to_addr",
		"from_asset", "from_e8", "to_asset", "to_e8",
		"memo", "pool", "to_e8_min", "swap_slip_bp", "liq_fee_e8", "liq_fee_in_rune_e8",
		"_direction", "_streaming", "streaming_quantity", "streaming_count"}
	err := InsertWithMeta("swap_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr,
		e.FromAsset, e.FromE8, e.ToAsset, e.ToE8,
		e.Memo, e.Pool, e.ToE8Min, e.SwapSlipBP, e.LiqFeeE8, e.LiqFeeInRuneE8,
		direction, isStreaming, e.StreamingQuantity, e.StreamingCount)
	if err != nil {
		miderr.LogEventParseErrorF("swap event from height %d lost on %s", meta.BlockHeight, err)
		return
	}

	if toCoin == Rune {
		// Swap adds pool asset in exchange of RUNE.
		if fromCoin == AssetNative {
			r.AddPoolAssetE8Depth(e.Pool, e.FromE8)
		}
		// Swap burns synths in exchange of RUNE.
		if fromCoin == AssetSynth {
			r.AddPoolSynthE8Depth(e.Pool, -e.FromE8)
		}
		r.AddPoolRuneE8Depth(e.Pool, -e.ToE8)
	} else {
		// Swap adds RUNE to pool in exchange of asset.
		r.AddPoolRuneE8Depth(e.Pool, e.FromE8)
		if toCoin == AssetNative {
			r.AddPoolAssetE8Depth(e.Pool, -e.ToE8)
		}
		// Swap mints synths in exchange of RUNE.
		if toCoin == AssetSynth {
			r.AddPoolSynthE8Depth(e.Pool, e.ToE8)
		}
	}
}

func (*eventRecorder) OnTransfer(e *Transfer, meta *Metadata) {
	if !config.Global.EventRecorder.OnTransferEnabled {
		return
	}

	cols := []string{"from_addr", "to_addr", "asset", "amount_e8"}
	err := InsertWithMeta("transfer_events", meta, cols,
		e.FromAddr, e.ToAddr, e.Asset, e.AmountE8)

	if err != nil {
		miderr.LogEventParseErrorF(
			"transfer event from height %d lost on %s",
			meta.BlockHeight, err)
		return
	}
}

func (r *eventRecorder) OnWithdraw(e *Withdraw, meta *Metadata) {
	// TODO(muninn): Separate this side data calculation from the sync process.
	aE8, rE8, _ := r.CurrentDepths(e.Pool)
	var emitAssetInRune int64
	if aE8 != 0 {
		emitAssetInRune = int64(float64(e.EmitAssetE8)*(float64(rE8)/float64(aE8)) + 0.5)
	}
	// there have been events from thornode with attribute `to` as null
	// mostly in pool cycle, for pool removal. although it's fixed on the newer versions
	// this check make sure that the eailer versions will make the pools membership correct.
	if e.ToAddr == nil {
		e.ToAddr = []byte("")
	}

	cols := []string{
		"tx", "chain", "from_addr", "to_addr", "asset",
		"asset_e8", "emit_asset_e8", "emit_rune_e8",
		"memo", "pool", "stake_units", "basis_points", "asymmetry", "imp_loss_protection_e8",
		"_emit_asset_in_rune_e8"}
	err := InsertWithMeta(
		"withdraw_events", meta, cols,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.Asset,
		e.AssetE8, e.EmitAssetE8, e.EmitRuneE8,
		e.Memo, e.Pool, e.StakeUnits, e.BasisPoints, e.Asymmetry, e.ImpLossProtectionE8,
		emitAssetInRune)

	if err != nil {
		miderr.LogEventParseErrorF("withdraw event from height %d lost on %s", meta.BlockHeight, err)
	}
	// Rune/Asset withdrawn from pool
	r.AddPoolAssetE8Depth(e.Pool, -e.EmitAssetE8)
	r.AddPoolRuneE8Depth(e.Pool, -e.EmitRuneE8)

	// Rune added to pool from reserve as impermanent loss protection
	r.AddPoolRuneE8Depth(e.Pool, e.ImpLossProtectionE8)

	// Logic for withdraw changed since start of chaosnet 2021-04.
	//
	// Background: In order to initiate a withdraw the user needs to send a transaction with a memo.
	// On many asset chains one can't send 0 value, therefore they often send a small amount
	// of asset (e.g. 1e-8). The value sent in by the user is shown in withdraw.coin
	// (unstake.asset_e8)
	//
	// New logic: keeps withdraw.coin in the wallets but it doesn't increment depths with it.
	//
	// Old logic: Pools don't keep this amount, it's forwarded back to the user and included in
	// the EmitAssetE8. Therefore pool depth decreases with less then the EmitAssetE8
	// This was not applied for rune or assets different then the native asset of the chain:
	//   - for Rune there is no minimum amount, if some rune is sent it's kept as donation.
	//   - when for non chain native assets (e.g. ETH.USDT) the EmitAssetE8 could not have contained
	//     the coin sent in. After withdrawCoinPooledHeight THORNode does not add AssetE8 into
	//     the withdraw EmitAssetE8 instead it will be added to its corresponding pool
	if meta.BlockHeight < withdrawCoinKeptHeight {
		if e.AssetE8 != 0 && string(e.Pool) == string(e.Asset) {
			r.AddPoolAssetE8Depth(e.Pool, e.AssetE8)
		}
	}

	// In some cases a saver withdrawal to the LTC/LTC pool happens (saver vault) and the AssetE8
	// will be added to the LTC.LTC pool. Also, another example can be withdrawal
	// from ETH.USDC-ID (EmitAssetE8) with ETH.ETH (AssetE8) withdraw message,
	// then ETH.ETH (AssetE8) will be added to its pool.
	// Checking chain and depth here is just a sanity check.
	if meta.BlockHeight >= withdrawCoinPooledHeight && e.AssetE8 != 0 {
		if aD, rD, _ := r.CurrentDepths(e.Asset); (aD > 0 || rD > 0) &&
			string(e.Chain) == util.AssetFromString(string(e.Pool)).Chain {
			r.AddPoolAssetE8Depth(e.Asset, e.AssetE8)
		}
	}
}

func (*eventRecorder) OnUpdateNodeAccountStatus(e *UpdateNodeAccountStatus, meta *Metadata) {
	if e.Former == nil {
		e.Former = empty
	}
	cols := []string{"node_addr", "former", "current"}
	err := InsertWithMeta("update_node_account_status_events", meta, cols,
		e.NodeAddr, e.Former, e.Current)

	if err != nil {
		miderr.LogEventParseErrorF(
			"UpdateNodeAccountStatus event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnValidatorRequestLeave(e *ValidatorRequestLeave, meta *Metadata) {
	cols := []string{"tx", "from_addr", "node_addr"}
	err := InsertWithMeta("validator_request_leave_events", meta, cols,
		e.Tx, e.FromAddr, e.NodeAddr)

	if err != nil {
		miderr.LogEventParseErrorF(
			"validator_request_leave event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnPoolBalanceChange(e *PoolBalanceChange, meta *Metadata) {
	cols := []string{
		"asset", "rune_amt", "rune_add", "asset_amt", "asset_add", "reason"}
	err := InsertWithMeta("pool_balance_change_events", meta, cols,
		e.Asset, e.RuneAmt, e.RuneAdd, e.AssetAmt, e.AssetAdd, e.Reason)

	if err != nil {
		miderr.LogEventParseErrorF(
			"pool_balance_change event from height %d lost on %s",
			meta.BlockHeight, err)
	}

	assetAmount := e.AssetAmt
	if assetAmount != 0 {
		if !e.AssetAdd {
			assetAmount *= -1
		}
		r.AddPoolAssetE8Depth(e.Asset, assetAmount)
	}
	runeAmount := e.RuneAmt
	if runeAmount != 0 {
		if !e.RuneAdd {
			runeAmount *= -1
		}
		r.AddPoolRuneE8Depth(e.Asset, runeAmount)
	}
}

func (*eventRecorder) OnTHORNameChange(e *THORNameChange, meta *Metadata) {
	cols := []string{
		"name", "chain", "address", "registration_fee_e8", "fund_amount_e8", "expire", "owner"}
	err := InsertWithMeta("thorname_change_events", meta, cols,
		e.Name, e.Chain, e.Address, e.RegistrationFeeE8, e.FundAmountE8, e.ExpireHeight, e.Owner)

	if err != nil {
		miderr.LogEventParseErrorF(
			"thorname event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSwitch(e *Switch, meta *Metadata) {
	cols := []string{
		"tx", "from_addr", "to_addr", "burn_asset", "burn_e8", "mint_e8"}
	err := InsertWithMeta("switch_events", meta, cols,
		e.Tx, e.FromAddr, e.ToAddr, e.BurnAsset, e.BurnE8, e.MintE8)

	if err != nil {
		miderr.LogEventParseErrorF("switch event from height %d lost on %s", meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSlashPoints(e *SlashPoints, meta *Metadata) {
	cols := []string{"node_address", "slash_points", "reason"}
	err := InsertWithMeta("slash_points_events", meta, cols,
		e.NodeAddress, e.SlashPoints, e.Reason)
	if err != nil {
		miderr.LogEventParseErrorF(
			"slash_points event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnSetNodeMimir(e *SetNodeMimir, meta *Metadata) {
	cols := []string{"address", "key", "value"}
	err := InsertWithMeta("set_node_mimir_events", meta, cols,
		e.Address, e.Key, e.Value)
	if err != nil {
		miderr.LogEventParseErrorF(
			"set_node_mimir event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnMintBurn(e *MintBurn, meta *Metadata) {
	cols := []string{"asset", "asset_e8", "supply", "reason"}
	err := InsertWithMeta("mint_burn_events", meta, cols,
		e.Asset, e.AssetE8, e.Supply, e.Reason)
	if err != nil {
		miderr.LogEventParseErrorF(
			"mint_burn event from height %d lost on %s",
			meta.BlockHeight, err)
	}

	// Reflect synth burning when done for MintBurn reason "failed_refund" .
	if GetCoinType(e.Asset) == AssetSynth &&
		string(e.Supply) == "burn" &&
		string(e.Reason) == "failed_refund" {
		r.AddPoolSynthE8Depth([]byte(util.ConvertSynthPoolToNative(string(e.Asset))), -e.AssetE8)
	}
}

func (*eventRecorder) OnVersion(e *Version, meta *Metadata) {
	cols := []string{"version"}
	err := InsertWithMeta("network_version_events", meta, cols,
		e.Version)
	if err != nil {
		miderr.LogEventParseErrorF(
			"version event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnLoanOpen(e *LoanOpen, meta *Metadata) {
	cols := []string{"owner", "collateral_deposited", "debt_issued",
		"collateralization_ratio", "collateral_asset", "target_asset", "tx_id"}
	err := InsertWithMeta("loan_open_events", meta, cols,
		e.Owner, e.CollateralDeposited, e.DebtIssued, e.CollateralizationRatio, e.CollateralAsset, e.TargetAsset, e.TxID)
	if err != nil {
		miderr.LogEventParseErrorF(
			"loan_open event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (*eventRecorder) OnLoanRepayment(e *LoanRepayment, meta *Metadata) {
	cols := []string{"owner", "collateral_withdrawn", "debt_repaid", "collateral_asset", "tx_id"}
	err := InsertWithMeta("loan_repayment_events", meta, cols,
		e.Owner, e.CollateralWithdrawn, e.DebtRepaid, e.CollateralAsset, e.TxID)
	if err != nil {
		miderr.LogEventParseErrorF(
			"loan_repayment event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnStreamingSwapDetails(e *StreamingSwapDetails, meta *Metadata) {
	cols := []string{"tx_id", "interval", "quantity", "count", "last_height",
		"deposit_asset", "deposit_e8", "in_asset", "in_e8",
		"out_asset", "out_e8", "failed_swaps", "failed_swap_reasons"}
	err := InsertWithMeta("streaming_swap_details_events", meta, cols,
		e.TxID, e.Interval, e.Quantity, e.Count, e.LastHeight,
		e.DepositAsset, e.DepoitE8, e.InAsset, e.InE8, e.OutAsset,
		e.OutE8, pq.Array(e.FailedSwaps), pq.Array(e.FailedReasons))
	if err != nil {
		miderr.LogEventParseErrorF(
			"streaming_swap_details event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnTSSKeygenSuccess(e *TSSKeygenSuccess, meta *Metadata) {
	cols := []string{"pub_key", "members", "height"}
	err := InsertWithMeta("tss_keygen_success_events", meta, cols,
		e.PubKey, pq.Array(e.Members), e.Height)
	if err != nil {
		miderr.LogEventParseErrorF(
			"tss_keygen_success_events event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

func (r *eventRecorder) OnTSSKeygenFailure(e *TSSKeygenFailure, meta *Metadata) {
	cols := []string{"fail_reason", "is_unicast", "blame_nodes", "round", "height"}
	err := InsertWithMeta("tss_keygen_failure_events", meta, cols,
		e.Reason, e.IsUniCast, pq.Array(e.BlameNodes), e.Round, e.Height)
	if err != nil {
		miderr.LogEventParseErrorF(
			"tss_keygen_failure_events event from height %d lost on %s",
			meta.BlockHeight, err)
	}
}

// TODO: add these events to Midgard records and deprecate other synth Burn/Mint

// Mint synthetic assets from cosmos native messages
func (r *eventRecorder) OnCoinbase(e *Coinbase, meta *Metadata) {
	r.AddPoolSynthE8Depth([]byte(util.ConvertSynthPoolToNative(string(e.Asset))), e.AssetE8)
}

// Burn synthetic assets from cosmos native messages
func (r *eventRecorder) OnBurn(e *Burn, meta *Metadata) {
	r.AddPoolSynthE8Depth([]byte(util.ConvertSynthPoolToNative(string(e.Asset))), -e.AssetE8)
}
