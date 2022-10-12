package stat

import (
	"context"

	"gitlab.com/thorchain/midgard/internal/db"
)

// TODO(acsaba): This file should be renamed to withdraw.go once the terminology of all
// functions is updated

type CountAndTotal struct {
	Count                     int64
	TotalVolume               int64
	ImpermanentLossProtection int64
}

func liquidityChange(ctx context.Context,
	w db.Window, table, assetColumn, runeColumn, impLossProtColumn string) (
	ret CountAndTotal, err error) {
	buckets := db.OneIntervalBuckets(w.From, w.Until)

	withdraws, err := liquidityChangesFromTable(ctx, buckets, "*",
		table, assetColumn, runeColumn, impLossProtColumn)
	if err != nil {
		return
	}
	bucket, ok := withdraws.buckets[w.From]
	if !ok {
		// We didn't have withdraws yet, probably the beginning of chain.
		// If there are withdraws then maybe the block_pool_depths are missing for the exact
		// timestamps.
		return ret, nil
	}
	ret.Count = bucket.count
	ret.TotalVolume = bucket.runeVolume + bucket.assetVolume
	ret.ImpermanentLossProtection = bucket.impermanentLossProtection
	return
}

func WithdrawsLookup(ctx context.Context, w db.Window) (ret CountAndTotal, err error) {
	return liquidityChange(ctx, w,
		"withdraw_events", "_emit_asset_in_rune_e8", "emit_rune_e8", "imp_loss_protection_e8")
}

func StakesLookup(ctx context.Context, w db.Window) (ret CountAndTotal, err error) {
	return liquidityChange(ctx, w, "deposit_events", "_asset_in_rune_e8", "rune_e8", "")
}
