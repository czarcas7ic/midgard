package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/decimal"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"

	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

// Version 1 compatibility is a minimal effort attempt to provide smooth migration.

// TODO(HooriRn): this struct is not needed since the graphql depracation, replace with the corresponding oapi version. (delete-graphql)
type Health struct {
	CatchingUp    bool  `json:"catching_up"`
	Database      bool  `json:"database"`
	ScannerHeight int64 `json:"scannerHeight,string"`
}

func jsonHealth(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	height, _, _ := timeseries.LastBlock()
	synced := db.FullyCaughtUp()

	respJSON(w, oapigen.HealthResponse{
		InSync:         synced,
		Database:       true,
		ScannerHeight:  util.IntStr(height + 1),
		LastThorNode:   db.LastThorNodeBlock.AsHeightTS(),
		LastFetched:    db.LastFetchedBlock.AsHeightTS(),
		LastCommitted:  db.LastCommittedBlock.AsHeightTS(),
		LastAggregated: db.LastAggregatedBlock.AsHeightTS(),
	})
}

func jsonEarningsHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()
	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	var res oapigen.EarningsHistoryResponse
	res, err := stat.GetEarningsHistory(r.Context(), buckets)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}
	if buckets.OneInterval() {
		res.Intervals = oapigen.EarningsHistoryIntervals{}
	}
	respJSON(w, res)
}

func jsonLiquidityHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	pool := util.ConsumeUrlParam(&urlParams, "pool")
	if pool == "" {
		pool = "*"
	}
	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	var res oapigen.LiquidityHistoryResponse
	res, err := stat.GetLiquidityHistory(r.Context(), buckets, pool)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}
	if buckets.OneInterval() {
		res.Intervals = oapigen.LiquidityHistoryIntervals{}
	}
	respJSON(w, res)
}

func jsonDepths(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	pool := ps[0].Value

	if !timeseries.PoolExists(pool) {
		miderr.BadRequestF("Unknown pool: %s", pool).ReportHTTP(w)
		return
	}

	urlParams := r.URL.Query()
	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	beforeDepth, depths, err := stat.PoolDepthHistory(r.Context(), buckets, pool)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}
	beforeLPUnits, units, err := stat.PoolLiquidityUnitsHistory(r.Context(), buckets, pool)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}
	if len(depths) != len(units) || depths[0].Window != units[0].Window {
		miderr.InternalErr("Buckets misaligned").ReportHTTP(w)
		return
	}
	var result oapigen.DepthHistoryResponse = toOapiDepthResponse(
		r.Context(), beforeDepth, depths, beforeLPUnits, units)
	respJSON(w, result)
}

