package timeseries

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/pascaldekloe/metrics"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"

	"gitlab.com/thorchain/midgard/internal/fetch/notinchain"
)

// ErrBeyondLast denies a request into the future.
var errBeyondLast = errors.New("cannot resolve beyond the last block (timestamp)")

// LastChurnHeight gets the latest block where a vault was activated
func LastChurnHeight(ctx context.Context) (int64, error) {
	q := `SELECT bl.height
	FROM active_vault_events av
	INNER JOIN block_log bl ON av.block_timestamp = bl.timestamp
	ORDER BY av.block_timestamp DESC LIMIT 1;
	`
	rows, err := db.Query(ctx, q)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	ok := rows.Next()

	if !ok {
		return -1, nil
	}

	var lastChurnHeight int64
	err = rows.Scan(&lastChurnHeight)
	if err != nil {
		return 0, err
	}
	return lastChurnHeight, nil
}

func GetChurnsData(ctx context.Context) (oapigen.Churns, error) {
	const q = `SELECT DISTINCT ON (bl.height) height, bl.timestamp 
	FROM active_vault_events as ac 
	INNER JOIN block_log as bl ON ac.block_timestamp = bl.timestamp
	ORDER BY height DESC;
	`

	rows, err := db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var churnMet oapigen.Churns
	var entry oapigen.ChurnItem
	for rows.Next() {
		err = rows.Scan(&entry.Height, &entry.Date)
		if err != nil {
			return nil, err
		}
		churnMet = append(churnMet, entry)
	}

	return churnMet, err
}

// PoolsWithDeposit gets all asset identifiers that have at least one deposit
func PoolsWithDeposit(ctx context.Context) ([]string, error) {
	const q = "SELECT pool FROM stake_events GROUP BY pool"
	rows, err := db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return pools, err
		}
		pools = append(pools, s)
	}
	return pools, rows.Err()
}

const DefaultPoolStatus = "staged"

