package main

// Automated check. To manually check values go here:
// https://testnet.thornode.thorchain.info/thorchain/pools
// https://testnet.midgard.thorchain.info/v2/pools

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

const usageStr = `Checks state at latest height.
Usage:
$ go run ./cmd/statechecks [--onlydepthdiff] [--nonodescheck] [--seachmin] config
`

func init() {
	flag.Usage = func() {
		fmt.Print(usageStr)
		flag.PrintDefaults()
	}
}

var OnlyStructuredDiff = flag.Bool("onlydepthdiff", false,
	"No binary search, only the latest depth differences in structured form.")

var NoNodesCheck = flag.Bool("nonodescheck", false,
	"Skip active node count and bonds check.")

var BinarySearchMin = flag.Int64("searchmin", 1,
	"Base of the binary search, a known good state.")

var (
	CheckUnits bool = true
	CheckBonds bool = false
)

type Pool struct {
	Pool        string `json:"asset"`
	AssetDepth  int64  `json:"balance_asset,string"`
	RuneDepth   int64  `json:"balance_rune,string"`
	SynthSupply int64  `json:"synth_supply,string"`
	LPUnits     int64  `json:"LP_units,string"`
	SaversDepth int64  `json:"savers_depth,string"`
	SaversUnits int64  `json:"savers_units,string"`
	Status      string `json:"status"`
	Timestamp   db.Nano
}

func (pool Pool) String() string {
	return fmt.Sprintf("%s [Asset: %d, Rune: %d, Synth: %d, Units: %d, Savers Depth: %d, Savers Units: %d]",
		pool.Pool, pool.AssetDepth, pool.RuneDepth, pool.SynthSupply,
		pool.LPUnits, pool.SaversDepth, pool.SaversUnits)
}

type State struct {
	Pools           map[string]Pool
	ActiveNodeCount int64
	TotalBonded     int64
}

type Node struct {
	Status      string `json:"status"`
	Address     string `json:"node_address"`
	TotalBond   string `json:"total_bond"`
	BondAddress string `json:"bond_address"`
}

func main() {
	midlog.LogCommandLine()

	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Println("Missing config argument!", flag.Args())
		flag.Usage()
		return
	}

	config.ReadGlobalFrom(flag.Arg(0))

	ctx := context.Background()

	dbinit.Setup()

	db.InitializeChainVarsFromThorNode()
	db.EnsureDBMatchesChain()

	if config.Global.Genesis.Local != "" {
		*BinarySearchMin = config.Global.Genesis.InitialBlockHeight
	}

	lastHeight, lastTimestamp := getLastBlockFromDB(ctx)
	midlog.InfoF("Latest height: %d, timestamp: %d", lastHeight, lastTimestamp)

	midgardState := getMidgardState(ctx, lastHeight, lastTimestamp)
	midlog.DebugF("Pools checked: %v", midgardState)

	thorNodeURL := config.Global.ThorChain.ThorNodeURL
	thornodeState := getThornodeState(ctx, thorNodeURL, lastHeight)

	if *OnlyStructuredDiff {
		reportStructuredDiff(midgardState, thornodeState)
	} else {
		problems := compareStates(midgardState, thornodeState)

		for _, pool := range problems.mismatchingPools {
			binarySearchPool(ctx, thorNodeURL, pool, *BinarySearchMin, lastHeight)
		}

		if problems.activeNodeCountError {
			binarySearchNodes(ctx, thorNodeURL, *BinarySearchMin, lastHeight)
		}

		if problems.bondError {
			BondDetails(ctx, thorNodeURL)
		}
	}
}

func getLastBlockFromDB(ctx context.Context) (lastHeight int64, lastTimestamp db.Nano) {
	midlog.Info("Getting latest recorded height...")
	lastHeightRows, err := db.Query(ctx,
		"SELECT height, timestamp from block_log order by height desc limit 2")
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer lastHeightRows.Close()

	// To avoid a race we take the second newest block, because maybe not all events are present yet.
	takeOne := func() {
		if !lastHeightRows.Next() {
			midlog.Fatal("No block found in DB")
		}
		err = lastHeightRows.Scan(&lastHeight, &lastTimestamp)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
	}
	takeOne()
	takeOne()
	return
}