func toOapiDepthResponse(
	ctx context.Context,
	beforeDepth timeseries.PoolDepths,
	depths []stat.PoolDepthBucket,
	beforeLPUnits int64,
	units []stat.UnitsBucket) (
	result oapigen.DepthHistoryResponse) {
	result.Intervals = make(oapigen.DepthHistoryIntervals, 0, len(depths))
	for i, bucket := range depths {
		liquidityUnits := units[i].Units
		synthUnits := timeseries.CalculateSynthUnits(
			bucket.Depths.AssetDepth, bucket.Depths.SynthDepth, liquidityUnits)
		poolUnits := liquidityUnits + synthUnits
		assetDepth := bucket.Depths.AssetDepth
		runeDepth := bucket.Depths.RuneDepth
		liqUnitValIndex := luvi(bucket.Depths.AssetDepth, bucket.Depths.RuneDepth, poolUnits)
		result.Intervals = append(result.Intervals, oapigen.DepthHistoryItem{
			StartTime:      util.IntStr(bucket.Window.From.ToI()),
			EndTime:        util.IntStr(bucket.Window.Until.ToI()),
			AssetDepth:     util.IntStr(assetDepth),
			RuneDepth:      util.IntStr(runeDepth),
			AssetPrice:     floatStr(bucket.Depths.AssetPrice()),
			AssetPriceUSD:  floatStr(bucket.AssetPriceUSD),
			LiquidityUnits: util.IntStr(liquidityUnits),
			SynthUnits:     util.IntStr(synthUnits),
			SynthSupply:    util.IntStr(bucket.Depths.SynthDepth),
			Units:          util.IntStr(poolUnits),
			Luvi:           floatStr(liqUnitValIndex),
		})
	}
	endDepth := depths[len(depths)-1].Depths
	endLPUnits := units[len(units)-1].Units
	beforeSynthUnits := timeseries.CalculateSynthUnits(
		beforeDepth.AssetDepth, beforeDepth.SynthDepth, beforeLPUnits)
	endSynthUnits := timeseries.CalculateSynthUnits(
		endDepth.AssetDepth, endDepth.SynthDepth, endLPUnits)
	luviIncrease := luviFromLPUnits(endDepth, endLPUnits) / luviFromLPUnits(beforeDepth, beforeLPUnits)

	result.Meta.StartTime = util.IntStr(depths[0].Window.From.ToI())
	result.Meta.EndTime = util.IntStr(depths[len(depths)-1].Window.Until.ToI())
	result.Meta.PriceShiftLoss = floatStr(priceShiftLoss(beforeDepth, endDepth))
	result.Meta.LuviIncrease = floatStr(luviIncrease)
	result.Meta.StartAssetDepth = util.IntStr(beforeDepth.AssetDepth)
	result.Meta.StartRuneDepth = util.IntStr(beforeDepth.RuneDepth)
	result.Meta.StartLPUnits = util.IntStr(beforeLPUnits)
	result.Meta.StartSynthUnits = util.IntStr(beforeSynthUnits)
	result.Meta.EndAssetDepth = util.IntStr(endDepth.AssetDepth)
	result.Meta.EndRuneDepth = util.IntStr(endDepth.RuneDepth)
	result.Meta.EndLPUnits = util.IntStr(endLPUnits)
	result.Meta.EndSynthUnits = util.IntStr(endSynthUnits)
	return
}

func luvi(assetE8 int64, runeE8 int64, poolUnits int64) float64 {
	if poolUnits <= 0 {
		return math.NaN()
	}
	return math.Sqrt(float64(assetE8)*float64(runeE8)) / float64(poolUnits)
}

func luviFromLPUnits(depths timeseries.PoolDepths, lpUnits int64) float64 {
	synthUnits := timeseries.CalculateSynthUnits(depths.AssetDepth, depths.SynthDepth, lpUnits)
	return luvi(depths.AssetDepth, depths.RuneDepth, lpUnits+synthUnits)
}

func priceShiftLoss(beforeDepth timeseries.PoolDepths, lastDepth timeseries.PoolDepths) float64 {
	//Price0 = R0 / A0 (rune depth at time 0, asset depth at time 0)
	//Price1 = R1 / A1 (rune depth at time 1, asset depth at time 1)
	//PriceShift = Price1 / Price0
	//PriceShiftLoss = 2*sqrt(PriceShift) / (1 + PriceShift)
	price0 := float64(beforeDepth.RuneDepth) / float64(beforeDepth.AssetDepth)
	price1 := float64(lastDepth.RuneDepth) / float64(lastDepth.AssetDepth)
	ratio := price1 / price0
	return 2 * math.Sqrt(ratio) / (1 + ratio)
}

func jsonSwapHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	var pool *string
	poolParam := util.ConsumeUrlParam(&urlParams, "pool")
	if poolParam != "" {
		pool = &poolParam
	}

	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	mergedPoolSwaps, err := stat.GetPoolSwaps(r.Context(), pool, buckets)
	if err != nil {
		miderr.InternalErr(err.Error()).ReportHTTP(w)
		return
	}
	var result oapigen.SwapHistoryResponse = createVolumeIntervals(mergedPoolSwaps)
	if buckets.OneInterval() {
		result.Intervals = oapigen.SwapHistoryIntervals{}
	}
	respJSON(w, result)
}

