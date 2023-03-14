package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

type PoolDepthBucket struct {
	Window        db.Window
	Depths        timeseries.PoolDepths
	AssetPriceUSD float64
}

type TVLDepthBucket struct {
	Window            db.Window
	TotalPoolDepth    int64
	RunePriceUSD      float64
	PoolsMapRuneDepth map[string]int64
}

// - Queries database, possibly multiple rows per window.
// - Calls scanNext for each row. scanNext should store the values outside.
// - Calls applyLastScanned to save the scanned value for the current bucket.
// - Calls saveBucket for each bucket.
func queryBucketedGeneral(
	ctx context.Context, buckets db.Buckets,
	scan func(*sql.Rows) (db.Second, error),
	applyLastScanned func(),
	saveBucket func(idx int, bucketWindow db.Window),
	q string, qargs ...interface{}) error {
	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var nextDBTimestamp db.Second

	for i := 0; i < buckets.Count(); i++ {
		bucketWindow := buckets.BucketWindow(i)

		if nextDBTimestamp == bucketWindow.From {
			applyLastScanned()
		}

		for nextDBTimestamp <= bucketWindow.From {
			if rows.Next() {
				nextDBTimestamp, err = scan(rows)
				if err != nil {
					return err
				}
				if nextDBTimestamp == bucketWindow.From {
					applyLastScanned()
				}
			} else {
				// There were no more depths, all following buckets will
				// repeat the previous values
				nextDBTimestamp = buckets.End() + 1
			}
		}
		saveBucket(i, bucketWindow)
	}

	return nil
}

func addUsdPools(pool string) []string {
	usdPoolWhitelist := config.Global.UsdPools
	allPools := make([]string, 0, len(usdPoolWhitelist)+1)
	allPools = append(allPools, pool)
	allPools = append(allPools, usdPoolWhitelist...)
	return allPools
}

var poolDepthsAggregate = db.RegisterAggregate(
	db.NewAggregate("pool_depths", "block_pool_depths").
		AddGroupColumn("pool").
		AddLastColumn("asset_e8").
		AddLastColumn("rune_e8").
		AddLastColumn("synth_e8"))

func getDepthsHistory(ctx context.Context, buckets db.Buckets, pools []string,
	saveDepths func(idx int, bucketWindow db.Window, depths timeseries.DepthMap)) (beforeDepthMap timeseries.DepthMap, err error) {
	var poolDepths timeseries.DepthMap
	beforeDepthMap = timeseries.DepthMap{}

	// last rune and asset depths before the first bucket
	poolDepths, err = DepthsBefore(ctx, pools, buckets.Timestamps[0].ToNano())
	if err != nil {
		return nil, err
	}

	//deep copy the struct, otherwise the value will be overwritten with the last value
	for k, v := range poolDepths {
		beforeDepthMap[k] = v
	}

	if buckets.OneInterval() {
		// We only interested in the state at the end of the single interval:
		poolDepths, err = DepthsBefore(ctx, pools, buckets.Timestamps[1].ToNano())
		if err != nil {
			return nil, err
		}
		saveDepths(0, buckets.BucketWindow(0), poolDepths)
		return beforeDepthMap, nil
	}

	poolFilter := ""
	qargs := []interface{}{buckets.Start().ToNano(), buckets.End().ToNano()}
	if pools != nil {
		poolFilter = "pool = ANY($3)"
		qargs = []interface{}{buckets.Start().ToNano(), buckets.End().ToNano(), pools}
	}

	q := `
		SELECT
			pool,
			asset_e8,
			rune_e8,
			synth_e8,
			aggregate_timestamp / 1000000000 AS truncated
		FROM midgard_agg.pool_depths_` + buckets.AggregateName() + `
		` + db.Where("$1 <= aggregate_timestamp", "aggregate_timestamp < $2", poolFilter) + `
		ORDER BY aggregate_timestamp ASC
	`

	var next struct {
		pool   string
		depths timeseries.PoolDepths
	}

	readNext := func(rows *sql.Rows) (nextTimestamp db.Second, err error) {
		err = rows.Scan(&next.pool, &next.depths.AssetDepth, &next.depths.RuneDepth, &next.depths.SynthDepth, &nextTimestamp)
		if err != nil {
			return 0, err
		}
		return
	}
	applyNext := func() {
		poolDepths[next.pool] = next.depths
	}
	saveBucket := func(idx int, bucketWindow db.Window) {
		saveDepths(idx, bucketWindow, poolDepths)
	}

	return beforeDepthMap, queryBucketedGeneral(ctx, buckets, readNext, applyNext, saveBucket, q, qargs...)
}

