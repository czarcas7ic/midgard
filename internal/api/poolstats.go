package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

// statsForPools is used both for stats and stats/legacy.
// The creates PoolStatsResponse, but to help the creation of PoolLegacyResponse it also
// puts int values into this struct. This way we don't need to convert back and forth between
// strings and ints.
type extraStats struct {
	now           db.Second
	runeDepth     int64
	toAssetCount  int64
	toRuneCount   int64
	swapCount     int64
	toAssetVolume int64
	toRuneVolume  int64
	totalVolume   int64
	toAssetFees   int64
	toRuneFees    int64
	totalFees     int64
}

func setAggregatesStats(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse, extra *extraStats) (merr miderr.Err) {
	state := timeseries.Latest.GetState()

	poolInfo := state.PoolInfo(pool)
	if poolInfo == nil || !poolInfo.ExistsNow() {
		merr = miderr.BadRequestF("Unknown pool: %s", pool)
		return
	}

	poolUnitsMap, err := stat.CurrentPoolsLiquidityUnits(ctx, []string{pool})
	if err != nil {
		return miderr.InternalErrE(err)
	}

	poolAPY, err := timeseries.GetSinglePoolAPY(
		ctx, poolInfo.RuneDepth, pool, buckets.Window())
	if err != nil {
		return miderr.InternalErrE(err)
	}

	status, err := timeseries.PoolStatus(ctx, pool)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}

	poolUnits := poolUnitsMap[pool]
	price := poolInfo.AssetPrice()
	priceUSD := price * stat.RunePriceUSD()

	ret.Asset = pool
	ret.AssetDepth = util.IntStr(poolInfo.AssetDepth)
	ret.RuneDepth = util.IntStr(poolInfo.RuneDepth)
	ret.PoolAPY = floatStr(poolAPY)
	ret.AssetPrice = floatStr(price)
	ret.AssetPriceUSD = floatStr(priceUSD)
	ret.Status = status
	ret.Units = util.IntStr(poolUnits)

	extra.runeDepth = poolInfo.RuneDepth
	extra.now = state.Timestamp.ToSecond()
	return
}

func setSwapStats(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse, extra *extraStats) (merr miderr.Err) {
	allSwaps, err := stat.GetPoolSwaps(ctx, &pool, buckets)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}
	if len(allSwaps) != 1 {
		merr = miderr.InternalErr("Internal error: wrong time interval.")
		return
	}
	var swapHistory stat.SwapBucket = allSwaps[0]

	ret.ToRuneVolume = util.IntStr(swapHistory.ToRuneVolume)
	ret.ToAssetVolume = util.IntStr(swapHistory.ToAssetVolume)
	ret.SwapVolume = util.IntStr(swapHistory.TotalVolume)

	ret.ToRuneCount = util.IntStr(swapHistory.ToRuneCount)
	ret.ToAssetCount = util.IntStr(swapHistory.ToAssetCount)
	ret.SwapCount = util.IntStr(swapHistory.TotalCount)

	ret.ToAssetAverageSlip = ratioStr(swapHistory.ToAssetSlip, swapHistory.ToAssetCount)
	ret.ToRuneAverageSlip = ratioStr(swapHistory.ToRuneSlip, swapHistory.ToRuneCount)
	ret.AverageSlip = ratioStr(swapHistory.TotalSlip, swapHistory.TotalCount)

	ret.ToAssetFees = util.IntStr(swapHistory.ToAssetFees)
	ret.ToRuneFees = util.IntStr(swapHistory.ToRuneFees)
	ret.TotalFees = util.IntStr(swapHistory.TotalFees)

	extra.toAssetCount = swapHistory.ToAssetCount
	extra.toRuneCount = swapHistory.ToRuneCount
	extra.swapCount = swapHistory.TotalCount
	extra.toAssetVolume = swapHistory.ToAssetVolume
	extra.toRuneVolume = swapHistory.ToRuneVolume
	extra.totalVolume = swapHistory.TotalVolume
	extra.toAssetFees = swapHistory.ToAssetFees
	extra.toRuneFees = swapHistory.ToRuneFees
	extra.totalFees = swapHistory.TotalFees
	return
}

func setLiquidityStats(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse, extra *extraStats) (merr miderr.Err) {
	var allLiquidity oapigen.LiquidityHistoryResponse
	allLiquidity, err := stat.GetLiquidityHistory(ctx, buckets, pool)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}
	ret.AddAssetLiquidityVolume = allLiquidity.Meta.AddAssetLiquidityVolume
	ret.AddRuneLiquidityVolume = allLiquidity.Meta.AddRuneLiquidityVolume
	ret.AddLiquidityVolume = allLiquidity.Meta.AddLiquidityVolume
	ret.AddLiquidityCount = allLiquidity.Meta.AddLiquidityCount
	ret.WithdrawAssetVolume = allLiquidity.Meta.WithdrawAssetVolume
	ret.WithdrawRuneVolume = allLiquidity.Meta.WithdrawRuneVolume
	ret.ImpermanentLossProtectionPaid = allLiquidity.Meta.ImpermanentLossProtectionPaid
	ret.WithdrawVolume = allLiquidity.Meta.WithdrawVolume
	ret.WithdrawCount = allLiquidity.Meta.WithdrawCount
	return
}

