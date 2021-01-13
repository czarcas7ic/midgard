package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

// statsForPools is used both for stats and stats/legacy.
// The creates PoolStatsResponse, but to help the creation of PoolLegacyResponse it also
// puts int values into this struct. This way we don't need to convert back and forth between
// strings and ints.
type extraStats struct {
	runeDepth     int64
	toAssetCount  int64
	toRuneCount   int64
	swapCount     int64
	toAssetVolume int64
	toRuneVolume  int64
	totalVolume   int64
	totalFees     int64
}

func setAggregatesStats(
	ctx context.Context, pool string,
	ret *oapigen.PoolStatsResponse, extra *extraStats) (merr miderr.Err) {

	assetE8DepthPerPool, runeE8DepthPerPool, timestamp := timeseries.AssetAndRuneDepths()
	_, assetOk := assetE8DepthPerPool[pool]
	_, runeOk := runeE8DepthPerPool[pool]

	// TODO(acsaba): check that pool exists.
	// Return not found if there's no track of the pool
	if !assetOk && !runeOk {
		merr = miderr.BadRequestF("Unknown pool: %s", pool)
		return
	}

	status, err := timeseries.PoolStatus(ctx, pool, timestamp)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}

	aggregates, err := getPoolAggregates(ctx, []string{pool})
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}

	assetDepth := aggregates.assetE8DepthPerPool[pool]
	runeDepth := aggregates.runeE8DepthPerPool[pool]
	dailyVolume := aggregates.dailyVolumes[pool]
	poolUnits := aggregates.poolUnits[pool]
	rewards := aggregates.poolWeeklyRewards[pool]
	poolAPY := timeseries.GetPoolAPY(runeDepth, rewards)

	ret.Asset = pool
	ret.AssetDepth = intStr(assetDepth)
	ret.RuneDepth = intStr(runeDepth)
	ret.PoolAPY = floatStr(poolAPY)
	ret.AssetPrice = floatStr(stat.AssetPrice(assetDepth, runeDepth))
	ret.Status = status
	ret.Units = intStr(poolUnits)
	ret.Volume24h = intStr(dailyVolume)

	extra.runeDepth = runeDepth
	return
}

func setSwapStats(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse, extra *extraStats) (merr miderr.Err) {

	allSwaps, err := stat.GetPoolSwaps(ctx, pool, buckets)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}
	if len(allSwaps) != 1 {
		merr = miderr.InternalErr("Internal error: wrong time interval.")
		return
	}
	var swapHistory stat.SwapBucket = allSwaps[0]

	ret.ToRuneVolume = intStr(swapHistory.ToRuneVolume)
	ret.ToAssetVolume = intStr(swapHistory.ToAssetVolume)
	ret.SwapVolume = intStr(swapHistory.TotalVolume)

	ret.ToRuneCount = intStr(swapHistory.ToRuneCount)
	ret.ToAssetCount = intStr(swapHistory.ToAssetCount)
	ret.SwapCount = intStr(swapHistory.TotalCount)

	ret.AverageSlip = ratioStr(swapHistory.TotalSlip, swapHistory.TotalCount)
	ret.TotalFees = intStr(swapHistory.TotalFees)

	extra.toAssetCount = swapHistory.ToAssetCount
	extra.toRuneCount = swapHistory.ToRuneCount
	extra.swapCount = swapHistory.TotalCount
	extra.toAssetVolume = swapHistory.ToAssetVolume
	extra.toRuneVolume = swapHistory.ToRuneVolume
	extra.totalVolume = swapHistory.TotalVolume
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
	ret.AddLiquidityVolume = allLiquidity.Meta.AddLiquidityVolume
	ret.AddLiquidityCount = allLiquidity.Meta.AddLiquidityCount
	ret.WithdrawVolume = allLiquidity.Meta.WithdrawVolume
	ret.WithdrawCount = allLiquidity.Meta.WithdrawCount
	return
}

func statsForPool(ctx context.Context, pool string, buckets db.Buckets) (
	ret oapigen.PoolStatsResponse, extra extraStats, merr miderr.Err) {

	merr = setAggregatesStats(ctx, pool, &ret, &extra)
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

	return
}

func jsonPoolStats(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	pool := ps[0].Value

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}
	var buckets db.Buckets
	now := timeseries.Now().ToSecond() + 1
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
	}

	respJSON(w, result)
}
func jsonPoolStatsLegacy(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	pool := ps[0].Value
	stats, extra, merr := statsForPool(r.Context(), pool, db.AllHistoryBuckets())
	if merr != nil {
		merr.ReportHTTP(w)
	}

	addLiquidityCount, _ := strconv.ParseInt(stats.AddLiquidityCount, 10, 64)
	withdrawCount, _ := strconv.ParseInt(stats.WithdrawCount, 10, 64)

	result := oapigen.PoolLegacyResponse{
		Asset:           stats.Asset,
		Status:          stats.Status,
		Price:           stats.AssetPrice,
		AssetDepth:      stats.AssetDepth,
		RuneDepth:       stats.RuneDepth,
		PoolDepth:       intStr(2 * extra.runeDepth),
		PoolUnits:       stats.Units,
		BuyVolume:       stats.ToAssetVolume,
		SellVolume:      stats.ToRuneVolume,
		PoolVolume:      stats.SwapVolume,
		Volume24h:       stats.Volume24h,
		BuyAssetCount:   stats.ToAssetCount,
		SellAssetCount:  stats.ToRuneCount,
		SwappingTxCount: stats.SwapCount,
		BuyTxAverage:    ratioStr(extra.toAssetVolume, extra.toAssetCount),
		SellTxAverage:   ratioStr(extra.toRuneVolume, extra.toRuneCount),
		PoolTxAverage:   ratioStr(extra.totalVolume, extra.swapCount),
		PoolSlipAverage: stats.AverageSlip,
		PoolFeesTotal:   stats.TotalFees,
		PoolFeeAverage:  ratioStr(extra.totalFees, extra.swapCount),
		PoolAPY:         stats.PoolAPY,
		PoolStakedTotal: stats.AddLiquidityVolume,
		StakeTxCount:    stats.AddLiquidityCount,
		WithdrawTxCount: stats.WithdrawCount,
		StakingTxCount:  intStr(addLiquidityCount + withdrawCount),
	}

	respJSON(w, result)
}
