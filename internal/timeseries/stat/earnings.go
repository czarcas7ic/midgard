package stat

import (
	"context"
	"strconv"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

type poolEarnings struct {
	pool                   string
	runeLiquidityFees      int64 // fees charged in RUNE
	assetLiquidityFees     int64 // fees charged in asset
	totalLiquidityFeesRune int64 // asset + RUNE fees in RUNE
	saverRewards           int64 // saver reward
	rewards                int64 // rewards sent to / extracted from pool each block
}

func (pe *poolEarnings) toOapigen() oapigen.EarningsHistoryItemPool {
	return oapigen.EarningsHistoryItemPool{
		Pool:                   pe.pool,
		RuneLiquidityFees:      util.IntStr(pe.runeLiquidityFees),
		AssetLiquidityFees:     util.IntStr(pe.assetLiquidityFees),
		TotalLiquidityFeesRune: util.IntStr(pe.totalLiquidityFeesRune),
		Rewards:                util.IntStr(pe.rewards),
		SaverEarning:           util.IntStr(pe.saverRewards),
		Earnings:               util.IntStr(pe.totalLiquidityFeesRune + pe.rewards),
	}
}

type poolEarningsMap map[string]*poolEarnings

func (peMap poolEarningsMap) getPoolEarnings(pool string) *poolEarnings {
	pe, _ := peMap[pool]
	if pe == nil {
		newPoolEarnings := poolEarnings{pool: pool}
		// Nil map means there were no entries for the bucket at all
		if peMap != nil {
			peMap[pool] = &newPoolEarnings
		}
		return &newPoolEarnings
	} else {
		return pe
	}
}

var RewardsAggregate = db.RegisterAggregate(db.NewAggregate("rewards_events", "rewards_events").
	AddBigintSumColumn("bond_e8"))

func GetEarningsHistory(ctx context.Context, buckets db.Buckets) (oapigen.EarningsHistoryResponse, error) {
	window := buckets.Window()
	timestamps := buckets.Timestamps[:len(buckets.Timestamps)-1]

	liquidityFeesByPoolQ, params := SwapsAggregate.BucketedQuery(`
		SELECT
			rune_fees_E8,
			asset_fees_E8,
			liq_fee_in_rune_E8,
			aggregate_timestamp/1000000000 AS start_time,
			pool
		FROM %s
	`, buckets, nil, nil)

	liquidityFeesByPoolRows, err := db.Query(ctx, liquidityFeesByPoolQ, params...)
	if err != nil {
		return oapigen.EarningsHistoryResponse{}, err
	}
	defer liquidityFeesByPoolRows.Close()

	// TODO(huginn): just use the basic bucketed query with nano timestamp
	bondingRewardsQ, params := RewardsAggregate.BucketedQuery(`
		SELECT
			bond_e8,
			aggregate_timestamp/1000000000 AS start_time
		FROM %s
	`, buckets, nil, nil)

	bondingRewardsRows, err := db.Query(ctx, bondingRewardsQ, params...)
	if err != nil {
		return oapigen.EarningsHistoryResponse{}, err
	}
	defer bondingRewardsRows.Close()

	// TODO(huginn): just use the basic bucketed query with nano timestamp and reorder columns
	poolRewardsQ, params := timeseries.RewardEntriesAggregate.BucketedQuery(`
		SELECT
			rune_e8,
			saver_e8,
			aggregate_timestamp/1000000000 AS start_time,
			pool
		FROM %s
	`, buckets, nil, nil)

	poolRewardsRows, err := db.Query(ctx, poolRewardsQ, params...)
	if err != nil {
		return oapigen.EarningsHistoryResponse{}, err
	}
	defer poolRewardsRows.Close()

	nodeStartCount, err := timeseries.ActiveNodeCount(ctx, window.From.ToNano())
	if err != nil {
		return oapigen.EarningsHistoryResponse{}, err
	}

	nodeDeltasQ := `
	SELECT
	SUM(CASE WHEN current = 'Active' THEN 1 WHEN former = 'Active' THEN -1 else 0 END) AS delta,
	(block_timestamp/1000000000)::BIGINT AS seconds_timestamp
	FROM update_node_account_status_events
	WHERE $1 < block_timestamp AND block_timestamp < $2
	GROUP BY seconds_timestamp
	ORDER BY seconds_timestamp
	`
	nodeDeltasRows, err := db.Query(ctx, nodeDeltasQ, window.From.ToNano(), window.Until.ToNano())
	if err != nil {
		return oapigen.EarningsHistoryResponse{}, err
	}
	defer nodeDeltasRows.Close()

	// PROCESS DATA
	// Create aggregate variables to be filled with row results

	intervalTotalLiquidityFees := make(map[db.Second]int64)
	var metaTotalLiquidityFees int64

	// NOTE: BondingRewards are total bonding rewards sent from reserve to nodes. They equal
	// the exact earnings  (BondingRewards = BondingEarnings = share of fees + block rewards)
	intervalTotalBondingRewards := make(map[db.Second]int64)
	var metaTotalBondingRewards int64

	// NOTE: Pool rewards are pool rewards sent from reserve (+) or sent to nodes (-). They
	// are the difference between share of rewards (fees + block) and the fees collected by
	// the pool
	intervalTotalPoolRewards := make(map[db.Second]int64)
	var metaTotalPoolRewards int64

	// NOTE: PoolEarnings = PoolRewards + LiquidityFees
	intervalPoolEarningsMaps := make(map[db.Second]poolEarningsMap)
	metaPoolEarningsMap := make(poolEarningsMap)

	intervalNodeCountWeightedSum := make(map[db.Second]int64)
	var metaNodeCountWeightedSum int64

	// Store query results into aggregate variables
	for liquidityFeesByPoolRows.Next() {
		var runeLiquidityFees, assetLiquidityFees, totalLiquidityFeesRune int64
		var startTime db.Second
		var pool string
		err := liquidityFeesByPoolRows.Scan(
			&runeLiquidityFees,
			&assetLiquidityFees,
			&totalLiquidityFeesRune,
			&startTime,
			&pool)
		if err != nil {
			return oapigen.EarningsHistoryResponse{}, err
		}

		if intervalPoolEarningsMaps[startTime] == nil {
			intervalPoolEarningsMaps[startTime] = make(poolEarningsMap)
		}

		// Add fees to earnings by pool
		intervalPoolEarnings := intervalPoolEarningsMaps[startTime].getPoolEarnings(pool)
		metaPoolEarnings := metaPoolEarningsMap.getPoolEarnings(pool)

		intervalPoolEarnings.runeLiquidityFees += runeLiquidityFees
		metaPoolEarnings.runeLiquidityFees += runeLiquidityFees

		intervalPoolEarnings.assetLiquidityFees += assetLiquidityFees
		metaPoolEarnings.assetLiquidityFees += assetLiquidityFees

		intervalPoolEarnings.totalLiquidityFeesRune += totalLiquidityFeesRune
		metaPoolEarnings.totalLiquidityFeesRune += totalLiquidityFeesRune

		// Add fees to total fees aggregate
		intervalTotalLiquidityFees[startTime] += totalLiquidityFeesRune
		metaTotalLiquidityFees += totalLiquidityFeesRune
	}

	for bondingRewardsRows.Next() {
		var bondingRewards int64
		var startTime db.Second
		err := bondingRewardsRows.Scan(&bondingRewards, &startTime)
		if err != nil {
			return oapigen.EarningsHistoryResponse{}, err
		}

		// Add rewards to total bonding rewards
		intervalTotalBondingRewards[startTime] += bondingRewards
		metaTotalBondingRewards += bondingRewards
	}

	for poolRewardsRows.Next() {
		var runeE8 int64
		var saverE8 int64
		var startTime db.Second
		var pool string
		err := poolRewardsRows.Scan(&runeE8, &saverE8, &startTime, &pool)
		if err != nil {
			return oapigen.EarningsHistoryResponse{}, err
		}

		if intervalPoolEarningsMaps[startTime] == nil {
			intervalPoolEarningsMaps[startTime] = make(poolEarningsMap)
		}

		// Add rewards to earnings by pool
		intervalPoolEarningsMaps[startTime].getPoolEarnings(pool).rewards += runeE8
		metaPoolEarningsMap.getPoolEarnings(pool).rewards += runeE8

		// Add saver reward to the pool
		intervalPoolEarningsMaps[startTime].getPoolEarnings(pool).saverRewards += saverE8
		metaPoolEarningsMap.getPoolEarnings(pool).saverRewards += saverE8

		// Add rewards to total pool rewards
		intervalTotalPoolRewards[startTime] += runeE8
		metaTotalPoolRewards += runeE8
	}

	// NOTE: Node Weighted Sums:
	// Transverse node updates calculating aggregated weighted sums
	// Start with node count from genesis up to the first interval
	// timestamp (NodeStartCount). Look for the next timestamp where a change in node
	// count happens and add weighted sum using where we started up to a
	// second before the change is detected for all time intervals in between.
	// Then update the counts and starting timestamps and keep
	// on interating until there are no more changes.
	// This is needed to get node avg count for each interval
	nodesCurrentTimestampIndex := 0
	nodesLastCount := nodeStartCount
	nodesLastCountTimestamp := timestamps[0]
	for nodeDeltasRows.Next() {
		var delta int64
		var deltaTimestamp db.Second
		err := nodeDeltasRows.Scan(&delta, &deltaTimestamp)
		if err != nil {
			return oapigen.EarningsHistoryResponse{}, err
		}

		for (nodesCurrentTimestampIndex < len(timestamps)-1) && timestamps[nodesCurrentTimestampIndex+1] < deltaTimestamp {
			// if delta timestamp is greater than the interval timestamp, the node count
			// didn't change from current timestamp to the start of next interval so weights
			// for the remaining of the interval are computed using nodesLastCount

			// Add weighted count up to the end of the interval
			weightedCount := (timestamps[nodesCurrentTimestampIndex+1].ToI() - nodesLastCountTimestamp.ToI()) * nodesLastCount
			intervalNodeCountWeightedSum[timestamps[nodesCurrentTimestampIndex]] += weightedCount
			metaNodeCountWeightedSum += weightedCount
			// Move to the next interval
			nodesCurrentTimestampIndex++
			nodesLastCountTimestamp = timestamps[nodesCurrentTimestampIndex]
		}

		// Add last weighted sum to interval and global aggregates (last count happend up to deltaTimestamp - 1)
		weightedCount := (deltaTimestamp.ToI() - nodesLastCountTimestamp.ToI()) * nodesLastCount
		intervalNodeCountWeightedSum[timestamps[nodesCurrentTimestampIndex]] += weightedCount
		metaNodeCountWeightedSum += weightedCount

		// Update Count and Last timestamps
		nodesLastCount += delta
		nodesLastCountTimestamp = deltaTimestamp
	}

	// Advance until last interval adding corresponding weighted counts
	for nodesCurrentTimestampIndex < (len(timestamps) - 1) {
		// Add weighted count up to the end of the interval
		weightedCount := (timestamps[nodesCurrentTimestampIndex+1].ToI() - nodesLastCountTimestamp.ToI()) * nodesLastCount
		intervalNodeCountWeightedSum[timestamps[nodesCurrentTimestampIndex]] += weightedCount
		metaNodeCountWeightedSum += weightedCount
		// Move to the next interval
		nodesCurrentTimestampIndex++
		nodesLastCountTimestamp = timestamps[nodesCurrentTimestampIndex]
	}

	// Add last weighted count
	endTimeInt := window.Until
	if nodesLastCountTimestamp < (endTimeInt - 1) {
		weightedCount := (endTimeInt - nodesLastCountTimestamp).ToI() * nodesLastCount
		intervalNodeCountWeightedSum[timestamps[nodesCurrentTimestampIndex]] += weightedCount
		metaNodeCountWeightedSum += weightedCount
	}

	// TODO(huginn): optimize USD Price history, alone this is ~500 ms.
	// f := timer.Console("earningsUSD")()

	usdPrices, err := USDPriceHistory(ctx, buckets)
	if err != nil {
		return oapigen.EarningsHistoryResponse{}, err
	}
	if len(usdPrices) != buckets.Count() {
		return oapigen.EarningsHistoryResponse{}, miderr.InternalErr("Misalligned buckets")
	}

	// TODO(huginn): remove console timer tail call
	// f()

	// BUILD RESPONSE

	// From earnings by pool get all Pools and build meta EarningsHistoryItemPools
	poolsList := make([]string, 0, len(metaPoolEarningsMap))
	metaEarningsItemPools := make([]oapigen.EarningsHistoryItemPool, 0, len(metaPoolEarningsMap))
	for pool, poolEarnings := range metaPoolEarningsMap {
		poolsList = append(poolsList, pool)
		metaEarningsItemPool := poolEarnings.toOapigen()
		metaEarningsItemPools = append(metaEarningsItemPools, metaEarningsItemPool)
	}

	// Build Response and Meta
	earnings := oapigen.EarningsHistoryResponse{
		Meta: buildEarningsItem(
			timestamps[0], window.Until, metaTotalLiquidityFees, metaTotalPoolRewards,
			metaTotalBondingRewards, metaNodeCountWeightedSum,
			usdPrices[len(usdPrices)-1].RunePriceUSD,
			metaEarningsItemPools),
		Intervals: make([]oapigen.EarningsHistoryItem, 0, len(timestamps)),
	}

	// Build and add Items to Response
	for i := 0; i < buckets.Count(); i++ {
		timestamp, endTime := buckets.Bucket(i)
		if usdPrices[i].Window.From != timestamp {
			err = miderr.InternalErr("Misalligned buckets")
		}

		intervalPoolEarningsMap := intervalPoolEarningsMaps[timestamp]

		// Process pools
		earningsItemPools := make([]oapigen.EarningsHistoryItemPool, 0, len(poolsList))
		for _, pool := range poolsList {
			var earningsItemPool oapigen.EarningsHistoryItemPool
			poolEarnings := intervalPoolEarningsMap.getPoolEarnings(pool)
			earningsItemPool = poolEarnings.toOapigen()
			earningsItemPools = append(earningsItemPools, earningsItemPool)
		}

		// build resulting interval
		earningsItem := buildEarningsItem(
			timestamp, endTime,
			intervalTotalLiquidityFees[timestamp], intervalTotalPoolRewards[timestamp],
			intervalTotalBondingRewards[timestamp],
			intervalNodeCountWeightedSum[timestamp],
			usdPrices[i].RunePriceUSD, earningsItemPools)

		earnings.Intervals = append(earnings.Intervals, earningsItem)
	}

	return earnings, nil
}

func buildEarningsItem(startTime, endTime db.Second,
	totalLiquidityFees, totalPoolRewards, totalBondingRewards, nodeCountWeightedSum int64,
	runePriceUSD float64,
	earningsItemPools []oapigen.EarningsHistoryItemPool) oapigen.EarningsHistoryItem {
	liquidityEarnings := totalPoolRewards + totalLiquidityFees
	earnings := liquidityEarnings + totalBondingRewards
	blockRewards := earnings - totalLiquidityFees

	avgNodeCount := float64(nodeCountWeightedSum) / float64(endTime-startTime)

	return oapigen.EarningsHistoryItem{
		StartTime:         util.IntStr(startTime.ToI()),
		EndTime:           util.IntStr(endTime.ToI()),
		LiquidityFees:     util.IntStr(totalLiquidityFees),
		BlockRewards:      util.IntStr(blockRewards),
		BondingEarnings:   util.IntStr(totalBondingRewards),
		LiquidityEarnings: util.IntStr(liquidityEarnings),
		Earnings:          util.IntStr(earnings),
		AvgNodeCount:      strconv.FormatFloat(avgNodeCount, 'f', 2, 64),
		RunePriceUSD:      floatStr(runePriceUSD),
		Pools:             earningsItemPools,
	}
}