func getMidgardState(ctx context.Context, height int64, timestamp db.Nano) (state State) {
	midlog.DebugF("Getting Midgard data at height: %d , timestamp: %d", height, timestamp)

	poolsQ := `
		SELECT asset FROM pool_events WHERE block_timestamp <= $1 GROUP BY asset ORDER BY asset`
	poolsRows, err := db.Query(ctx, poolsQ, timestamp)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer poolsRows.Close()

	poolsWithStatus, err := timeseries.GetPoolsStatuses(ctx, timestamp)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}

	state.Pools = map[string]Pool{}
	pools := []string{}

	for poolsRows.Next() {
		var poolName, status string

		err := poolsRows.Scan(&poolName)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}

		if record.GetCoinType([]byte(poolName)) == record.AssetNative {
			midlog.DebugF("Fetching Midgard pool: %s", poolName)
		}

		status = poolsWithStatus[poolName]
		if status == "" {
			status = timeseries.DefaultPoolStatus
		}
		if status == "suspended" {
			continue
		}

		pool := midgardPoolAtHeight(ctx, poolName, height)
		pool.Status = status
		state.Pools[pool.Pool] = pool
		pools = append(pools, pool.Pool)
	}

	if !*NoNodesCheck {
		state.ActiveNodeCount, err = timeseries.ActiveNodeCount(ctx, timestamp)
		if err != nil {
			midlog.FatalE(err, "Error getting Midgard active node count")
		}
		state.TotalBonded, err = stat.GetTotalBond(ctx, -1)
		if err != nil {
			midlog.FatalE(err, "Error gettign Midgard bonds")
		}
	}
	return
}

func queryThorNode(thorNodeUrl string, urlPath string, height int64, dest interface{}) {
	url := thorNodeUrl + urlPath
	if 0 < height {
		url += "?height=" + strconv.FormatInt(height, 10)
	}
	midlog.DebugF("Querying thornode: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		midlog.FatalE(err, "Error querying ThorNode")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		midlog.FatalE(err, "Error while reading the result")
	}

	err = json.Unmarshal(body, dest)
	if err != nil {
		midlog.WarnF("Json unmarshal error for url: %s", url)
		midlog.FatalE(err, "Error unmarshalling ThorNode response")

	}
}

func getThornodeNodesInfo(ctx context.Context, thorNodeUrl string, height int64) (
	nodeCount int64, totalBonded int64) {

	if *NoNodesCheck {
		return 0, 0
	}

	var nodes []Node
	queryThorNode(thorNodeUrl, "/nodes", height, &nodes)
	for _, node := range nodes {
		if strings.ToLower(node.Status) == "active" {
			nodeCount++
		}
		bond, err := strconv.ParseInt(node.TotalBond, 10, 64)
		if err != nil {
			midlog.FatalE(err, "Error getting ThorNode info")
		}

		totalBonded += bond
	}
	return
}

// true if active
func allThornodeNodes(ctx context.Context, thorNodeUrl string, height int64) map[string]bool {

	if *NoNodesCheck {
		return map[string]bool{}
	}

	var nodes []Node
	queryThorNode(thorNodeUrl, "/nodes", height, &nodes)
	ret := map[string]bool{}
	for _, node := range nodes {
		ret[node.Address] = (strings.ToLower(node.Status) == "active")
	}
	return ret
}

func getThornodeState(ctx context.Context, thorNodeUrl string, height int64) (state State) {
	midlog.Debug("Getting ThorNode data...")

	var pools []Pool

	queryThorNode(thorNodeUrl, "/pools", height, &pools)
	state.Pools = map[string]Pool{}
	for _, pool := range pools {
		state.Pools[pool.Pool] = pool
	}

	state.ActiveNodeCount, state.TotalBonded = getThornodeNodesInfo(ctx, thorNodeUrl, height)
	return
}

