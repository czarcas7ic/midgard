package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

func totalUnitChanges(ctx context.Context, pools []string, tableName string, until *db.Nano) (
	map[string]int64, error) {
	timeFilter := ""
	qargs := []interface{}{pools}
	if until != nil {
		timeFilter = "block_timestamp < $2"
		qargs = append(qargs, *until)
	}

	q := `
		SELECT
			pool,
			COALESCE(SUM(stake_units), 0) as units
		FROM ` + tableName + `
		` + db.Where(timeFilter, "pool = ANY($1)") + `
		GROUP BY pool
	`

	poolsUnits := make(map[string]int64)
	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pool string
		var units int64
		err := rows.Scan(&pool, &units)
		if err != nil {
			return nil, err
		}
		poolsUnits[pool] = units
	}

	return poolsUnits, nil
}

// Units is the total units at the end of the period.
type UnitsBucket struct {
	Window db.Window
	Units  int64
}

func bucketedUnitChanges(ctx context.Context, buckets db.Buckets, pool string, tableName string) (
	beforeUnit int64, ret []UnitsBucket, err error) {
	startTime := buckets.Window().From.ToNano()
	lastValueMap, err := totalUnitChanges(ctx, []string{pool}, tableName, &startTime)
	if err != nil {
		return 0, nil, err
	}
	lastValue := lastValueMap[pool]
	beforeUnit = lastValue
	q := `
		SELECT
			COALESCE(SUM(stake_units), 0) as units,
			` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated
		FROM ` + tableName + `
		WHERE pool = $1 AND $2 <= block_timestamp AND block_timestamp < $3
		GROUP BY truncated
		ORDER BY truncated ASC
	`

	qargs := []interface{}{pool, buckets.Start().ToNano(), buckets.End().ToNano()}

	ret = make([]UnitsBucket, buckets.Count())
	var nextValue int64

	readNext := func(rows *sql.Rows) (nextTimestamp db.Second, err error) {
		err = rows.Scan(&nextValue, &nextTimestamp)
		if err != nil {
			return 0, err
		}
		return
	}
	nextIsCurrent := func() { lastValue += nextValue }
	saveBucket := func(idx int, bucketWindow db.Window) {
		ret[idx].Window = bucketWindow
		ret[idx].Units = lastValue
	}

	err = queryBucketedGeneral(ctx, buckets, readNext, nextIsCurrent, saveBucket, q, qargs...)
	if err != nil {
		return 0, nil, err
	}

	return beforeUnit, ret, nil
}

// Not including the until timestamp
func PoolsLiquidityUnitsBefore(ctx context.Context, pools []string, until *db.Nano) (
	map[string]int64, error) {
	stakes, err := totalUnitChanges(ctx, pools, "stake_events", until)
	if err != nil {
		return nil, err
	}
	withdraws, err := totalUnitChanges(ctx, pools, "withdraw_events", until)
	if err != nil {
		return nil, err
	}
	for k, v := range withdraws {
		stakes[k] -= v
	}
	return stakes, nil
}

// PoolUnits gets net liquidity units in pools
func CurrentPoolsLiquidityUnits(ctx context.Context, pools []string) (map[string]int64, error) {
	return PoolsLiquidityUnitsBefore(ctx, pools, nil)
}

// PoolUnits gets net liquidity units in pools
func PoolLiquidityUnitsHistory(ctx context.Context, buckets db.Buckets, pool string) (int64, []UnitsBucket, error) {
	beforeUnitStake, ret, err := bucketedUnitChanges(ctx, buckets, pool, "stake_events")
	if err != nil {
		return 0, nil, err
	}
	beforeUnitWithdraw, withdraws, err := bucketedUnitChanges(ctx, buckets, pool, "withdraw_events")
	if err != nil {
		return 0, nil, err
	}
	if len(ret) != len(withdraws) {
		return 0, nil, miderr.InternalErr("bucket count is different for deposits and withdraws")
	}
	for i := range ret {
		ret[i].Units -= withdraws[i].Units
	}
	beforeUnit := beforeUnitStake - beforeUnitWithdraw
	return beforeUnit, ret, nil
}
