package api

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func setAggregatesStats(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse) (merr miderr.Err) {

	state := timeseries.Latest.GetState()

	poolInfo := state.PoolInfo(pool)
	if poolInfo == nil || !poolInfo.ExistsNow() {
		merr = miderr.BadRequestF("Unknown pool: %s", pool)
		return
	}

	liquidityUnitsMap, err := stat.CurrentPoolsLiquidityUnits(ctx, []string{pool})
	if err != nil {
		return miderr.InternalErrE(err)
	}
	lpUnits := liquidityUnitsMap[pool]

	// TODO(muninn): consider the period parameter, not assume always 30 days
	apr, err := GetSinglePoolAPR(
		ctx, state.Pools[pool], lpUnits, pool,
		buckets.Start().ToNano(), buckets.End().ToNano())
	if err != nil {
		return miderr.InternalErrE(err)
	}

	status, err := timeseries.PoolStatus(ctx, pool)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}

	price := poolInfo.AssetPrice()
	priceUSD := price * stat.RunePriceUSD()
	liquidityUnits := lpUnits
	synthUnits := timeseries.CalculateSynthUnits(
		poolInfo.AssetDepth, poolInfo.SynthDepth, liquidityUnits)
	poolUnits := liquidityUnits + synthUnits

	ret.Asset = pool
	ret.AssetDepth = util.IntStr(poolInfo.AssetDepth)
	ret.RuneDepth = util.IntStr(poolInfo.RuneDepth)
	ret.AnnualPercentageRate = floatStr(apr)
	ret.PoolAPY = floatStr(util.Max(apr, 0))
	ret.AssetPrice = floatStr(price)
	ret.AssetPriceUSD = floatStr(priceUSD)
	ret.Status = status
	ret.Units = util.IntStr(poolUnits)
	ret.LiquidityUnits = util.IntStr(liquidityUnits)
	ret.SynthUnits = util.IntStr(synthUnits)
	ret.SynthSupply = util.IntStr(poolInfo.SynthDepth)

	return
}

func setSwapStats(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse) (merr miderr.Err) {

	swapHistory, err := stat.GetOneIntervalSwapsNoUSD(ctx, &pool, buckets)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}

	ret.ToRuneVolume = util.IntStr(swapHistory.AssetToRuneVolume)
	ret.ToAssetVolume = util.IntStr(swapHistory.RuneToAssetVolume)
	ret.SwapVolume = util.IntStr(swapHistory.TotalVolume)

	ret.ToRuneCount = util.IntStr(swapHistory.AssetToRuneCount)
	ret.ToAssetCount = util.IntStr(swapHistory.RuneToAssetCount)
	ret.SwapCount = util.IntStr(swapHistory.TotalCount)

	ret.ToAssetAverageSlip = ratioStr(swapHistory.RuneToAssetSlip, swapHistory.RuneToAssetCount)
	ret.ToRuneAverageSlip = ratioStr(swapHistory.AssetToRuneSlip, swapHistory.AssetToRuneCount)
	ret.AverageSlip = ratioStr(swapHistory.TotalSlip, swapHistory.TotalCount)

	ret.ToAssetFees = util.IntStr(swapHistory.RuneToAssetFees)
	ret.ToRuneFees = util.IntStr(swapHistory.AssetToRuneFees)
	ret.TotalFees = util.IntStr(swapHistory.TotalFees)

	return
}

func setLiquidityStats(
	ctx context.Context, pool string, buckets db.Buckets,
	ret *oapigen.PoolStatsResponse) (merr miderr.Err) {

	var allLiquidity oapigen.LiquidityHistoryResponse
	// TODO(muninn): replace with GetLiquidityHistoryNOUSD to speed up
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

func statsForPool(ctx context.Context, pool string, buckets db.Buckets) (
	ret oapigen.PoolStatsResponse, merr miderr.Err) {

	merr = setAggregatesStats(ctx, pool, buckets, &ret)
	if merr != nil {
		return
	}

	merr = setSwapStats(ctx, pool, buckets, &ret)
	if merr != nil {
		return
	}

	// TODO(huginn): optimize deposit/withdraw total volme and count
	merr = setLiquidityStats(ctx, pool, buckets, &ret)
	if merr != nil {
		return
	}

	// TODO(huginn): optimize unique member adresses to use latest
	_, memberBucket, err := stat.GetMembersCountBucket(ctx, buckets, pool)
	if err != nil {
		merr = miderr.InternalErrE(err)
		return
	}
	ret.UniqueMemberCount = util.IntStr(memberBucket[buckets.Count()-1].Count)

	ret.UniqueSwapperCount = "0" // deprecated

	return
}

func jsonPoolStats(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	pool := ps[0].Value

	urlParams := r.URL.Query()
	buckets, err := parsePeriodParam(&urlParams)
	if err != nil {
		miderr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	result, merr := statsForPool(r.Context(), pool, buckets)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}
	respJSON(w, result)
}