func reportStructuredDiff(midgardState, thornodeState State) {
	existenceDiff := strings.Builder{}
	depthDiffs := strings.Builder{}

	for _, thornodePool := range thornodeState.Pools {
		midgardPool, ok := midgardState.Pools[thornodePool.Pool]
		delete(midgardState.Pools, thornodePool.Pool)
		if !ok {
			fmt.Fprintf(&existenceDiff, "%s - did not find pool in Midgard (Exists in Thornode)\n", thornodePool.Pool)
			continue
		}

		runeDiff := thornodePool.RuneDepth - midgardPool.RuneDepth
		assetDiff := thornodePool.AssetDepth - midgardPool.AssetDepth
		if runeDiff != 0 || assetDiff != 0 {
			fmt.Fprintf(
				&depthDiffs, `{"%s", %d, %d},`+"\n",
				thornodePool.Pool, runeDiff, assetDiff)
		}

	}

	for name, pool := range midgardState.Pools {
		if pool.RuneDepth > 0 && pool.AssetDepth > 0 {
			fmt.Fprintf(&existenceDiff, "%s - did not find pool in Thornode (Exists in Midgard)\n", name)
		}
	}

	if existenceDiff.Len() != 0 {
		midlog.WarnF("Pool existence differences:\n%s", existenceDiff.String())
	}
	if depthDiffs.Len() != 0 {
		midlog.WarnF("Depth differences:\n%s", depthDiffs.String())
	}
}

type Problems struct {
	mismatchingPools     []string
	activeNodeCountError bool
	bondError            bool
}

func compareStates(midgardState, thornodeState State) (problems Problems) {
	mismatchingPools := map[string]bool{}

	errors := strings.Builder{}

	for _, thornodePool := range thornodeState.Pools {
		// ignore lending pools for now
		if thornodePool.Status == "Suspended" || strings.HasPrefix(thornodePool.Pool, "THOR") {
			continue
		}

		midgardPool, ok := midgardState.Pools[thornodePool.Pool]
		midgardSynthPool, synthOk := midgardState.Pools[util.ConvertNativePoolToSynth(thornodePool.Pool)]
		prompt := fmt.Sprintf("\t- [Pool:%s]:", thornodePool.Pool)

		delete(midgardState.Pools, thornodePool.Pool)
		if synthOk {
			delete(midgardState.Pools, util.ConvertNativePoolToSynth(thornodePool.Pool))
		}

		if !ok {
			fmt.Fprintf(&errors, "%s Did not find pool in Midgard (Exists in Thornode)\n", prompt)
			continue
		}

		if midgardPool.RuneDepth != thornodePool.RuneDepth {
			mismatchingPools[thornodePool.Pool] = true
			fmt.Fprintf(
				&errors, "%s RUNE Depth mismatch Thornode: %d, Midgard: %d\n",
				prompt, thornodePool.RuneDepth, midgardPool.RuneDepth)

		}

		if midgardPool.AssetDepth != thornodePool.AssetDepth {
			mismatchingPools[thornodePool.Pool] = true
			fmt.Fprintf(
				&errors, "%s Asset Depth mismatch Thornode: %d, Midgard: %d\n",
				prompt, thornodePool.AssetDepth, midgardPool.AssetDepth)
		}

		if midgardPool.SynthSupply != thornodePool.SynthSupply {
			mismatchingPools[thornodePool.Pool] = true
			fmt.Fprintf(
				&errors, "%s Synth Supply mismatch Thornode: %d, Midgard: %d\n",
				prompt, thornodePool.SynthSupply, midgardPool.SynthSupply)
		}

		if CheckUnits && midgardPool.LPUnits != thornodePool.LPUnits {
			mismatchingPools[thornodePool.Pool] = true
			fmt.Fprintf(
				&errors, "%s Pool Units mismatch Thornode: %d, Midgard: %d\n",
				prompt, thornodePool.LPUnits, midgardPool.LPUnits)
		}

		if midgardPool.Status != strings.ToLower(thornodePool.Status) {
			fmt.Fprintf(&errors, "%s Status mismatch Thornode: %s, Midgard: %s\n",
				prompt, strings.ToLower(thornodePool.Status), midgardPool.Status)
		}

		if synthOk && midgardSynthPool.AssetDepth != thornodePool.SaversDepth {
			mismatchingPools[thornodePool.Pool] = true
			fmt.Fprintf(
				&errors, "%s Pool Savers Depth mismatch Thornode: %d, Midgard: %d\n",
				prompt, thornodePool.SaversDepth, midgardSynthPool.AssetDepth)
		}

		if synthOk && midgardSynthPool.LPUnits != thornodePool.SaversUnits {
			mismatchingPools[thornodePool.Pool] = true
			fmt.Fprintf(
				&errors, "%s Pool Savers Units mismatch Thornode: %d, Midgard: %d\n",
				prompt, thornodePool.SaversUnits, midgardSynthPool.LPUnits)
		}
	}

	for name, pool := range midgardState.Pools {
		if pool.Status == "Suspended" || strings.HasPrefix(name, "THOR") {
			continue
		}

		isSynth := record.GetCoinType([]byte(name)) == record.AssetSynth

		prompt := fmt.Sprintf("\t- [Pool:%s]:", name)
		if !isSynth && pool.RuneDepth > 0 && pool.AssetDepth > 0 {
			fmt.Fprintf(&errors, "%s Did not find pool in Thornode (Exists in Midgard)\n", prompt)
		} else if isSynth && pool.AssetDepth > 0 {
			fmt.Fprintf(&errors, "%s Did not find saver for this pool in Thornode (Exists in Midgard)\n", prompt)
		}
	}

	if thornodeState.ActiveNodeCount != midgardState.ActiveNodeCount {
		problems.activeNodeCountError = true
		fmt.Fprintf(
			&errors, "\t- [Nodes]: Active Node Count mismatch Thornode: %d, Midgard %d\n",
			thornodeState.ActiveNodeCount, midgardState.ActiveNodeCount)
	}

	if CheckBonds && thornodeState.TotalBonded != midgardState.TotalBonded {
		problems.bondError = true
		tBonded := thornodeState.TotalBonded
		mBonded := midgardState.TotalBonded
		fmt.Fprintf(
			&errors,
			"\t- [Bonded]: Total Bonded mismatch Thornode: %d Midgard %d MidgardExcess %.2f%%\n",
			tBonded, mBonded, 100*float64(mBonded-tBonded)/float64(tBonded))
	}

	if errors.Len() > 0 {
		midlog.WarnF("ERRORS where found\n%s", errors.String())
	} else {
		midlog.Info("All state checks OK")
	}

	for pool := range mismatchingPools {
		problems.mismatchingPools = append(problems.mismatchingPools, pool)
	}
	sort.Strings(problems.mismatchingPools)
	return
}