func toSwapHistoryItem(bucket stat.SwapBucket) oapigen.SwapHistoryItem {
	return oapigen.SwapHistoryItem{
		StartTime:              util.IntStr(bucket.StartTime.ToI()),
		EndTime:                util.IntStr(bucket.EndTime.ToI()),
		ToAssetVolume:          util.IntStr(bucket.RuneToAssetVolume),
		ToRuneVolume:           util.IntStr(bucket.AssetToRuneVolume),
		SynthMintVolume:        util.IntStr(bucket.RuneToSynthVolume),
		SynthRedeemVolume:      util.IntStr(bucket.SynthToRuneVolume),
		TotalVolume:            util.IntStr(bucket.TotalVolume),
		ToAssetCount:           util.IntStr(bucket.RuneToAssetCount),
		ToRuneCount:            util.IntStr(bucket.AssetToRuneCount),
		SynthMintCount:         util.IntStr(bucket.RuneToSynthCount),
		SynthRedeemCount:       util.IntStr(bucket.SynthToRuneCount),
		TotalCount:             util.IntStr(bucket.TotalCount),
		ToAssetFees:            util.IntStr(bucket.RuneToAssetFees),
		ToRuneFees:             util.IntStr(bucket.AssetToRuneFees),
		SynthMintFees:          util.IntStr(bucket.RuneToSynthFees),
		SynthRedeemFees:        util.IntStr(bucket.SynthToRuneFees),
		TotalFees:              util.IntStr(bucket.TotalFees),
		ToAssetAverageSlip:     ratioStr(bucket.RuneToAssetSlip, bucket.RuneToAssetCount),
		ToRuneAverageSlip:      ratioStr(bucket.AssetToRuneSlip, bucket.AssetToRuneCount),
		SynthMintAverageSlip:   ratioStr(bucket.RuneToSynthSlip, bucket.RuneToSynthCount),
		SynthRedeemAverageSlip: ratioStr(bucket.SynthToRuneSlip, bucket.SynthToRuneCount),
		AverageSlip:            ratioStr(bucket.TotalSlip, bucket.TotalCount),
		RunePriceUSD:           floatStr(bucket.RunePriceUSD),
	}
}

func createVolumeIntervals(buckets []stat.SwapBucket) (result oapigen.SwapHistoryResponse) {
	metaBucket := stat.SwapBucket{}

	for _, bucket := range buckets {
		metaBucket.AddBucket(bucket)

		result.Intervals = append(result.Intervals, toSwapHistoryItem(bucket))
	}

	result.Meta = toSwapHistoryItem(metaBucket)
	result.Meta.StartTime = result.Intervals[0].StartTime
	result.Meta.EndTime = result.Intervals[len(result.Intervals)-1].EndTime
	result.Meta.RunePriceUSD = result.Intervals[len(result.Intervals)-1].RunePriceUSD
	return
}

// TODO(huginn): remove when bonds are fixed
var ShowBonds bool = false

func jsonTVLHistory(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	buckets, merr := db.BucketsFromQuery(r.Context(), &urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}
	merr = util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	// TODO(huginn): optimize, just this call is 1.8 sec
	// defer timer.Console("tvlDepthSingle")()
	depths, err := stat.TVLDepthHistory(r.Context(), buckets)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}

	bonds, err := stat.BondsHistory(r.Context(), buckets)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}
	if len(depths) != len(bonds) || depths[0].Window != bonds[0].Window {
		miderr.InternalErr("Buckets misalligned").ReportHTTP(w)
		return
	}

	var result oapigen.TVLHistoryResponse = toTVLHistoryResponse(depths, bonds)
	respJSON(w, result)
}