// Each bucket contains the latest depths before the timestamp.
// Returns dense results (i.e. not sparse).
func PoolDepthHistory(ctx context.Context, buckets db.Buckets, pool string) (
	beforeDepth timeseries.PoolDepths, ret []PoolDepthBucket, err error) {
	allPools := addUsdPools(pool)
	ret = make([]PoolDepthBucket, buckets.Count())

	saveDepths := func(idx int, bucketWindow db.Window, poolDepths timeseries.DepthMap) {
		runePriceUSD := runePriceUSDForDepths(poolDepths)
		depths := poolDepths[pool]

		ret[idx].Window = bucketWindow
		ret[idx].Depths = depths
		ret[idx].AssetPriceUSD = depths.AssetPrice() * runePriceUSD
	}

	beforeDepthMap, err := getDepthsHistory(ctx, buckets, allPools, saveDepths)
	return beforeDepthMap[pool], ret, err
}

func TVLDepthHistory(ctx context.Context, buckets db.Buckets) (
	ret []TVLDepthBucket, err error) {
	ret = make([]TVLDepthBucket, buckets.Count())

	saveDepths := func(idx int, bucketWindow db.Window, poolDepths timeseries.DepthMap) {
		runePriceUSD := runePriceUSDForDepths(poolDepths)

		var depth int64 = 0
		ret[idx].PoolsMapRuneDepth = map[string]int64{}
		for poolName, pair := range poolDepths {
			// exclude derived asset pools, e.g. THOR.BTC
			if record.GetCoinType([]byte(poolName)) != record.AssetDerived {
				depth += pair.RuneDepth
				if record.GetCoinType([]byte(poolName)) == record.AssetNative {
					ret[idx].PoolsMapRuneDepth[poolName] = pair.RuneDepth
				}
			}
		}

		ret[idx].Window = bucketWindow
		ret[idx].TotalPoolDepth = depth
		ret[idx].RunePriceUSD = runePriceUSD
	}

	_, err = getDepthsHistory(ctx, buckets, nil, saveDepths)
	return ret, err
}

type USDPriceBucket struct {
	Window       db.Window
	RunePriceUSD float64
}

// Each bucket contains the latest depths before the timestamp.
// Returns dense results (i.e. not sparse).
func USDPriceHistory(ctx context.Context, buckets db.Buckets) (
	ret []USDPriceBucket, err error) {

	usdPoolWhitelist := config.Global.UsdPools
	if len(usdPoolWhitelist) == 0 {
		return nil, miderr.InternalErr("No USD pools defined")
	}

	ret = make([]USDPriceBucket, buckets.Count())

	saveDepths := func(idx int, bucketWindow db.Window, poolDepths timeseries.DepthMap) {
		ret[idx].Window = bucketWindow
		ret[idx].RunePriceUSD = runePriceUSDForDepths(poolDepths)
	}

	_, err = getDepthsHistory(ctx, buckets, usdPoolWhitelist, saveDepths)
	return ret, err
}

func DepthsBefore(ctx context.Context, pools []string, time db.Nano) (
	ret timeseries.DepthMap, err error) {

	// TODO(huginn): optimize, this call takes 1.8s if called from /v2/history/tvl
	// defer timer.Console("DepthBefore")()

	whereConditions := []string{}
	qargs := []interface{}{}
	if pools != nil {
		whereConditions = append(whereConditions, "pool = ANY($1)")
		qargs = append(qargs, pools)
	}

	subQuery, qargs := poolDepthsAggregate.UnionQuery(0, time, whereConditions, qargs)

	firstValueQuery := `
		SELECT
			pool,
			last(asset_e8, aggregate_timestamp) AS asset_e8,
			last(rune_e8, aggregate_timestamp) AS rune_e8,
			last(synth_e8, aggregate_timestamp) AS synth_e8
		FROM ` + subQuery + ` AS u
		GROUP BY pool
	`

	rows, err := db.Query(ctx, firstValueQuery, qargs...)
	if err != nil {
		return
	}
	defer rows.Close()

	ret = timeseries.DepthMap{}
	for rows.Next() {
		var pool string
		var depths timeseries.PoolDepths
		err = rows.Scan(&pool, &depths.AssetDepth, &depths.RuneDepth, &depths.SynthDepth)
		if err != nil {
			return
		}

		ret[pool] = depths
	}
	for _, pool := range pools {
		_, present := ret[pool]
		if !present {
			ret[pool] = timeseries.PoolDepths{}
		}
	}
	return
}