func midgardPoolAtHeight(ctx context.Context, pool string, height int64) Pool {
	midlog.DebugF("Getting Midgard data at height: %d pool: %s", height, pool)

	q := `
		SELECT timestamp
		FROM block_log
		WHERE height=$1
	`

	ret := Pool{Pool: pool}
	rows, err := db.Query(ctx, q, height)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}

	if rows.Next() {
		err := rows.Scan(&ret.Timestamp)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
	}
	rows.Close()

	q = `
		SELECT block_timestamp, asset_e8, rune_e8, synth_e8
		FROM block_pool_depths
		WHERE block_timestamp<=$1 AND pool = $2
		ORDER BY block_timestamp DESC
		LIMIT 1
	`

	rows, err = db.Query(ctx, q, ret.Timestamp, pool)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}

	if rows.Next() {
		err := rows.Scan(&ret.Timestamp, &ret.AssetDepth, &ret.RuneDepth, &ret.SynthSupply)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
	}
	rows.Close()

	until := ret.Timestamp + 1
	unitsMap, err := stat.PoolsLiquidityUnitsBefore(ctx, []string{pool}, &until)
	if err != nil {
		midlog.FatalE(err, "Error getting Midgard pool units")
	}
	ret.LPUnits = unitsMap[pool]

	return ret
}