func toTVLHistoryResponse(depths []stat.TVLDepthBucket, bonds []stat.BondBucket) (
	result oapigen.TVLHistoryResponse) {

	showBonds := func(value string) *string {
		if !ShowBonds {
			return nil
		}
		return &value
	}

	result.Intervals = make(oapigen.TVLHistoryIntervals, 0, len(depths))
	for i, bucket := range depths {
		pools := 2 * bucket.TotalPoolDepth
		bonds := bonds[i].Bonds
		result.Intervals = append(result.Intervals, oapigen.TVLHistoryItem{
			StartTime:        util.IntStr(bucket.Window.From.ToI()),
			EndTime:          util.IntStr(bucket.Window.Until.ToI()),
			TotalValuePooled: util.IntStr(pools),
			TotalValueBonded: showBonds(util.IntStr(bonds)),
			TotalValueLocked: showBonds(util.IntStr(pools + bonds)),
			RunePriceUSD:     floatStr(bucket.RunePriceUSD),
		})
	}
	result.Meta = result.Intervals[len(depths)-1]
	result.Meta.StartTime = result.Intervals[0].StartTime
	return
}

func jsonNetwork(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	network, err := timeseries.GetNetworkData(r.Context())
	if err != nil {
		respError(w, err)
		return
	}

	respJSON(w, network)
}

// TODO(HooriRn): this struct is not needed since the graphql depracation, replace with the corresponding oapi version. (delete-graphql)
type Node struct {
	Secp256K1 string `json:"secp256k1"`
	Ed25519   string `json:"ed25519"`
}

func jsonNodes(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	secpAddrs, edAddrs, err := timeseries.NodesSecpAndEd(r.Context(), time.Now())
	if err != nil {
		respError(w, err)
		return
	}

	m := make(map[string]struct {
		Secp string
		Ed   string
	}, len(secpAddrs))
	for key, addr := range secpAddrs {
		e := m[addr]
		e.Secp = key
		m[addr] = e
	}
	for key, addr := range edAddrs {
		e := m[addr]
		e.Ed = key
		m[addr] = e
	}

	array := make([]oapigen.Node, 0, len(m))
	for key, e := range m {
		array = append(array, oapigen.Node{
			Secp256k1:   e.Secp,
			Ed25519:     e.Ed,
			NodeAddress: key,
		})
	}
	respJSON(w, array)
}

func jsonKnownPools(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()
	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	lastTime := timeseries.Latest.GetState().Timestamp
	pools, err := timeseries.GetPoolsStatuses(r.Context(), lastTime)
	if err != nil {
		respError(w, err)
		return
	}

	respJSON(w, oapigen.KnownPools{AdditionalProperties: pools})
}

// Filters out Suspended pools.
// If there is a status url parameter then returns pools with that status only.
func poolsWithRequestedStatus(
	ctx context.Context, urlParams *url.Values, statusMap map[string]string) (
	[]string, error) {

	pools, err := timeseries.PoolsWithDeposit(ctx)
	if err != nil {
		return nil, err
	}
	requestedStatus := util.ConsumeUrlParam(urlParams, "status")
	if requestedStatus != "" {
		const errormsg = "Max one status parameter, accepted values: available, staged, suspended"
		requestedStatus = strings.ToLower(requestedStatus)
		// Allowed statuses in
		// https://gitlab.com/thorchain/thornode/-/blob/master/x/thorchain/types/type_pool.go
		if requestedStatus != "available" && requestedStatus != "staged" && requestedStatus != "suspended" {
			return nil, fmt.Errorf(errormsg)
		}
	}
	ret := []string{}
	for _, pool := range pools {
		poolStatus := poolStatusFromMap(pool, statusMap)
		if poolStatus != "suspended" && (requestedStatus == "" || poolStatus == requestedStatus) {
			ret = append(ret, pool)
		}
	}
	return ret, nil
}