// Returns last status change for pool for a given point in time (UnixNano timestamp)
// If a pool with assets has no status change, it means it is in "staged" status
// status is lowercase
func GetPoolsStatuses(ctx context.Context, moment db.Nano) (map[string]string, error) {
	const q = `
	SELECT asset, last(status, event_id) AS status FROM pool_events
	WHERE block_timestamp <= $1
	GROUP BY asset`

	rows, err := db.Query(ctx, q, moment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ret := map[string]string{}
	for rows.Next() {
		var pool, status string

		err := rows.Scan(&pool, &status)
		status = strings.ToLower(status)
		if err != nil {
			return nil, err
		}

		ret[pool] = status
	}
	return ret, nil
}

func PoolStatus(ctx context.Context, pool string) (string, error) {
	const q = "SELECT COALESCE(last(status, event_id), '') FROM pool_events WHERE asset = $1"
	rows, err := db.Query(ctx, q, pool)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var status string
	if rows.Next() {
		if err := rows.Scan(&status); err != nil {
			return "", err
		}
	}

	if status == "" {
		status = DefaultPoolStatus
	}
	return strings.ToLower(status), rows.Err()
}

var RewardEntriesAggregate = db.RegisterAggregate(
	db.NewAggregate("rewards_event_entries", "rewards_event_entries").
		AddGroupColumn("pool").
		AddBigintSumColumn("rune_e8"))

// TotalLiquidityFeesRune gets sum of liquidity fees in Rune for a given time interval
func TotalLiquidityFeesRune(ctx context.Context, from time.Time, to time.Time) (int64, error) {
	liquidityFeeQ := `SELECT COALESCE(SUM(liq_fee_in_rune_E8), 0)
	FROM swap_events
	WHERE block_timestamp >= $1 AND block_timestamp <= $2
	`
	var liquidityFees int64
	err := QueryOneValue(&liquidityFees, ctx, liquidityFeeQ, from.UnixNano(), to.UnixNano())
	if err != nil {
		return 0, err
	}

	return liquidityFees, nil
}

// Get value from Mimir overrides or from the Thorchain constants.
func GetLastConstantValue(ctx context.Context, key string) (int64, error) {
	// TODO(elfedy): This looks at the last time the mimir value was set. This may not be
	// the latest value (i.e: Does Thorchain send an event with the value in constants if mimir
	// override is unset?). The logic behind this needs to be investigated further.
	q := `SELECT CAST (value AS BIGINT)
	FROM set_mimir_events
	WHERE key ILIKE $1
	ORDER BY block_timestamp DESC
	LIMIT 1`
	rows, err := db.Query(ctx, q, key)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	// Use mimir value if there is one
	var result int64
	if rows.Next() {
		err := rows.Scan(&result)
		if err != nil {
			return 0, err
		}
	} else {
		constants := notinchain.GetConstants()

		var ok bool
		result, ok = constants.Int64Values[key]
		if !ok {
			return 0, fmt.Errorf("Key %q not found in constants\n", key)
		}
	}
	return result, nil
}

// StatusPerNode gets the labels for a given point in time.
// New nodes have the empty string (for no confirmed status).
// A zero moment defaults to the latest available.
// Requests beyond the last block cause an error.
func StatusPerNode(ctx context.Context, moment time.Time) (map[string]string, error) {
	_, timestamp, _ := LastBlock()
	if moment.IsZero() {
		moment = timestamp
	} else if timestamp.Before(moment) {
		return nil, errBeyondLast
	}

	m, err := newNodes(ctx, moment)
	if err != nil {
		return nil, err
	}

	// could optimise by only fetching latest
	const q = "SELECT node_addr, current FROM update_node_account_status_events WHERE block_timestamp <= $1"
	rows, err := db.Query(ctx, q, moment.UnixNano())
	if err != nil {
		return nil, fmt.Errorf("status per node lookup: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var node, status string
		err := rows.Scan(&node, &status)
		if err != nil {
			return m, fmt.Errorf("status per node retrieve: %w", err)
		}
		m[node] = status
	}
	return m, rows.Err()
}

// Returns Active node count for a given Unix Nano timestamp
func ActiveNodeCount(ctx context.Context, moment db.Nano) (int64, error) {
	nodeStartCountQ := `
	SELECT COUNT(*)
	FROM (
		SELECT
			node_addr,
			LAST(current, block_timestamp) AS last_status
		FROM update_node_account_status_events
		WHERE block_timestamp <= $1
		GROUP BY node_addr) AS NODES
	WHERE last_status = 'Active';`

	var nodeStartCount int64
	err := QueryOneValue(&nodeStartCount, ctx, nodeStartCountQ, moment)
	if err != nil {
		return nodeStartCount, err
	}
	return nodeStartCount, nil
}

func newNodes(ctx context.Context, moment time.Time) (map[string]string, error) {
	// could optimise by only fetching latest
	const q = "SELECT node_addr FROM new_node_events WHERE block_timestamp <= $1"
	rows, err := db.Query(ctx, q, moment.UnixNano())
	if err != nil {
		return nil, fmt.Errorf("new node lookup: %w", err)
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var node string
		err := rows.Scan(&node)
		if err != nil {
			return m, fmt.Errorf("new node retrieve: %w", err)
		}
		m[node] = ""
	}
	return m, rows.Err()
}

// NodesSecpAndEd returs the public keys mapped to their respective addresses.
func NodesSecpAndEd(ctx context.Context, t time.Time) (secp256k1Addrs, ed25519Addrs map[string]string, err error) {
	const q = `SELECT node_addr, secp256k1, ed25519
FROM set_node_keys_events
WHERE block_timestamp <= $1`

	rows, err := db.Query(ctx, q, t.UnixNano())
	if err != nil {
		return nil, nil, fmt.Errorf("node addr lookup: %w", err)
	}
	defer rows.Close()

	secp256k1Addrs = make(map[string]string)
	ed25519Addrs = make(map[string]string)
	for rows.Next() {
		var addr, secp, ed string
		if err := rows.Scan(&addr, &secp, &ed); err != nil {
			return nil, nil, fmt.Errorf("node addr resolve: %w", err)
		}
		if current, ok := secp256k1Addrs[secp]; ok && current != addr {
			log.Debug().Msgf("secp256k1 key %q used by node address %q and %q", secp, current, addr)
		}
		secp256k1Addrs[secp] = addr
		if current, ok := ed25519Addrs[ed]; ok && current != addr {
			log.Debug().Msgf("ed25519 key %q used by node address %q and %q", ed, current, addr)
		}
		ed25519Addrs[secp] = addr
	}
	return
}

var NetworkNilNode = metrics.MustCounter(
	"midgard_network_nil_node",
	"Number of times thornode returned nil node in thorchain/nodes.")

func GetNetworkData(ctx context.Context) (oapigen.Network, error) {
	// GET DATA
	// in memory lookups
	var result oapigen.Network

	_, runeE8DepthPerPool, timestamp := AssetAndRuneDepths()
	var runeDepth int64
	for _, depth := range runeE8DepthPerPool {
		runeDepth += depth
	}
	currentHeight, _, _ := LastBlock()

	// db lookups
	lastChurnHeight, err := LastChurnHeight(ctx)
	if err != nil {
		return result, err
	}

	weeklyLiquidityFeesRune, err := TotalLiquidityFeesRune(ctx, timestamp.Add(-1*time.Hour*24*7), timestamp)
	if err != nil {
		return result, err
	}

	// Thorchain constants
	emissionCurve, err := GetLastConstantValue(ctx, "EmissionCurve")
	if err != nil {
		return result, err
	}
	blocksPerYear, err := GetLastConstantValue(ctx, "BlocksPerYear")
	if err != nil {
		return result, err
	}
	churnInterval, err := GetLastConstantValue(ctx, "ChurnInterval")
	if err != nil {
		return result, err
	}
	churnRetryInterval, err := GetLastConstantValue(ctx, "ChurnRetryInterval")
	if err != nil {
		return result, err
	}
	poolCycle, err := GetLastConstantValue(ctx, "PoolCycle")
	if err != nil {
		return result, err
	}
	incentiveCurve, err := GetLastConstantValue(ctx, "IncentiveCurve")
	if err != nil {
		return result, err
	}
	minimumEligibleBond, err := GetLastConstantValue(ctx, "MinimumBondInRune")
	if err != nil {
		return result, err
	}

	// Thornode queries
	nodes, err := notinchain.NodeAccountsLookup()
	if err != nil {
		return result, err
	}
	networkData, err := notinchain.NetworkLookup()
	if err != nil {
		return result, err
	}

	// PROCESS DATA
	activeNodes := make(map[string]struct{})
	standbyNodes := make(map[string]struct{})
	var activeBonds, standbyBonds sortedBonds
	for _, node := range nodes {
		if node == nil {
			// TODO(muninn): check if this was the reason of the errors in production
			midlog.Warn("ThorNode returned nil node in thorchain/nodes")
			NetworkNilNode.Add(1)
			continue
		}
		switch node.Status {
		case "Active":
			activeNodes[node.NodeAddr] = struct{}{}
			activeBonds = append(activeBonds, node.Bond)
		case "Standby":
			standbyNodes[node.NodeAddr] = struct{}{}
			standbyBonds = append(standbyBonds, node.Bond)
		}
	}
	sort.Sort(activeBonds)
	sort.Sort(standbyBonds)

	bondMetrics := ActiveAndStandbyBondMetrics(activeBonds, standbyBonds, minimumEligibleBond)

	var poolShareFactor float64 = 0

	if bondMetrics.TotalActiveBond > runeDepth {
		fBond := float64(bondMetrics.TotalActiveBond)
		fPooled := float64(runeDepth)
		if incentiveCurve <= 0 {
			incentiveCurve = 1
		}
		// This is imitating the way ThorNode calculates poolsharefactor:
		// https://gitlab.com/thorchain/thornode/-/blob/12a80fef4a18a91ed27ecad812c1117ce9721e49/x/thorchain/manager_network_v1.go#L854
		var fPooledDenominator float64 = 0
		if incentiveCurve < 100 {
			fPooledDenominator = fPooled / float64(incentiveCurve)
		}
		poolShareFactor = (fBond - fPooled) / (fBond + fPooledDenominator)
	}

	blockRewards := calculateBlockRewards(emissionCurve, blocksPerYear, networkData.TotalReserve, poolShareFactor)

	nextChurnHeight := calculateNextChurnHeight(currentHeight, lastChurnHeight, churnInterval, churnRetryInterval)

	// Calculate pool/node weekly income and extrapolate to get liquidity/bonding APY
	yearlyBlockRewards := float64(blockRewards.BlockReward * blocksPerYear)
	weeklyBlockRewards := yearlyBlockRewards / WeeksInYear

	weeklyTotalIncome := weeklyBlockRewards + float64(weeklyLiquidityFeesRune)
	weeklyBondIncome := weeklyTotalIncome * (1 - poolShareFactor)
	weeklyPoolIncome := weeklyTotalIncome * poolShareFactor

	var bondingAPY float64
	if bondMetrics.TotalActiveBond > 0 {
		weeklyBondingRate := weeklyBondIncome / float64(bondMetrics.TotalActiveBond)
		bondingAPY = calculateAPYInterest(weeklyBondingRate, WeeksInYear)
	}

	var liquidityAPY float64
	if runeDepth > 0 {
		poolDepthInRune := float64(2 * runeDepth)
		weeklyPoolRate := weeklyPoolIncome / poolDepthInRune
		liquidityAPY = calculateAPYInterest(weeklyPoolRate, WeeksInYear)
	}

	return oapigen.Network{
		ActiveBonds:     intArrayStrs(activeBonds),
		ActiveNodeCount: util.IntStr(int64(len(activeNodes))),
		BlockRewards: oapigen.BlockRewards{
			BlockReward: util.IntStr(blockRewards.BlockReward),
			BondReward:  util.IntStr(blockRewards.BondReward),
			PoolReward:  util.IntStr(blockRewards.PoolReward),
		},
		// TODO(acsaba): create bondmetrics right away with this type.
		BondMetrics: oapigen.BondMetrics{
			TotalActiveBond:    util.IntStr(bondMetrics.TotalActiveBond),
			AverageActiveBond:  util.IntStr(bondMetrics.AverageActiveBond),
			MedianActiveBond:   util.IntStr(bondMetrics.MedianActiveBond),
			MinimumActiveBond:  util.IntStr(bondMetrics.MinimumActiveBond),
			MaximumActiveBond:  util.IntStr(bondMetrics.MaximumActiveBond),
			TotalStandbyBond:   util.IntStr(bondMetrics.TotalStandbyBond),
			AverageStandbyBond: util.IntStr(bondMetrics.AverageStandbyBond),
			MedianStandbyBond:  util.IntStr(bondMetrics.MedianStandbyBond),
			MinimumStandbyBond: util.IntStr(bondMetrics.MinimumStandbyBond),
			MaximumStandbyBond: util.IntStr(bondMetrics.MaximumStandbyBond),
		},
		BondingAPY:              floatStr(bondingAPY),
		LiquidityAPY:            floatStr(liquidityAPY),
		NextChurnHeight:         util.IntStr(nextChurnHeight),
		PoolActivationCountdown: util.IntStr(poolCycle - currentHeight%poolCycle),
		PoolShareFactor:         floatStr(poolShareFactor),
		StandbyBonds:            intArrayStrs(standbyBonds),
		StandbyNodeCount:        util.IntStr(int64(len(standbyNodes))),
		TotalReserve:            util.IntStr(networkData.TotalReserve),
		TotalPooledRune:         util.IntStr(runeDepth),
	}, nil
}

func intArrayStrs(a []int64) []string {
	b := make([]string, len(a))
	for i, v := range a {
		b[i] = util.IntStr(v)
	}
	return b
}

const WeeksInYear = 52

type sortedBonds []int64

func (b sortedBonds) Len() int           { return len(b) }
func (b sortedBonds) Less(i, j int) bool { return b[i] < b[j] }
func (b sortedBonds) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

type bondMetricsInts struct {
	TotalActiveBond   int64
	MinimumActiveBond int64
	MaximumActiveBond int64
	AverageActiveBond int64
	MedianActiveBond  int64

	TotalStandbyBond   int64
	MinimumStandbyBond int64
	MaximumStandbyBond int64
	AverageStandbyBond int64
	MedianStandbyBond  int64
}

func ActiveAndStandbyBondMetrics(active, standby sortedBonds, minimumEligibleBond int64) *bondMetricsInts {
	var metrics bondMetricsInts
	if len(active) != 0 {
		var total int64
		for _, n := range active {
			total += n
		}
		metrics.TotalActiveBond = total
		metrics.MinimumActiveBond = active[0]
		metrics.MaximumActiveBond = active[len(active)-1]
		metrics.AverageActiveBond = total / int64(len(active))
		metrics.MedianActiveBond = active[len(active)/2]
	}
	if len(standby) != 0 {
		var total int64
		var minimumStandbyBond int64 = standby[0]
		var minimumIsFound bool = false

		for _, n := range standby {
			if n >= minimumEligibleBond && !minimumIsFound {
				minimumStandbyBond = n
				minimumIsFound = true
			}
			total += n
		}
		metrics.TotalStandbyBond = total
		metrics.MinimumStandbyBond = minimumStandbyBond
		metrics.MaximumStandbyBond = standby[len(standby)-1]
		metrics.AverageStandbyBond = total / int64(len(standby))
		metrics.MedianStandbyBond = standby[len(standby)/2]
	}
	return &metrics
}

type blockRewardsInts struct {
	BlockReward int64
	BondReward  int64
	PoolReward  int64
}

func calculateBlockRewards(emissionCurve int64, blocksPerYear int64, totalReserve int64, poolShareFactor float64) *blockRewardsInts {
	blockReward := float64(totalReserve) / float64(emissionCurve*blocksPerYear)
	bondReward := (1 - poolShareFactor) * blockReward
	poolReward := blockReward - bondReward

	rewards := blockRewardsInts{int64(blockReward), int64(bondReward), int64(poolReward)}
	return &rewards
}

func calculateNextChurnHeight(currentHeight int64, lastChurnHeight int64, churnInterval int64, churnRetryInterval int64) int64 {
	if lastChurnHeight < 0 {
		// We didn't find a churn yet.
		return -1
	}
	firstChurnAttempt := lastChurnHeight + churnInterval
	var next int64
	if currentHeight < firstChurnAttempt {
		next = firstChurnAttempt
	} else {
		retriesHappened := (currentHeight - firstChurnAttempt) / churnRetryInterval
		next = firstChurnAttempt + churnRetryInterval*(retriesHappened+1)
	}
	return next
}

func calculateAPYInterest(periodicRate float64, periodsPerYear float64) float64 {
	if 1 < periodsPerYear {
		return math.Pow(1+periodicRate, periodsPerYear) - 1
	}
	return periodicRate * periodsPerYear
}

// CalculateSynthUnits calculate dynamic synth units
// (L*S)/(2*A-S)
// L = LP units
// S = synth balance
// A = asset balance
func CalculateSynthUnits(assetDepth, synthDepth, liquidityUnits int64) int64 {
	if assetDepth == 0 {
		return 0
	}
	A := big.NewInt(assetDepth)
	S := big.NewInt(synthDepth)
	L := big.NewInt(liquidityUnits)
	numerator := new(big.Int).Mul(L, S)
	denominator := new(big.Int).Sub(new(big.Int).Mul(A, big.NewInt(2)), S)
	if denominator == big.NewInt(0) {
		return 0
	}
	return new(big.Int).Quo(numerator, denominator).Int64()
}