func findTablesWithColumns(ctx context.Context, columnName string) map[string]bool {
	q := `
	SELECT
		table_name
	FROM information_schema.columns
	WHERE table_schema='midgard' and column_name=$1
	`
	rows, err := db.Query(ctx, q, columnName)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	ret := map[string]bool{}
	for rows.Next() {
		var table string
		err := rows.Scan(&table)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
		ret[table] = true
	}

	return ret
}

type EventTable struct {
	TableName      string
	PoolColumnName string // "pool" or "asset" or ""
}

func findEventTables(ctx context.Context) []EventTable {
	blockTimestampTables := findTablesWithColumns(ctx, "block_timestamp")
	blockTimestampTables["block_pool_depths"] = false

	poolTables := findTablesWithColumns(ctx, "pool")
	assetTables := findTablesWithColumns(ctx, "asset")
	ret := []EventTable{}
	for table := range blockTimestampTables {
		if poolTables[table] {
			ret = append(ret, EventTable{TableName: table, PoolColumnName: "pool"})
		} else if assetTables[table] {
			ret = append(ret, EventTable{TableName: table, PoolColumnName: "asset"})
		} else {
			ret = append(ret, EventTable{TableName: table, PoolColumnName: ""})
		}
	}
	return ret
}

var (
	eventTablesCache []EventTable
	eventTablesOnce  sync.Once
)

func getEventTables(ctx context.Context) []EventTable {
	eventTablesOnce.Do(func() {
		eventTablesCache = findEventTables(ctx)
	})
	return eventTablesCache
}

func logEventsFromTable(ctx context.Context, eventTable EventTable, pool string, timestamp db.Nano) {
	if eventTable.TableName == "message_events" || eventTable.TableName == "block_pool_depths" {
		return
	}
	poolFilters := []string{"block_timestamp = $1"}
	qargs := []interface{}{timestamp}
	if eventTable.PoolColumnName != "" {
		synthPool := strings.Replace(pool, ".", "/", -1)
		poolFilters = append(poolFilters,
			fmt.Sprintf("(%[1]s = $2) OR (%[1]s = $3)",
				eventTable.PoolColumnName))
		qargs = append(qargs, pool, synthPool)
	}

	q := `
	SELECT *
	FROM ` + eventTable.TableName + ` ` + db.Where(poolFilters...)

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		panic(err)
	}

	eventNum := 0
	for rows.Next() {
		eventNum++
		colsPtr := make([]interface{}, len(colNames))
		for i := range colsPtr {
			var tmp interface{}
			colsPtr[i] = &tmp
		}
		err := rows.Scan(colsPtr...)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
		buf := strings.Builder{}

		fmt.Fprintf(&buf, "%s [", eventTable.TableName)
		for i := range colNames {
			if i != 0 {
				fmt.Fprintf(&buf, ", ")
			}
			fmt.Fprintf(&buf, "%s: %v", colNames[i], *(colsPtr[i].(*interface{})))
		}
		fmt.Fprintf(&buf, "]")
		midlog.Info(buf.String())
	}
}

func logAllEventsAtHeight(ctx context.Context, pool string, timestamp db.Nano) {
	eventTables := getEventTables(ctx)
	for _, eventTable := range eventTables {
		logEventsFromTable(ctx, eventTable, pool, timestamp)
	}
}

