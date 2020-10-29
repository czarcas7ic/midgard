package stat

import (
	"context"
	"errors"
	"fmt"
	"gitlab.com/thorchain/midgard/internal/graphql/model"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"strconv"
	"time"
)

// Swaps are generic swap statistics.
type Swaps struct {
	TxCount       int64
	RuneAddrCount int64 // Number of unique addresses involved.
	RuneE8Total   int64
}

func SwapsFromRuneLookup(ctx context.Context, w Window) (*Swaps, error) {
	const q = `SELECT COALESCE(COUNT(*), 0), COALESCE(COUNT(DISTINCT(from_addr)), 0), COALESCE(SUM(from_E8), 0)
        FROM swap_events
        WHERE pool = from_asset AND block_timestamp >= $1 AND block_timestamp <= $2`

	return querySwaps(ctx, q, w.Since.UnixNano(), w.Until.UnixNano())
}

func SwapsToRuneLookup(ctx context.Context, w Window) (*Swaps, error) {
	const q = `SELECT COALESCE(COUNT(*), 0), COALESCE(COUNT(DISTINCT(swap.from_addr)), 0), COALESCE(SUM(out.asset_E8), 0)
        FROM swap_events swap
	JOIN outbound_events out ON
		/* limit comparison set—no indinces */
		swap.block_timestamp <= out.block_timestamp AND
		swap.block_timestamp + 36000000000000 >= out.block_timestamp AND
		swap.tx = out.in_tx
        WHERE swap.block_timestamp >= $1 AND swap.block_timestamp <= $2 AND swap.pool <> swap.from_asset`

	return querySwaps(ctx, q, w.Since.UnixNano(), w.Until.UnixNano())
}