func GetPoolAPRs(ctx context.Context,
	depthsNow timeseries.DepthMap, lpUnitsNow map[string]int64, pools []string,
	aprStartTime db.Nano, now db.Nano) (
	map[string]float64, error) {

	var periodsPerYear float64 = 365 * 24 * 60 * 60 * 1e9 / float64(now-aprStartTime)
	liquidityUnitsBefore, err := stat.PoolsLiquidityUnitsBefore(ctx, pools, &aprStartTime)
	if err != nil {
		return nil, err
	}
	depthsBefore, err := stat.DepthsBefore(ctx, pools, aprStartTime)
	if err != nil {
		return nil, err
	}

	ret := map[string]float64{}
	for _, pool := range pools {
		luviNow := luviFromLPUnits(depthsNow[pool], lpUnitsNow[pool])
		luviBefore := luviFromLPUnits(depthsBefore[pool], liquidityUnitsBefore[pool])
		luviIncrease := luviNow / luviBefore
		ret[pool] = (luviIncrease - 1) * periodsPerYear
	}
	return ret, nil
}

func GetSinglePoolAPR(ctx context.Context,
	depths timeseries.PoolDepths, lpUnits int64, pool string, start db.Nano, now db.Nano) (
	float64, error) {
	aprs, err := GetPoolAPRs(
		ctx,
		timeseries.DepthMap{pool: depths},
		map[string]int64{pool: lpUnits},
		[]string{pool},
		start, now)
	if err != nil {
		return 0, err
	}
	return aprs[pool], nil
}

type poolAggregates struct {
	depths               timeseries.DepthMap
	dailyVolumes         map[string]int64
	liquidityUnits       map[string]int64
	annualPercentageRate map[string]float64
}

func getPoolAggregates(ctx context.Context, pools []string, apyBucket db.Buckets) (
	*poolAggregates, error) {

	latestState := timeseries.Latest.GetState()
	now := latestState.NextSecond()
	window24h := db.Window{From: now - 24*60*60, Until: now}

	dailyVolumes, err := stat.PoolsTotalVolume(ctx, pools, window24h)
	if err != nil {
		return nil, err
	}

	liquidityUnitsNow, err := stat.PoolsLiquidityUnitsBefore(ctx, pools, nil)
	if err != nil {
		return nil, err
	}

	aprs, err := GetPoolAPRs(ctx, latestState.Pools, liquidityUnitsNow, pools,
		apyBucket.Start().ToNano(), apyBucket.End().ToNano())
	if err != nil {
		return nil, err
	}

	aggregates := poolAggregates{
		depths:               latestState.Pools,
		dailyVolumes:         dailyVolumes,
		liquidityUnits:       liquidityUnitsNow,
		annualPercentageRate: aprs,
	}

	return &aggregates, nil
}

func poolStatusFromMap(pool string, statusMap map[string]string) string {
	status, ok := statusMap[pool]
	if !ok {
		status = timeseries.DefaultPoolStatus
	}
	return status
}

func buildPoolDetail(
	ctx context.Context, pool, status string, aggregates poolAggregates, runePriceUsd float64, decimal int64) oapigen.PoolDetail {
	assetDepth := aggregates.depths[pool].AssetDepth
	runeDepth := aggregates.depths[pool].RuneDepth
	synthSupply := aggregates.depths[pool].SynthDepth
	dailyVolume := aggregates.dailyVolumes[pool]
	liquidityUnits := aggregates.liquidityUnits[pool]
	synthUnits := timeseries.CalculateSynthUnits(assetDepth, synthSupply, liquidityUnits)
	poolUnits := liquidityUnits + synthUnits
	apr := aggregates.annualPercentageRate[pool]
	price := timeseries.AssetPrice(assetDepth, runeDepth)
	priceUSD := price * runePriceUsd

	return oapigen.PoolDetail{
		Asset:                pool,
		AssetDepth:           util.IntStr(assetDepth),
		RuneDepth:            util.IntStr(runeDepth),
		AnnualPercentageRate: floatStr(apr),
		PoolAPY:              floatStr(util.Max(apr, 0)),
		AssetPrice:           floatStr(price),
		AssetPriceUSD:        floatStr(priceUSD),
		Status:               status,
		Units:                util.IntStr(poolUnits),
		LiquidityUnits:       util.IntStr(liquidityUnits),
		SynthUnits:           util.IntStr(synthUnits),
		SynthSupply:          util.IntStr(synthSupply),
		Volume24h:            util.IntStr(dailyVolume),
		NativeDecimal:        util.IntStr(decimal),
	}
}