// Looks up the first difference in the (min, max) range. May choose max.
func binarySearchPool(ctx context.Context, thorNodeUrl string, pool string, minHeight, maxHeight int64) {
	midlog.InfoF("=====  [%s] Binary searching in range [%d, %d)", pool, minHeight, maxHeight)

	for 1 < maxHeight-minHeight {
		middleHeight := (minHeight + maxHeight) / 2
		midlog.DebugF(
			"--- [%s] Binary search step [%d, %d] height: %d",
			pool, minHeight, maxHeight, middleHeight)
		var thorNodePool Pool
		queryThorNode(thorNodeUrl, "/pool/"+pool, middleHeight, &thorNodePool)
		midlog.DebugF("Thornode: %v", thorNodePool)
		midgardPool := midgardPoolAtHeight(ctx, pool, middleHeight)
		midgardSynthPool := midgardPoolAtHeight(ctx, util.ConvertNativePoolToSynth(pool), middleHeight)
		midgardPool.SaversDepth = midgardSynthPool.AssetDepth
		midgardPool.SaversUnits = midgardSynthPool.LPUnits
		midlog.DebugF("Midgard: %v", midgardPool)
		ok := (thorNodePool.AssetDepth == midgardPool.AssetDepth &&
			thorNodePool.RuneDepth == midgardPool.RuneDepth &&
			thorNodePool.SynthSupply == midgardPool.SynthSupply &&
			(!CheckUnits || thorNodePool.LPUnits == midgardPool.LPUnits) &&
			(!CheckUnits || thorNodePool.SaversUnits == midgardSynthPool.LPUnits) &&
			(midgardSynthPool.AssetDepth == thorNodePool.SaversDepth))
		if ok {
			midlog.DebugF("Same at height %d", middleHeight)
			minHeight = middleHeight
		} else {
			midlog.DebugF("Differ at height %d", middleHeight)
			maxHeight = middleHeight
		}
	}

	midgardPoolBefore := midgardPoolAtHeight(ctx, pool, maxHeight-1)
	midgardSynthPoolBefore := midgardPoolAtHeight(ctx, util.ConvertNativePoolToSynth(pool), maxHeight-1)
	midgardPoolBefore.SaversDepth = midgardSynthPoolBefore.AssetDepth
	midgardPoolBefore.SaversUnits = midgardSynthPoolBefore.LPUnits

	var thorNodePool Pool
	queryThorNode(thorNodeUrl, "/pool/"+pool, maxHeight, &thorNodePool)
	midgardPool := midgardPoolAtHeight(ctx, pool, maxHeight)
	midgardSynthPool := midgardPoolAtHeight(ctx, util.ConvertNativePoolToSynth(pool), maxHeight)
	midgardPool.SaversDepth = midgardSynthPool.AssetDepth
	midgardPool.SaversUnits = midgardSynthPool.LPUnits

	midlog.InfoF("[%s] First difference at height: %d timestamp: %d date: %s",
		pool, maxHeight, midgardPool.Timestamp,
		midgardPool.Timestamp.ToSecond().ToTime().Format("2006-01-02 15:04:05"))
	midlog.InfoF("Previous state:  %v", midgardPoolBefore)
	midlog.InfoF("Thornode:        %v", thorNodePool)
	midlog.InfoF("Midgard:         %v", midgardPool)

	logWithPercent := func(msg string, diffValue int64, base int64) {
		percent := 100 * float64(diffValue) / float64(base)
		if base == 0 && diffValue == 0 {
			percent = 0
		}
		midlog.InfoF("%s:  %d (%f%%)", msg, diffValue, percent)
	}
	logWithPercent("Midgard Asset excess",
		midgardPool.AssetDepth-thorNodePool.AssetDepth,
		midgardPoolBefore.AssetDepth)
	logWithPercent("Midgard Rune excess",
		midgardPool.RuneDepth-thorNodePool.RuneDepth,
		midgardPoolBefore.RuneDepth)
	logWithPercent("Midgard Synth excess",
		midgardPool.SynthSupply-thorNodePool.SynthSupply,
		midgardPoolBefore.SynthSupply)
	logWithPercent("Midgard Unit excess",
		midgardPool.LPUnits-thorNodePool.LPUnits,
		midgardPoolBefore.LPUnits)
	logWithPercent("Midgard Savers Depth excess",
		midgardSynthPool.AssetDepth-thorNodePool.SaversDepth,
		midgardSynthPoolBefore.AssetDepth)
	logWithPercent("Midgard Savers Units excess",
		midgardSynthPool.LPUnits-thorNodePool.SaversUnits,
		midgardSynthPoolBefore.LPUnits)

	logAllEventsAtHeight(ctx, pool, midgardPool.Timestamp)
}

func timestampAtHeight(ctx context.Context, height int64) db.Nano {
	q := `
	SELECT timestamp
	FROM block_log
	WHERE height=$1
	`
	rows, err := db.Query(ctx, q, height)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	if !rows.Next() {
		midlog.FatalF("No rows selected: %v", q)
	}
	var ts db.Nano
	err = rows.Scan(&ts)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	return ts
}