func querySwaps(ctx context.Context, q string, args ...interface{}) (*Swaps, error) {
	rows, err := DBQuery(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var swaps Swaps
	if rows.Next() {
		err := rows.Scan(&swaps.TxCount, &swaps.RuneAddrCount, &swaps.RuneE8Total)
		if err != nil {
			return nil, err
		}
	}
	return &swaps, rows.Err()
}

// PoolSwaps are swap statistics for a specific asset.
// todo(donfrigo) remove unnecessary fields in order to use ToRune and FromRune instead
type PoolSwaps struct {
	TruncatedTime       time.Time
	TxCount             int64
	AssetE8Total        int64
	RuneE8Total         int64
	LiqFeeE8Total       int64
	LiqFeeInRuneE8Total int64
	TradeSlipBPTotal    int64
	ToRune              model.VolumeStats
	FromRune            model.VolumeStats
}

func PoolSwapsFromRuneLookup(ctx context.Context, pool string, w Window) (*PoolSwaps, error) {
	const q = `SELECT COALESCE(COUNT(*), 0), 0, COALESCE(SUM(from_E8), 0), COALESCE(SUM(liq_fee_E8), 0), COALESCE(SUM(liq_fee_in_rune_E8), 0), COALESCE(SUM(trade_slip_BP), 0), COALESCE(MIN(block_timestamp), 0), COALESCE(MAX(block_timestamp), 0), to_timestamp(0)
	FROM swap_events
	WHERE pool = $1 AND from_asset <> $1 AND block_timestamp >= $2 AND block_timestamp < $3`

	var swaps [1]PoolSwaps
	_, err := appendPoolSwaps(ctx, swaps[:0], q, false, pool, w.Since.UnixNano(), w.Until.UnixNano())
	if err != nil || len(swaps) == 0 {
		return nil, err
	}
	return &swaps[0], nil
}

func PoolSwapsToRuneLookup(ctx context.Context, pool string, w Window) (*PoolSwaps, error) {
	const q = `SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(from_E8), 0), 0, COALESCE(SUM(liq_fee_E8), 0), COALESCE(SUM(liq_fee_in_rune_E8), 0), COALESCE(SUM(trade_slip_BP), 0), COALESCE(MIN(block_timestamp), 0), COALESCE(MAX(block_timestamp), 0), to_timestamp(0)
	FROM swap_events
	WHERE pool = $1 AND from_asset = $1 AND block_timestamp >= $2 AND block_timestamp < $3`

	var swaps [1]PoolSwaps
	_, err := appendPoolSwaps(ctx, swaps[:0], q, false, pool, w.Since.UnixNano(), w.Until.UnixNano())
	if err != nil || len(swaps) == 0 {
		return nil, err
	}
	return &swaps[0], nil
}

func PoolSwapsFromRuneBucketsLookup(ctx context.Context, pool string, bucketSize time.Duration, w Window) ([]PoolSwaps, error) {
	n, err := bucketsFor(bucketSize, w)
	if err != nil {
		return nil, err
	}
	a := make([]PoolSwaps, 0, n)

	const q = `SELECT COALESCE(COUNT(*), 0), 0, COALESCE(SUM(from_E8), 0), COALESCE(SUM(liq_fee_E8), 0), COALESCE(SUM(liq_fee_in_rune_E8), 0), COALESCE(SUM(trade_slip_BP), 0), COALESCE(MIN(block_timestamp), 0), COALESCE(MAX(block_timestamp), 0), to_timestamp(0)
	FROM swap_events
	WHERE pool = $1 AND from_asset <> $1 AND block_timestamp >= $2 AND block_timestamp < $3
	GROUP BY time_bucket($4, block_timestamp)
	ORDER BY time_bucket($4, block_timestamp)`

	return appendPoolSwaps(ctx, a, q, false, pool, w.Since.UnixNano(), w.Until.UnixNano(), bucketSize)
}

func PoolSwapsToRuneBucketsLookup(ctx context.Context, pool string, bucketSize time.Duration, w Window) ([]PoolSwaps, error) {
	n, err := bucketsFor(bucketSize, w)
	if err != nil {
		return nil, err
	}
	a := make([]PoolSwaps, 0, n)

	const q = `SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(from_E8), 0), 0, COALESCE(SUM(liq_fee_E8), 0), COALESCE(SUM(liq_fee_in_rune_E8), 0), COALESCE(SUM(trade_slip_BP), 0), COALESCE(MIN(block_timestamp), 0), COALESCE(MAX(block_timestamp), 0), to_timestamp(0)
	FROM swap_events
	WHERE pool = $1 AND from_asset = $1 AND block_timestamp >= $2 AND block_timestamp < $3
	GROUP BY time_bucket($4, block_timestamp)
	ORDER BY time_bucket($4, block_timestamp)`

	return appendPoolSwaps(ctx, a, q, false, pool, w.Since.UnixNano(), w.Until.UnixNano(), bucketSize)
}

// GetIntervalFromString converts string to PoolVolumeInterval.
func GetIntervalFromString(str string) (model.PoolVolumeInterval, error) {
	switch str {
	case "5min":
		return model.PoolVolumeIntervalMinute5, nil
	case "hour":
		return model.PoolVolumeIntervalHour, nil
	case "day":
		return model.PoolVolumeIntervalDay, nil
	case "week":
		return model.PoolVolumeIntervalWeek, nil
	case "month":
		return model.PoolVolumeIntervalMonth, nil
	case "quarter":
		return model.PoolVolumeIntervalQuarter, nil
	case "year":
		return model.PoolVolumeIntervalYear, nil
	}
	return "", errors.New("the requested interval is invalid: " + str)
}

// GetDurationFromInterval returns the the limited duration for given interval (duration = interval * limit)
func getDurationFromInterval(inv model.PoolVolumeInterval) (time.Duration, error) {
	switch inv {
	case model.PoolVolumeIntervalMinute5:
		return time.Minute * 5 * 101, nil
	case model.PoolVolumeIntervalHour:
		return time.Hour * 101, nil
	case model.PoolVolumeIntervalDay:
		return time.Hour * 24 * 31, nil
	case model.PoolVolumeIntervalWeek:
		return time.Hour * 24 * 7 * 6, nil
	case model.PoolVolumeIntervalMonth:
		return time.Hour * 24 * 31 * 3, nil
	case model.PoolVolumeIntervalQuarter:
		return time.Hour * 24 * 122 * 3, nil
	case model.PoolVolumeIntervalYear:
		return time.Hour * 24 * 365 * 3, nil
	}
	return time.Duration(0), errors.New(string("the requested interval is invalid: " + inv))
}

// Function that converts interval to a string necessary for the gapfill functionality in the SQL query
// 300E9 stands for 300*10^9 -> 5 minutes in nanoseconds and same logic for the rest of the entries
func getGapfillFromLimit(inv model.PoolVolumeInterval) (string, error) {
	switch inv {
	case model.PoolVolumeIntervalMinute5:
		return "300E9::BIGINT", nil
	case model.PoolVolumeIntervalHour:
		return "3600E9::BIGINT", nil
	case model.PoolVolumeIntervalDay:
		return "864E11::BIGINT", nil
	case model.PoolVolumeIntervalWeek:
		return "604800E9::BIGINT", nil
	case model.PoolVolumeIntervalMonth:
		return "604800E9::BIGINT", nil
	case model.PoolVolumeIntervalQuarter:
		return "604800E9::BIGINT", nil
	case model.PoolVolumeIntervalYear:
		return "604800E9::BIGINT", nil
	}
	return "", errors.New(string("the requested interval is invalid: " + inv))
}

// Function to get asset volumes from all (*) or  given pool, for given asset with given interval
func PoolSwapsLookup(ctx context.Context, pool string, interval model.PoolVolumeInterval, w Window, swapToRune bool) ([]PoolSwaps, error) {
	var q, poolQuery string
	if pool != "*" {
		poolQuery = fmt.Sprintf(`swap.pool = '%s' AND`, pool)
	}
	w, err := calcBounds(w, interval)
	if err != nil {
		return nil, err
	}
	gapfill, err := getGapfillFromLimit(interval)
	if err != nil {
		return nil, err
	}

	// If conversion is true then it assumes that the query selects to the flowing fields in addition: TruncatedTime, volumeInRune
	if swapToRune {
		q = fmt.Sprintf(`SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(from_E8), 0),  
			COALESCE(CAST(SUM(CAST(rune_e8 as NUMERIC) / CAST(asset_e8 as NUMERIC) * swap.from_e8) as bigint), 0) as rune_volume, 
			COALESCE(SUM(liq_fee_E8), 0), COALESCE(SUM(liq_fee_in_rune_E8), 0), COALESCE(SUM(trade_slip_BP), 0), 
			COALESCE(MIN(swap.block_timestamp), 0), COALESCE(MAX(swap.block_timestamp), 0), 
			time_bucket('5 min',date_trunc($3, to_timestamp(swap.block_timestamp/1000000000))) AS bucket
			
			FROM swap_events AS swap
			LEFT JOIN LATERAL (
				SELECT depths.asset_e8, depths.rune_e8
					FROM block_pool_depths as depths
				WHERE
				depths.block_timestamp <= swap.block_timestamp AND swap.pool = depths.pool
				ORDER  BY depths.block_timestamp DESC
				LIMIT  1
			) AS joined on TRUE
			WHERE %s swap.from_asset = swap.pool AND swap.block_timestamp >= $1 AND swap.block_timestamp <= $2
			GROUP BY bucket
			ORDER BY bucket ASC`, poolQuery)
	} else {
		q = fmt.Sprintf(`WITH gapfill AS (
    		SELECT COALESCE(COUNT(*), 0) as count, COALESCE(SUM(from_E8), 0) as from_E8, COALESCE(SUM(liq_fee_E8), 0) as liq_fee_E8, 
			COALESCE(SUM(liq_fee_in_rune_E8), 0) as liq_fee_in_rune_E8, COALESCE(SUM(trade_slip_BP), 0) as trade_slip_BP, 
			COALESCE(MIN(swap.block_timestamp), 0) as min, COALESCE(MAX(swap.block_timestamp), 0) as max, 
			time_bucket_gapfill(%s, block_timestamp) as bucket

			FROM swap_events as swap
    		WHERE %s from_asset <> pool AND block_timestamp >= $1 AND block_timestamp < $2
    		GROUP BY bucket)

			SELECT SUM(count), 0, SUM(from_E8), SUM(liq_fee_E8), SUM(liq_fee_in_rune_E8), SUM(trade_slip_BP), COALESCE(MIN(min), 0), COALESCE(MAX(max), 0), date_trunc($3, to_timestamp(bucket/1000000000)) as truncated
			FROM gapfill
			GROUP BY truncated
			ORDER BY truncated ASC`, gapfill, poolQuery)
	}

	return appendPoolSwaps(ctx, []PoolSwaps{}, q, swapToRune, w.Since.UnixNano(), w.Until.UnixNano(), interval)
}

func calcBounds(w Window, inv model.PoolVolumeInterval) (Window, error) {
	duration, err := getDurationFromInterval(inv)
	if err != nil {
		return Window{}, err
	}

	if w.Since.Unix() != 0 && w.Until.Unix() == 0 {
		// if only since is defined
		limitedTime := w.Since.Add(duration)
		w.Until = limitedTime
	} else if w.Since.Unix() == 0 && w.Until.Unix() != 0 {
		// if only until is defined
		limitedTime := w.Until.Add(-duration)
		w.Since = limitedTime
	} else if w.Since.Unix() == 0 && w.Until.Unix() == 0 {
		// if neither is defined
		w.Until = time.Now()
	}

	// if the starting time lies outside the limit
	limitedTime := w.Until.Add(-duration)
	if limitedTime.After(w.Since) {
		// limit the value
		w.Since = limitedTime
	}

	return w, nil
}

func appendPoolSwaps(ctx context.Context, swaps []PoolSwaps, q string, swapToRune bool, args ...interface{}) ([]PoolSwaps, error) {
	rows, err := DBQuery(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r PoolSwaps
		var first, last int64
		if swapToRune {
			if err := rows.Scan(&r.TxCount, &r.AssetE8Total, &r.RuneE8Total, &r.LiqFeeE8Total, &r.LiqFeeInRuneE8Total, &r.TradeSlipBPTotal, &first, &last, &r.TruncatedTime); err != nil {
				return swaps, err
			}
			r.ToRune = model.VolumeStats{
				Count:        r.TxCount,
				VolumeInRune: r.RuneE8Total,
				FeesInRune:   r.LiqFeeInRuneE8Total,
			}
		} else {
			if err := rows.Scan(&r.TxCount, &r.AssetE8Total, &r.RuneE8Total, &r.LiqFeeE8Total, &r.LiqFeeInRuneE8Total, &r.TradeSlipBPTotal, &first, &last, &r.TruncatedTime); err != nil {
				return swaps, err
			}
			r.FromRune = model.VolumeStats{
				Count:        r.TxCount,
				VolumeInRune: r.RuneE8Total,
				FeesInRune:   r.LiqFeeInRuneE8Total,
			}
		}
		swaps = append(swaps, r)
	}
	return swaps, rows.Err()
}

// struct returned from v1/history/total_volume endpoint
type SwapVolumeChanges struct {
	BuyVolume   string `json:"buyVolume"`   // volume RUNE bought in given a timeframe
	SellVolume  string `json:"sellVolume"`  // volume of RUNE sold in given a timeframe
	Time        int64  `json:"time"`        // beginning of the timeframe
	TotalVolume string `json:"totalVolume"` // sum of bought and sold volume
}

func TotalVolumeChanges(ctx context.Context, inv, pool string, from, to time.Time) ([]SwapVolumeChanges, error) {
	interval, err := GetIntervalFromString(inv)
	if err != nil {
		return nil, err
	}
	window := Window{
		Since: from,
		Until: to,
	}

	// fromRune stores conversion from Rune to Asset -> selling Rune
	fromRune, err := PoolSwapsLookup(ctx, pool, interval, window, false)
	if err != nil {
		return nil, err
	}

	// fromAsset stores conversion from Asset to Rune -> buying Rune
	fromAsset, err := PoolSwapsLookup(ctx, pool, interval, window, true)
	if err != nil {
		return nil, err
	}

	result, err := createSwapVolumeChanges(fromRune, fromAsset)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func createSwapVolumeChanges(fromRune, fromAsset []PoolSwaps) ([]SwapVolumeChanges, error) {
	result := make([]SwapVolumeChanges, 0)

	mergedPoolSwaps, err := MergeSwaps(fromRune, fromAsset)
	if err != nil {
		return nil, err
	}

	for _, ps := range mergedPoolSwaps {
		timestamp := ps.TruncatedTime.Unix()
		fr := ps.FromRune
		tr := ps.ToRune

		runeSellVolume := strconv.FormatInt(fr.VolumeInRune, 10)
		runeBuyVolume := strconv.FormatInt(tr.VolumeInRune, 10)
		totalVolume := strconv.FormatInt(fr.VolumeInRune+tr.VolumeInRune, 10)

		svc := SwapVolumeChanges{
			BuyVolume:   runeBuyVolume,
			SellVolume:  runeSellVolume,
			Time:        timestamp,
			TotalVolume: totalVolume,
		}

		result = append(result, svc)
	}
	return result, nil
}

func MergeSwaps(fromRune, fromAsset []PoolSwaps) ([]PoolSwaps, error) {
	result := make([]PoolSwaps, 0)

	if len(fromRune) == 0 {
		fromRune = append(fromRune, PoolSwaps{TruncatedTime: time.Now()})
	}

	if len(fromAsset) == 0 {
		fromAsset = append(fromAsset, PoolSwaps{TruncatedTime: time.Now()})
	}

	for i, j := 0, 0; i < len(fromRune) && j < len(fromAsset); {
		// selling Rune -> volume is already in Rune
		fr := fromRune[i]
		// buying Rune -> volume is calculated from asset volume and exchange rate
		fa := fromAsset[j]

		if fr.TruncatedTime.Before(fa.TruncatedTime) {
			result = append(result, fr)
			i++
		} else if fa.TruncatedTime.Before(fr.TruncatedTime) {
			result = append(result, fa)
			j++
		} else if fr.TruncatedTime.Equal(fa.TruncatedTime) {
			toRuneStats := model.VolumeStats{
				Count:        fa.ToRune.Count,
				VolumeInRune: fa.ToRune.VolumeInRune,
				FeesInRune:   fa.ToRune.FeesInRune,
			}

			fromRuneStats := model.VolumeStats{
				Count:        fr.FromRune.Count,
				VolumeInRune: fr.FromRune.VolumeInRune,
				FeesInRune:   fr.FromRune.FeesInRune,
			}

			ps := PoolSwaps{
				TruncatedTime: fr.TruncatedTime,
				FromRune:      fromRuneStats,
				ToRune:        toRuneStats,
			}

			result = append(result, ps)
			i++
			j++
		} else {
			return result, errors.New("error occurred while merging arrays")
		}
	}

	return result, nil
}

// PoolTotalVolume computes total volume amount for given timestamps (from/to) and pool
func PoolTotalVolume(ctx context.Context, pool string, from, to time.Time) (int64, error) {
	toRuneVolumeQ := `SELECT
		COALESCE(CAST(SUM(CAST(rune_e8 as NUMERIC) / CAST(asset_e8 as NUMERIC) * swap.from_e8) as bigint), 0)
		FROM swap_events AS swap
			LEFT JOIN LATERAL (
				SELECT depths.asset_e8, depths.rune_e8
					FROM block_pool_depths as depths
				WHERE
				depths.block_timestamp <= swap.block_timestamp AND swap.pool = depths.pool
				ORDER  BY depths.block_timestamp DESC
				LIMIT  1
			) AS joined on TRUE
		WHERE swap.from_asset = swap.pool AND swap.pool = $1 AND swap.block_timestamp >= $2 AND swap.block_timestamp <= $3
	`
	var toRuneVolume int64
	err := timeseries.QueryOneValue(&toRuneVolume, ctx, toRuneVolumeQ, pool, from.UnixNano(), to.UnixNano())
	if err != nil {
		return 0, err
	}

	fromRuneVolumeQ := `SELECT COALESCE(SUM(from_e8), 0)
	FROM swap_events
	WHERE from_asset <> pool AND pool = $1 AND block_timestamp >= $2 AND block_timestamp <= $3
	`
	var fromRuneVolume int64
	err = timeseries.QueryOneValue(&fromRuneVolume, ctx, fromRuneVolumeQ, pool, from.UnixNano(), to.UnixNano())
	if err != nil {
		return 0, err
	}

	return toRuneVolume + fromRuneVolume, nil
}