func jsonPools(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	_, lastTime, _ := timeseries.LastBlock()
	statusMap, err := timeseries.GetPoolsStatuses(r.Context(), db.Nano(lastTime.UnixNano()))
	if err != nil {
		respError(w, err)
		return
	}
	pools, err := poolsWithRequestedStatus(r.Context(), &urlParams, statusMap)
	if err != nil {
		respError(w, err)
		return
	}

	apyBucket, err := parsePeriodParam(&urlParams)
	if err != nil {
		miderr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	aggregates, err := getPoolAggregates(r.Context(), pools, apyBucket)
	if err != nil {
		respError(w, err)
		return
	}

	runePriceUsd := stat.RunePriceUSD()

	poolsDecimal := decimal.PoolsDecimal()

	poolsResponse := oapigen.PoolsResponse{}
	for _, pool := range pools {
		runeDepth := aggregates.depths[pool].RuneDepth
		assetDepth := aggregates.depths[pool].AssetDepth
		poolDecimal, ok := poolsDecimal[pool]
		if !ok {
			poolDecimal.NativeDecimals = -1
		}
		if 0 < runeDepth && 0 < assetDepth {
			status := poolStatusFromMap(pool, statusMap)
			poolsResponse = append(poolsResponse, buildPoolDetail(r.Context(), pool, status, *aggregates, runePriceUsd, poolDecimal.NativeDecimals))
		}
	}

	respJSON(w, poolsResponse)
}

func jsonPool(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()

	apyBucket, err := parsePeriodParam(&urlParams)
	if err != nil {
		miderr.BadRequest(err.Error()).ReportHTTP(w)
		return
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	pool := ps[0].Value

	if !timeseries.PoolExistsNow(pool) {
		miderr.BadRequestF("Unknown pool: %s", pool).ReportHTTP(w)
		return
	}

	status, err := timeseries.PoolStatus(r.Context(), pool)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}

	aggregates, err := getPoolAggregates(r.Context(), []string{pool}, apyBucket)
	if err != nil {
		miderr.InternalErrE(err).ReportHTTP(w)
		return
	}

	runePriceUsd := stat.RunePriceUSD()

	poolsDecimal := decimal.PoolsDecimal()

	poolDecimal, ok := poolsDecimal[pool]
	if !ok {
		poolDecimal.NativeDecimals = -1
	}

	poolResponse := oapigen.PoolResponse(
		buildPoolDetail(r.Context(), pool, status, *aggregates, runePriceUsd, poolDecimal.NativeDecimals))
	respJSON(w, poolResponse)
}

// returns string array
func jsonMembers(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()

	var pool *string
	poolParam := util.ConsumeUrlParam(&urlParams, "pool")
	if poolParam != "" {
		pool = &poolParam
		if !timeseries.PoolExists(*pool) {
			miderr.BadRequestF("Unknown pool: %s", *pool).ReportHTTP(w)
			return
		}
	}
	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	addrs, err := timeseries.GetMemberIds(r.Context(), pool)
	if err != nil {
		respError(w, err)
		return
	}
	result := oapigen.MembersResponse(addrs)
	respJSON(w, result)
}

func jsonMemberDetails(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	addr := ps[0].Value

	var pools timeseries.MemberPools
	var err error
	for _, addr := range withLowered(addr) {
		pools, err = timeseries.GetMemberPools(r.Context(), addr)
		if err != nil {
			respError(w, err)
			return
		}
		if len(pools) > 0 {
			break
		}
	}
	if len(pools) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	respJSON(w, oapigen.MemberDetailsResponse{
		Pools: pools.ToOapigen(),
	})
}

func jsonTHORName(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	name := ps[0].Value

	n, err := timeseries.GetTHORName(r.Context(), name)
	if err != nil {
		respError(w, err)
		return
	}
	if n.Owner == "" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	entries := make([]oapigen.THORNameEntry, len(n.Entries))
	for i, e := range n.Entries {
		entries[i] = oapigen.THORNameEntry{
			Chain:   e.Chain,
			Address: e.Address,
		}
	}

	respJSON(w, oapigen.THORNameDetailsResponse{
		Owner:   n.Owner,
		Expire:  util.IntStr(n.Expire),
		Entries: entries,
	})
}

type ThornameReverseLookupFunc func(ctx context.Context, addr string) (names []string, err error)

func jsonTHORNameReverse(
	w http.ResponseWriter, r *http.Request, ps httprouter.Params,
	lookupFunc ThornameReverseLookupFunc) {

	merr := util.CheckUrlEmpty(r.URL.Query())
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	addr := ps[0].Value

	var names []string
	for _, addr := range withLowered(addr) {
		var err error
		names, err = lookupFunc(r.Context(), addr)
		if err != nil {
			respError(w, err)
			return
		}
		if 0 < len(names) {
			break
		}
	}

	if len(names) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	respJSON(w, oapigen.ReverseTHORNameResponse(
		names,
	))
}

func jsonTHORNameAddress(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jsonTHORNameReverse(w, r, ps, timeseries.GetTHORNamesByAddress)
}

func jsonTHORNameOwner(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jsonTHORNameReverse(w, r, ps, timeseries.GetTHORNamesByOwnerAddress)
}

func jsonChurns(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	churns, err := timeseries.GetChurnsData(r.Context())
	if err != nil {
		return
	}
	respJSON(w, churns)
}

// TODO(muninn): remove cache once it's <0.5s
func calculateJsonStats(ctx context.Context, w io.Writer) error {
	state := timeseries.Latest.GetState()
	now := db.NowSecond()
	window := db.Window{From: 0, Until: now}

	// TODO(huginn): Rewrite to member table if doable, stakes/unstakes (deposits/withdraws) lookup is ~0.8 s
	deposits, err := stat.DepositsLookup(ctx, window)
	if err != nil {
		return err
	}
	withdraws, err := stat.WithdrawsLookup(ctx, window)
	if err != nil {
		return err
	}

	swapsAll, err := stat.GlobalSwapStats(ctx, "day", 0)
	if err != nil {
		return err
	}

	swaps24h, err := stat.GlobalSwapStats(ctx, "5min", now-24*60*60)
	if err != nil {
		return err
	}

	swaps30d, err := stat.GlobalSwapStats(ctx, "hour", now-30*24*60*60)
	if err != nil {
		return err
	}

	var runeDepth int64
	for _, poolInfo := range state.Pools {
		runeDepth += poolInfo.RuneDepth
	}

	switchedRune, err := stat.SwitchedRune(ctx)
	if err != nil {
		return err
	}

	runePrice := stat.RunePriceUSD()

	writeJSON(w, oapigen.StatsResponse{
		RuneDepth:                     util.IntStr(runeDepth),
		SwitchedRune:                  util.IntStr(switchedRune),
		RunePriceUSD:                  floatStr(runePrice),
		SwapVolume:                    util.IntStr(swapsAll.Totals().Volume),
		SwapCount24h:                  util.IntStr(swaps24h.Totals().Count),
		SwapCount30d:                  util.IntStr(swaps30d.Totals().Count),
		SwapCount:                     util.IntStr(swapsAll.Totals().Count),
		ToAssetCount:                  util.IntStr(swapsAll[db.RuneToAsset].Count),
		ToRuneCount:                   util.IntStr(swapsAll[db.AssetToRune].Count),
		SynthMintCount:                util.IntStr(swapsAll[db.RuneToSynth].Count),
		SynthBurnCount:                util.IntStr(swapsAll[db.SynthToRune].Count),
		DailyActiveUsers:              "0", // deprecated
		MonthlyActiveUsers:            "0", // deprecated
		UniqueSwapperCount:            "0", // deprecated
		AddLiquidityVolume:            util.IntStr(deposits.TotalVolume),
		WithdrawVolume:                util.IntStr(withdraws.TotalVolume),
		ImpermanentLossProtectionPaid: util.IntStr(withdraws.ImpermanentLossProtection),
		AddLiquidityCount:             util.IntStr(deposits.Count),
		WithdrawCount:                 util.IntStr(withdraws.Count),
	})
	return nil
}

func cachedJsonStats() httprouter.Handle {
	cachedHandler := CreateAndRegisterCache(calculateJsonStats, "stats")
	return cachedHandler.ServeHTTP
}

func jsonActions(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	urlParams := r.URL.Query()
	params := timeseries.ActionsParams{
		Limit:      util.ConsumeUrlParam(&urlParams, "limit"),
		Offset:     util.ConsumeUrlParam(&urlParams, "offset"),
		ActionType: util.ConsumeUrlParam(&urlParams, "type"),
		Address:    util.ConsumeUrlParam(&urlParams, "address"),
		TXId:       util.ConsumeUrlParam(&urlParams, "txid"),
		Asset:      util.ConsumeUrlParam(&urlParams, "asset"),
		Affiliate:  util.ConsumeUrlParam(&urlParams, "affiliate"),
	}

	merr := util.CheckUrlEmpty(urlParams)
	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	var actions oapigen.ActionsResponse
	var err error
	for _, addr := range withLowered(params.Address) {
		params.Address = addr
		actions, err = timeseries.GetActions(r.Context(), time.Time{}, params)
		if err != nil {
			respError(w, err)
			return
		}
		if len(actions.Actions) != 0 {
			break
		}
	}

	respJSON(w, actions)
}

func jsonBalance(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	urlParams := r.URL.Query()

	height := util.ConsumeUrlParam(&urlParams, "height")
	timestamp := util.ConsumeUrlParam(&urlParams, "timestamp")

	if merr := util.CheckUrlEmpty(urlParams); merr != nil {
		merr.ReportHTTP(w)
		return
	}

	address := ps[0].Value
	result, merr := timeseries.GetBalances(r.Context(), address, height, timestamp)

	if merr != nil {
		merr.ReportHTTP(w)
		return
	}

	respJSON(w, result)
}

func jsonSwagger(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	swagger, err := oapigen.GetSwagger()
	if err != nil {
		respError(w, err)
		return
	}
	respJSON(w, swagger)
}

func writeJSON(w io.Writer, body interface{}) {
	e := json.NewEncoder(w)
	e.SetIndent("", "\t")
	// Error discarded
	_ = e.Encode(body)
}

func respJSON(w http.ResponseWriter, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, body)
}

func respError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func ratioStr(a, b int64) string {
	if b == 0 {
		return "0"
	} else {
		return strconv.FormatFloat(float64(a)/float64(b), 'f', -1, 64)
	}
}

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// returns max 2 results
func withLowered(s string) []string {
	lower := strings.ToLower(s)
	if lower != s {
		return []string{s, lower}
	} else {
		return []string{s}
	}
}

func parsePeriodParam(urlParams *url.Values) (db.Buckets, error) {
	period := util.ConsumeUrlParam(urlParams, "period")
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
	case "100d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 100*24*60*60, now}}
	case "180d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 180*24*60*60, now}}
	case "365d":
		buckets = db.Buckets{Timestamps: db.Seconds{now - 365*24*60*60, now}}
	case "all":
		buckets = db.AllHistoryBuckets()
	default:
		return db.Buckets{}, fmt.Errorf(
			"invalid `period` param: %s. Accepted values:  1h, 24h, 7d, 30d, 90d, 100d, 180d, 365d, all",
			period)
	}

	return buckets, nil
}