func midgardActiveNodeCount(ctx context.Context, height int64) int64 {
	timestamp := timestampAtHeight(ctx, height)
	midgardCount, err := timeseries.ActiveNodeCount(ctx, timestamp)
	if err != nil {
		midlog.FatalE(err, "Error getting Midgard active node count")
	}
	return midgardCount
}

func allMidgardNodes(ctx context.Context, height int64) map[string]bool {
	timestamp := timestampAtHeight(ctx, height)
	q := `
	SELECT
		node_addr,
		last(current, block_timestamp) AS status
	FROM update_node_account_status_events
	WHERE block_timestamp <= $1
	GROUP BY node_addr
	`
	rows, err := db.Query(ctx, q, timestamp)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	ret := map[string]bool{}
	for rows.Next() {
		var addr, status string
		err = rows.Scan(&addr, &status)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
		midlog.DebugF("Status: %s", strings.ToLower(status))
		ret[addr] = (strings.ToLower(status) == "active")
	}
	return ret
}

func excessNodes(str string, a, b map[string]bool) {
	buf := strings.Builder{}
	hasdiff := false
	for node, status := range a {
		if !status {
			continue
		}
		status2b, ok := b[node]
		if !ok {
			fmt.Fprintf(&buf, "present: %s - ", node)
			hasdiff = true
		} else if status2b == false {
			fmt.Fprintf(&buf, "active: %s - ", node)
			hasdiff = true
		}
	}
	if hasdiff {
		midlog.InfoF("%s excess: %s", str, buf.String())
	} else {
		midlog.InfoF("%s OK", str)
	}
}

// Looks up the first difference in the (min, max) range. May choose max.
func binarySearchNodes(ctx context.Context, thorNodeUrl string, minHeight, maxHeight int64) {
	midlog.InfoF("=====  Binary searching active nodes in range [%d, %d)", minHeight, maxHeight)

	for 1 < maxHeight-minHeight {
		middleHeight := (minHeight + maxHeight) / 2
		midlog.DebugF(
			"--- Binary search step [%d, %d] height: %d",
			minHeight, maxHeight, middleHeight)
		thorNodeCount, _ := getThornodeNodesInfo(ctx, thorNodeUrl, middleHeight)
		midlog.DebugF("Thornode: %d", thorNodeCount)

		midgardCount := midgardActiveNodeCount(ctx, middleHeight)
		midlog.DebugF("Midgard: %d", midgardCount)
		ok := midgardCount == thorNodeCount
		if ok {
			midlog.DebugF("Same at height %d", middleHeight)
			minHeight = middleHeight
		} else {
			midlog.DebugF("Differ at height %d", middleHeight)
			maxHeight = middleHeight
		}
	}

	countBefore := midgardActiveNodeCount(ctx, maxHeight-1)

	thorNodeCount, _ := getThornodeNodesInfo(ctx, thorNodeUrl, maxHeight)
	midgardCount := midgardActiveNodeCount(ctx, maxHeight)

	midlog.InfoF("First node difference at height: %d timestamp: %d",
		maxHeight, timestampAtHeight(ctx, maxHeight))
	midlog.InfoF("Previous state:  %d", countBefore)
	midlog.InfoF("Thornode:        %d", thorNodeCount)
	midlog.InfoF("Midgard:         %d", midgardCount)

	prevThornodeNodes := allThornodeNodes(ctx, thorNodeUrl, maxHeight-1)
	prevMidgardNodes := allMidgardNodes(ctx, maxHeight-1)
	excessNodes("previous thornode vs midgard", prevThornodeNodes, prevMidgardNodes)
	excessNodes("previous midgard vs thornode", prevMidgardNodes, prevThornodeNodes)

	curentThornodeNodes := allThornodeNodes(ctx, thorNodeUrl, maxHeight)
	curentMidgardNodes := allMidgardNodes(ctx, maxHeight)
	excessNodes("Current thornode vs midgard", curentThornodeNodes, curentMidgardNodes)
	excessNodes("Current midgard vs thornode", curentMidgardNodes, curentThornodeNodes)
}
