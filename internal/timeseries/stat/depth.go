package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

type PoolDepthBucket struct {
	Window        db.Window
	Depths        timeseries.DepthPair
	AssetPriceUSD float64
}

type TVLDepthBucket struct {
	Window         db.Window
	TotalPoolDepth int64
	RunePriceUSD   float64
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
	allPools := make([]string, 0, len(usdPoolWhitelist)+1)
	allPools = append(allPools, pool)
	allPools = append(allPools, usdPoolWhitelist...)
	return allPools
}

func init() {
	db.RegisterAggregate(
		"pool_depths",
		`SELECT
			pool,
			last(asset_e8, block_timestamp) as asset_e8,
			last(rune_e8, block_timestamp) as rune_e8,
			%s as bucket_start
		FROM block_pool_depths
		GROUP BY bucket_start, pool`,
		`SELECT
			pool,
			last(asset_e8, d.bucket_start) as asset_e8,
			last(rune_e8, d.bucket_start) as rune_e8,
		%s as bucket_start
		FROM midgard_agg.pool_depths_day d
		GROUP BY bucket_start, pool`,
	)
}

func getDepthsHistory(ctx context.Context, buckets db.Buckets, pools []string,
	saveDepths func(idx int, bucketWindow db.Window, depths timeseries.DepthMap)) (err error) {

	var poolDepths timeseries.DepthMap
	if buckets.OneInterval() {
		// We only interested in the state at the end of the single interval:
		poolDepths, err = depthBefore(ctx, pools, buckets.Timestamps[1].ToNano())
		if err != nil {
			return err
		}
		saveDepths(0, buckets.BucketWindow(0), poolDepths)
		return
	}

	// last rune and asset depths before the first bucket
	poolDepths, err = depthBefore(ctx, pools, buckets.Timestamps[0].ToNano())
	if err != nil {
		return err
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
			bucket_start / 1000000000 AS truncated
		FROM midgard_agg.pool_depths_` + buckets.AggregateName() + `
		` + db.Where("$1 <= bucket_start", "bucket_start < $2", poolFilter) + `
		ORDER BY bucket_start ASC
	`

	var next struct {
		pool   string
		depths timeseries.DepthPair
	}

	readNext := func(rows *sql.Rows) (nextTimestamp db.Second, err error) {
		err = rows.Scan(&next.pool, &next.depths.AssetDepth, &next.depths.RuneDepth, &nextTimestamp)
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

	return queryBucketedGeneral(ctx, buckets, readNext, applyNext, saveBucket, q, qargs...)
}

// Each bucket contains the latest depths before the timestamp.
// Returns dense results (i.e. not sparse).
func PoolDepthHistory(ctx context.Context, buckets db.Buckets, pool string) (
	ret []PoolDepthBucket, err error) {
	allPools := addUsdPools(pool)
	ret = make([]PoolDepthBucket, buckets.Count())

	saveDepths := func(idx int, bucketWindow db.Window, poolDepths timeseries.DepthMap) {
		runePriceUSD := runePriceUSDForDepths(poolDepths)
		depths := poolDepths[pool]

		ret[idx].Window = bucketWindow
		ret[idx].Depths = depths
		ret[idx].AssetPriceUSD = depths.AssetPrice() * runePriceUSD
	}

	err = getDepthsHistory(ctx, buckets, allPools, saveDepths)
	return ret, err
}

func TVLDepthHistory(ctx context.Context, buckets db.Buckets) (
	ret []TVLDepthBucket, err error) {
	ret = make([]TVLDepthBucket, buckets.Count())

	saveDepths := func(idx int, bucketWindow db.Window, poolDepths timeseries.DepthMap) {
		runePriceUSD := runePriceUSDForDepths(poolDepths)
		var depth int64 = 0
		for _, pair := range poolDepths {
			depth += pair.RuneDepth
		}

		ret[idx].Window = bucketWindow
		ret[idx].TotalPoolDepth = depth
		ret[idx].RunePriceUSD = runePriceUSD
	}

	err = getDepthsHistory(ctx, buckets, nil, saveDepths)
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
	if len(usdPoolWhitelist) == 0 {
		return nil, miderr.InternalErr("No USD pools defined")
	}

	ret = make([]USDPriceBucket, buckets.Count())

	saveDepths := func(idx int, bucketWindow db.Window, poolDepths timeseries.DepthMap) {
		ret[idx].Window = bucketWindow
		ret[idx].RunePriceUSD = runePriceUSDForDepths(poolDepths)
	}

	err = getDepthsHistory(ctx, buckets, usdPoolWhitelist, saveDepths)
	return ret, err
}

func depthBefore(ctx context.Context, pools []string, time db.Nano) (
	ret timeseries.DepthMap, err error) {
	poolFilter := ""
	qargs := []interface{}{time}
	if pools != nil {
		poolFilter = "pool = ANY($2)"
		qargs = []interface{}{time, pools}
	}

	firstValueQuery := `
		SELECT
			pool,
			last(asset_e8, ts) AS asset_e8,
			last(rune_e8, ts) AS rune_e8
		FROM (
			(SELECT
				pool,
				last(asset_e8, bucket_start) AS asset_e8,
				last(rune_e8, bucket_start) AS rune_e8,
				MAX(bucket_start) as ts
			FROM midgard_agg.pool_depths_hour
			` + db.Where("bucket_start < time_bucket('3600000000000' :: BIGINT, $1)", poolFilter) + `
			GROUP BY pool)
		UNION
			(SELECT
				pool,
				last(asset_e8, block_timestamp) AS asset_e8,
				last(rune_e8, block_timestamp) AS rune_e8,
				MAX(block_timestamp) as ts
			FROM block_pool_depths
			` + db.Where("time_bucket('3600000000000' :: BIGINT, $1) <= block_timestamp", "block_timestamp < $1", poolFilter) + `
			GROUP BY pool)
		) AS u
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
		var depths timeseries.DepthPair
		err = rows.Scan(&pool, &depths.AssetDepth, &depths.RuneDepth)
		if err != nil {
			return
		}

		ret[pool] = depths
	}
	for _, pool := range pools {
		_, present := ret[pool]
		if !present {
			ret[pool] = timeseries.DepthPair{}
		}
	}
	return
}