func setUniqueCounts(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse, extra *extraStats) (merr miderr.Err) {
	swapperCount, err := stat.GetUniqueSwapperCount(
		ctx, pool, buckets.Window())
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}
	ret.UniqueSwapperCount = util.IntStr(swapperCount)

	members, err := timeseries.GetMemberAddrs(ctx, &pool)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}
	ret.UniqueMemberCount = strconv.Itoa(len(members))
	return
}

func statsForPool(ctx context.Context, pool string, buckets db.Buckets) (
	ret oapigen.PoolStatsResponse, extra extraStats, merr miderr.Err) {
	merr = setAggregatesStats(ctx, pool, buckets, &ret, &extra)
	if merr != nil {
		return
	}

	merr = setSwapStats(ctx, pool, buckets, &ret, &extra)
	if merr != nil {
		return
	}

	merr = setLiquidityStats(ctx, pool, buckets, &ret, &extra)
	if merr != nil {
		return
	}

	merr = setUniqueCounts(ctx, pool, buckets, &ret, &extra)
	if merr != nil {
		return
	}

	return
}

func jsonPoolStats(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	pool := ps[0].Value

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	var buckets db.Buckets
	now := db.NowSecond()
	switch period {
	case "1h":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 60*60, now}}
	case "24h":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 24*60*60, now}}
	case "7d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 7*24*60*60, now}}
	case "30d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 30*24*60*60, now}}
	case "90d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 90*24*60*60, now}}
	case "180d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 180*24*60*60, now}}
	case "365d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 365*24*60*60, now}}
	case "all":
		buckets = db.AllHistoryBuckets()
	default:
		miderr.BadRequestF(
			"Parameter period parameter(%s). Accepted values:  1h, 24h, 7d, 30d, 90d, 365d, all",
			period).ReportHTTP(w)
		return
	}
	result, _, merr := statsForPool(r.Context(), pool, buckets)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}
	respJSON(w, result)
}

func jsonPoolStatsLegacy(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	pool := ps[0].Value
	stats, extra, merr := statsForPool(r.Context(), pool, db.AllHistoryBuckets())
	if merr != nil {
		merr.ReportHTTP(w)
	}

	now := extra.now
	dayAgo := now - 24*60*60
	dailyVolumes, err := stat.PoolsTotalVolume(r.Context(), []string{pool}, dayAgo.ToNano(), now.ToNano())
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
	}
	dailyVolume := dailyVolumes[pool]

	week := db.Window{From: now - 7*24*60*60, Until: now}
	poolAPY, err := timeseries.GetSinglePoolAPY(r.Context(), extra.runeDepth, pool, week)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
	}

	addLiquidityCount, _ := strconv.ParseInt(stats.AddLiquidityCount, 10, 64)
	withdrawCount, _ := strconv.ParseInt(stats.WithdrawCount, 10, 64)

	result := oapigen.PoolLegacyResponse{
		Asset:            stats.Asset,
		Status:           stats.Status,
		Price:            stats.AssetPrice,
		AssetDepth:       stats.AssetDepth,
		RuneDepth:        stats.RuneDepth,
		PoolDepth:        util.IntStr(2 * extra.runeDepth),
		PoolUnits:        stats.Units,
		BuyVolume:        stats.ToAssetVolume,
		SellVolume:       stats.ToRuneVolume,
		PoolVolume:       stats.SwapVolume,
		Volume24h:        util.IntStr(dailyVolume),
		BuyAssetCount:    stats.ToAssetCount,
		SellAssetCount:   stats.ToRuneCount,
		SwappingTxCount:  stats.SwapCount,
		SwappersCount:    stats.UniqueSwapperCount,
		BuyTxAverage:     ratioStr(extra.toAssetVolume, extra.toAssetCount),
		SellTxAverage:    ratioStr(extra.toRuneVolume, extra.toRuneCount),
		PoolTxAverage:    ratioStr(extra.totalVolume, extra.swapCount),
		BuySlipAverage:   stats.ToAssetAverageSlip,
		SellSlipAverage:  stats.ToRuneAverageSlip,
		PoolSlipAverage:  stats.AverageSlip,
		BuyFeesTotal:     stats.ToAssetFees,
		SellFeesTotal:    stats.ToRuneFees,
		PoolFeesTotal:    stats.TotalFees,
		BuyFeeAverage:    ratioStr(extra.toAssetFees, extra.toAssetCount),
		SellFeeAverage:   ratioStr(extra.toRuneFees, extra.toRuneCount),
		PoolFeeAverage:   ratioStr(extra.totalFees, extra.swapCount),
		PoolAPY:          floatStr(poolAPY),
		AssetStakedTotal: stats.AddAssetLiquidityVolume,
		RuneStakedTotal:  stats.AddRuneLiquidityVolume,
		PoolStakedTotal:  stats.AddLiquidityVolume,
		StakeTxCount:     stats.AddLiquidityCount,
		WithdrawTxCount:  stats.WithdrawCount,
		StakingTxCount:   util.IntStr(addLiquidityCount + withdrawCount),
		StakersCount:     stats.UniqueMemberCount,
	}

	respJSON(w, result)
}
