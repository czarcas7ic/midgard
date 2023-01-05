package stat

import (
	"context"
	"database/sql"

	"gitlab.com/thorchain/midgard/internal/db"
)

func membersCount(ctx context.Context, pools []string, until *db.Nano) (map[string]int64, error) {
	timeFilter := ""
	qargs := []interface{}{pools}
	if until != nil {
		timeFilter = "block_timestamp < $2"
		qargs = append(qargs, *until)
	}

	q := `
		SELECT
			DISTINCT on (pool) pool, count 
		FROM midgard_agg.members_count
		` + db.Where(timeFilter, "pool = ANY($1)") + `
		ORDER BY pool, block_timestamp desc
	`

	poolsCount := make(map[string]int64)
	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pool string
		var count int64
		err := rows.Scan(&pool, &count)
		if err != nil {
			return nil, err
		}
		poolsCount[pool] = count
	}

	return poolsCount, nil
}

type CountBucket struct {
	Window db.Window
	Count  int64
}

func GetMembersCountBucket(ctx context.Context, buckets db.Buckets, pool string) (
	beforeCount int64, ret []CountBucket, err error) {
	startTime := buckets.Window().From.ToNano()
	lastValueMap, err := membersCount(ctx, []string{pool}, &startTime)
	if err != nil {
		return 0, nil, err
	}

	lastCountValue := lastValueMap[pool]
	beforeCount = lastCountValue

	q := `
		WITH 
		truncate AS (
			SELECT
				pool,
				count,
				block_timestamp,
				event_id,
				` + db.SelectTruncatedTimestamp("block_timestamp", buckets) + ` AS truncated
			FROM midgard_agg.members_count
			ORDER BY truncated ASC
		)
		SELECT DISTINCT ON (truncated) truncated, count
		FROM truncate
		WHERE pool = $1 AND $2 <= block_timestamp AND block_timestamp < $3
		ORDER BY truncated, block_timestamp desc
	`

	qargs := []interface{}{pool, buckets.Start().ToNano(), buckets.End().ToNano()}

	ret = make([]CountBucket, buckets.Count())
	var nextValue int64

	readNext := func(rows *sql.Rows) (nextTimestamp db.Second, err error) {
		err = rows.Scan(&nextTimestamp, &nextValue)
		if err != nil {
			return 0, err
		}
		return
	}
	nextIsCurrent := func() { lastCountValue = nextValue }
	saveBucket := func(idx int, bucketWindow db.Window) {
		ret[idx].Window = bucketWindow
		ret[idx].Count = lastCountValue
	}

	err = queryBucketedGeneral(ctx, buckets, readNext, nextIsCurrent, saveBucket, q, qargs...)
	if err != nil {
		return 0, nil, err
	}

	return beforeCount, ret, nil

}
