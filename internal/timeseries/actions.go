package timeseries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

const MaxAddresses = 50

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

type action struct {
	pools     []string
	eventType string
	status    string
	in        []transaction
	out       []transaction
	date      int64
	height    int64
	metadata  oapigen.Metadata
}

func (a action) toOapigen() oapigen.Action {
	oapigenIn := make([]oapigen.Transaction, len(a.in))
	oapigenOut := make([]oapigen.Transaction, len(a.out))

	for i, tx := range a.in {
		oapigenIn[i] = tx.toOapigen()
	}

	for i, tx := range a.out {
		oapigenOut[i] = tx.toOapigen()
	}

	return oapigen.Action{
		Pools:    a.pools,
		Type:     oapigen.ActionType(a.eventType),
		Status:   oapigen.ActionStatus(a.status),
		In:       oapigenIn,
		Out:      oapigenOut,
		Date:     util.IntStr(a.date),
		Height:   util.IntStr(a.height),
		Metadata: a.metadata,
	}
}

type transaction struct {
	address string
	coins   coinList
	txID    string
}

func (tx transaction) toOapigen() oapigen.Transaction {
	return oapigen.Transaction{
		Address: tx.address,
		TxID:    tx.txID,
		Coins:   tx.coins.toOapigen(),
	}
}

type coin struct {
	asset  string
	amount int64
}

func (c coin) toOapigen() oapigen.Coin {
	return oapigen.Coin{
		Asset:  c.asset,
		Amount: util.IntStr(c.amount),
	}
}

type coinList []coin

func (coins coinList) toOapigen() []oapigen.Coin {
	oapigenCoins := make([]oapigen.Coin, len(coins))
	for i, c := range coins {
		oapigenCoins[i] = c.toOapigen()
	}
	return oapigenCoins
}

const blankTxId = ""

type ActionsParams struct {
	Limit      string
	Offset     string
	ActionType string
	Address    string
	TXId       string
	Asset      string
}

// Gets a list of actions generated by external transactions and return its associated data
func GetActions(ctx context.Context, moment time.Time, params ActionsParams) (
	oapigen.ActionsResponse, error) {
	// CHECK PARAMS
	// give latest value if zero moment
	_, timestamp, _ := LastBlock()
	if moment.IsZero() {
		moment = timestamp
	} else if timestamp.Before(moment) {
		return oapigen.ActionsResponse{}, errBeyondLast
	}

	// check limit param
	if params.Limit == "" {
		return oapigen.ActionsResponse{}, errors.New("query parameter 'limit' is required")
	}
	limit, err := strconv.ParseUint(params.Limit, 10, 64)
	if err != nil || limit < 1 || limit > 50 {
		return oapigen.ActionsResponse{}, errors.New("'limit' must be an integer between 1 and 50")
	}

	// check offset param
	if params.Offset == "" {
		return oapigen.ActionsResponse{}, errors.New("query parameter 'offset' is required")
	}
	offset, err := strconv.ParseUint(params.Offset, 10, 64)
	if err != nil {
		return oapigen.ActionsResponse{}, errors.New("'offset' must be a non-negative integer")
	}

	// build types from type param
	types := make([]string, 0)
	for k := range txInSelectQueries {
		types = append(types, k)
	}
	if params.ActionType != "" {
		types = strings.Split(params.ActionType, ",")
	}

	var addresses []string
	if params.Address != "" {
		addresses = strings.Split(params.Address, ",")
		if MaxAddresses < len(addresses) {
			return oapigen.ActionsResponse{}, miderr.BadRequestF(
				"too many addresses: %d provided, maximum is %d",
				len(addresses), MaxAddresses)
		}
	}

	// EXECUTE QUERIES
	countPS, resultsPS, err := actionsPreparedStatements(
		moment,
		params.TXId,
		addresses,
		params.Asset,
		types,
		limit,
		offset)
	if err != nil {
		return oapigen.ActionsResponse{}, err
	}

	// Get count
	countRows, err := db.Query(ctx, countPS.Query, countPS.Values...)
	if err != nil {
		return oapigen.ActionsResponse{}, fmt.Errorf("actions count query: %w", err)
	}
	defer countRows.Close()
	var txCount uint
	countRows.Next()
	err = countRows.Scan(&txCount)
	if err != nil {
		return oapigen.ActionsResponse{}, fmt.Errorf("actions count read: %w", err)
	}

	// Get results subset
	rows, err := db.Query(ctx, resultsPS.Query, resultsPS.Values...)
	if err != nil {
		return oapigen.ActionsResponse{}, fmt.Errorf("actions query: %w", err)
	}
	defer rows.Close()

	// PROCESS RESULTS
	actions := []action{}

	for rows.Next() {
		var result actionQueryResult
		err := rows.Scan(
			&result.txID,
			&result.fromAddr,
			&result.txID_2nd,
			&result.fromAddr_2nd,
			&result.toAddr,
			&result.asset,
			&result.assetE8,
			&result.asset_2nd,
			&result.asset_2nd_E8,
			&result.pool,
			&result.pool_2nd,
			&result.liquidityFee,
			&result.liquidityUnits,
			&result.swapSlip,
			&result.swapTarget,
			&result.asymmetry,
			&result.basisPoints,
			&result.emitAssetE8,
			&result.emitRuneE8,
			&result.text,
			&result.eventType,
			&result.blockTimestamp,
			&result.height,
		)
		if err != nil {
			return oapigen.ActionsResponse{}, fmt.Errorf("actions read: %w", err)
		}

		action, err := actionProcessQueryResult(ctx, result)
		if err != nil {
			return oapigen.ActionsResponse{}, fmt.Errorf("actions process: %w", err)
		}

		actions = append(actions, action)
	}

	oapigenActions := make([]oapigen.Action, len(actions))
	for i, action := range actions {
		oapigenActions[i] = action.toOapigen()
	}
	return oapigen.ActionsResponse{Count: util.IntStr(int64(txCount)), Actions: oapigenActions}, nil
}

// Helper structs to build needed queries
// Query key is used in the query to then be replaced when parsed
// This way arguments can be dynamically inserted in query strings
type namedSqlValue struct {
	QueryKey string
	Value    interface{}
}

type preparedSqlStatement struct {
	Query  string
	Values []interface{}
}

// Builds prepared statements for Actions lookup. Two queries are needed, one to get the count
// of the total entries for the query, and one to get the subset that will actually be
// returned to the caller.
// The two queries are built form a base query with the structure:
// SELECT * FROM (inTxType1Query UNION_ALL inTxType2Query...inTxTypeNQuery) WHERE <<conditions>>
func actionsPreparedStatements(moment time.Time,
	txid string,
	addresses []string,
	asset string,
	types []string,
	limit,
	offset uint64) (preparedSqlStatement, preparedSqlStatement, error) {
	var countPS, resultsPS preparedSqlStatement
	// Initialize query param slices (to dynamically insert query params)
	baseValues := make([]namedSqlValue, 0)
	subsetValues := make([]namedSqlValue, 0)

	baseValues = append(baseValues, namedSqlValue{"#MOMENT#", moment.UnixNano()})
	subsetValues = append(subsetValues, namedSqlValue{"#LIMIT#", limit}, namedSqlValue{"#OFFSET#", offset})

	// Build select part of the query by taking the tx in queries from the selected types
	// and joining them using UNION ALL
	usedSelectQueries := make([]string, 0)
	for _, eventType := range types {
		q := txInSelectQueries[eventType]
		if q == nil {
			return countPS, resultsPS, fmt.Errorf("invalid type %q", eventType)
		}
		usedSelectQueries = append(usedSelectQueries, q...)
	}
	unionQuery := strings.Join(usedSelectQueries, " UNION ALL ")
	// Replace all #RUNE# values with actual asset
	unionQuery = strings.ReplaceAll(unionQuery, "#RUNE#", "'"+record.RuneAsset()+"'")

	countQuery := "SELECT count(1) FROM (" + unionQuery + ") AS union_results"
	selectQuery := `SELECT union_results.*, block_log.height FROM (` + unionQuery + `) AS union_results
		INNER JOIN block_log ON union_results.block_timestamp = block_log.timestamp`

	joinsQuery := ""
	addOutboundJoin := func(condition string, name string) {
		joinsQuery += " LEFT OUTER JOIN (SELECT in_tx, count(1) AS c FROM outbound_events WHERE " + condition + " GROUP BY in_tx) AS " + name + " ON union_results.tx = " + name + ".in_tx "
	}

	// build WHERE clause applied to the union_all result, based on filter arguments
	// (txid, address, asset)
	whereQuery := `
	WHERE union_results.block_timestamp <= #MOMENT#`

	if txid != "" {
		baseValues = append(baseValues, namedSqlValue{"#TXID#", strings.ToUpper(txid)})
		addOutboundJoin("tx = #TXID#", "o_tx")
		whereQuery += ` AND (
			union_results.tx = #TXID# OR
			union_results.tx_2nd = #TXID# OR
			o_tx.c > 0
		)`
	}

	if 0 < len(addresses) {
		baseValues = append(baseValues, namedSqlValue{"#ADDRESS#", addresses})
		addOutboundJoin("to_addr = ANY(#ADDRESS#) OR from_addr = ANY(#ADDRESS#)", "o_address")
		whereQuery += ` AND (
			union_results.to_addr = ANY(#ADDRESS#) OR
			union_results.from_addr = ANY(#ADDRESS#) OR
			union_results.from_addr_2nd = ANY(#ADDRESS#) OR
			o_address.c > 0
		)`
	}

	if asset != "" {
		baseValues = append(baseValues, namedSqlValue{"#ASSET#", asset})
		addOutboundJoin("asset = #ASSET#", "o_asset")
		whereQuery += ` AND (
			union_results.asset = #ASSET# OR
			union_results.asset_2nd = #ASSET# OR
			o_asset.c > 0
		)`
	}

	// build subset query for the results being shown (based on limit and offset)
	subsetQuery := `
	ORDER BY union_results.block_timestamp DESC
	LIMIT #LIMIT#
	OFFSET #OFFSET#
	`
	// build and return final queries
	countQuery = countQuery + joinsQuery + whereQuery
	countQueryValues := make([]interface{}, 0)
	for i, queryValue := range baseValues {
		position := i + 1
		positionLabel := fmt.Sprintf("$%d", position)
		countQuery = strings.ReplaceAll(countQuery, queryValue.QueryKey, positionLabel)
		countQueryValues = append(countQueryValues, queryValue.Value)
	}
	countPS = preparedSqlStatement{countQuery, countQueryValues}

	resultsQuery := selectQuery + joinsQuery + whereQuery + subsetQuery
	resultsQueryValues := make([]interface{}, 0)
	for i, queryValue := range append(baseValues, subsetValues...) {
		position := i + 1
		positionLabel := fmt.Sprintf("$%d", position)
		resultsQuery = strings.ReplaceAll(resultsQuery, queryValue.QueryKey, positionLabel)
		resultsQueryValues = append(resultsQueryValues, queryValue.Value)
	}
	resultsPS = preparedSqlStatement{resultsQuery, resultsQueryValues}

	return countPS, resultsPS, nil
}

type actionQueryResult struct {
	txID           string
	fromAddr       string
	txID_2nd       string
	fromAddr_2nd   string
	toAddr         string
	asset          sql.NullString
	assetE8        int64
	asset_2nd      sql.NullString
	asset_2nd_E8   int64
	pool           sql.NullString
	pool_2nd       sql.NullString
	liquidityFee   int64
	liquidityUnits int64
	swapSlip       int64
	swapTarget     int64
	asymmetry      float64
	basisPoints    int64
	emitAssetE8    int64
	emitRuneE8     int64
	text           string
	eventType      string
	blockTimestamp int64
	height         int64
}

func actionProcessQueryResult(ctx context.Context, result actionQueryResult) (action, error) {
	// build incoming related transactions
	var inTxs []transaction

	// Handle addLiquidity with a single transaction for both assets and the rest of the events
	// (They all have a single in Tx)
	if result.eventType != "addLiquidity" || result.txID == result.txID_2nd {
		inTx := transaction{
			address: result.fromAddr,
			txID:    result.txID,
		}
		if result.asset.Valid && result.assetE8 > 0 {
			inTx.coins = append(inTx.coins, coin{amount: result.assetE8, asset: result.asset.String})
		}
		if result.asset_2nd.Valid && result.asset_2nd_E8 > 0 {
			inTx.coins = append(inTx.coins, coin{amount: result.asset_2nd_E8, asset: result.asset_2nd.String})
		}
		inTxs = []transaction{inTx}
	} else {
		// Handle addLiquidity with separate transactions per asset
		if result.txID != "" {
			inTx1 := transaction{
				address: result.fromAddr,
				txID:    result.txID,
				coins:   coinList{{amount: result.assetE8, asset: result.asset.String}},
			}
			inTxs = append(inTxs, inTx1)
		}
		if result.txID_2nd != "" {
			inTx2 := transaction{
				address: result.fromAddr_2nd,
				txID:    result.txID_2nd,
				coins:   coinList{{amount: result.asset_2nd_E8, asset: result.asset_2nd.String}},
			}
			inTxs = append(inTxs, inTx2)
		}
	}

	// get outbounds and network fees
	outTxs := []transaction{}
	var networkFees coinList
	switch result.eventType {
	case "swap", "refund", "withdraw":
		var err error
		outTxs, networkFees, err = getOutboundsAndNetworkFees(ctx, result)
		if err != nil {
			return action{}, err
		}
	case "switch":
		outTxs = []transaction{{
			address: result.toAddr,
			coins: []coin{{
				asset:  record.RuneAsset(),
				amount: result.assetE8,
			}},
		}}
	}

	// process status
	status := "pending"
	switch result.eventType {
	case "swap":
		// There might be multiple outs. Maybe we could check if the full sum was sent out.
		// toe8 of last swap (1st or 2nd) <= sum(outTxs.coin.amount) + networkfee.amount
		// We would need to query toe8 in txInSelectQueries.
		if 1 <= len(outTxs) {
			status = "success"
		}
	case "refund":
		// success: either fee is greater than in amount or both
		// outbound and fees are present.
		// TODO(elfedy): Sometimes fee + outbound not equals in amount
		// The resons behind this must be investigated
		inBalances := make(map[string]int64)
		outBalances := make(map[string]int64)
		outFees := make(map[string]int64)

		for _, tx := range inTxs {
			for _, coin := range tx.coins {
				inBalances[coin.asset] = coin.amount
			}
		}
		for _, tx := range outTxs {
			for _, coin := range tx.coins {
				outBalances[coin.asset] = coin.amount
			}
		}
		for _, coin := range networkFees {
			outFees[coin.asset] = coin.amount
		}

		status = "success"
		for k, inBalance := range inBalances {
			if inBalance > outFees[k] && outBalances[k] == 0 {
				status = "pending"
				break
			}
		}
	case "withdraw":
		var runeOut, assetOut, runeFee, assetFee int64
		for _, tx := range outTxs {
			for _, coin := range tx.coins {
				if coin.asset == result.pool.String {
					assetOut = coin.amount
				} else {
					runeOut = coin.amount
				}
			}
		}
		for _, coin := range networkFees {
			if coin.asset == result.pool.String {
				assetFee = coin.amount
			} else {
				runeFee = coin.amount
			}
		}
		runeOk := result.emitRuneE8 <= runeFee || runeOut != 0
		assetOk := result.emitAssetE8 <= assetFee || assetOut != 0
		if runeOk && assetOk {
			status = "success"
		}
	case "addLiquidity":
		if result.text != "pending" {
			status = "success"
		}
	case "donate", "switch":
		status = "success"
	}

	// process pools
	pools := []string{}
	if result.pool.Valid {
		pools = append(pools, result.pool.String)
	}
	if result.pool_2nd.Valid && result.pool_2nd != result.pool {
		pools = append(pools, result.pool_2nd.String)
	}

	// Build metadata
	metadata := oapigen.Metadata{}

	switch result.eventType {
	case "swap":
		metadata.Swap = &oapigen.SwapMetadata{
			LiquidityFee: util.IntStr(result.liquidityFee),
			SwapSlip:     util.IntStr(result.swapSlip),
			SwapTarget:   util.IntStr(result.swapTarget),
			NetworkFees:  networkFees.toOapigen(),
		}
	case "addLiquidity":
		if result.liquidityUnits != 0 {
			metadata.AddLiquidity = &oapigen.AddLiquidityMetadata{
				LiquidityUnits: util.IntStr(result.liquidityUnits),
			}
		}
	case "withdraw":
		metadata.Withdraw = &oapigen.WithdrawMetadata{
			LiquidityUnits: util.IntStr(result.liquidityUnits),
			Asymmetry:      floatStr(result.asymmetry),
			BasisPoints:    util.IntStr(result.basisPoints),
			NetworkFees:    networkFees.toOapigen(),
		}
	case "refund":
		metadata.Refund = &oapigen.RefundMetadata{
			NetworkFees: networkFees.toOapigen(),
			Reason:      result.text,
		}
	}

	action := action{
		eventType: result.eventType,
		date:      result.blockTimestamp,
		height:    result.height,
		metadata:  metadata,
		in:        inTxs,
		out:       outTxs,
		pools:     pools,
		status:    status,
	}

	return action, nil
}

func getOutboundsAndNetworkFees(ctx context.Context, result actionQueryResult) ([]transaction, coinList, error) {
	blockTime := time.Unix(0, result.blockTimestamp)
	outboundTimeLower := blockTime.UnixNano()
	outboundTimeUpper := blockTime.Add(OutboundTimeout).UnixNano()

	// Get and process outbound transactions (from vault address to external address)
	outboundsQuery := `
	SELECT
	tx,
	to_addr,
	asset,
	asset_E8
	FROM outbound_events
	WHERE in_tx = $1 AND $2 <= block_timestamp AND block_timestamp < $3
	`

	networkFeesQuery := `
	SELECT
	asset,
	asset_E8
	FROM fee_events
	WHERE tx = $1 AND $2 <= block_timestamp AND block_timestamp < $3
	`

	outboundRows, err := db.Query(ctx, outboundsQuery, result.txID, outboundTimeLower, outboundTimeUpper)
	if err != nil {
		return nil, nil, fmt.Errorf("outbound tx lookup: %w", err)
	}
	defer outboundRows.Close()

	networkFeeRows, err := db.Query(ctx, networkFeesQuery, result.txID, outboundTimeLower, outboundTimeUpper)
	if err != nil {
		return nil, nil, fmt.Errorf("network fee lookup: %w", err)
	}
	defer networkFeeRows.Close()

	outTxs := []transaction{}

	for outboundRows.Next() {
		var tx sql.NullString
		var address, asset string
		var assetE8 int64

		err := outboundRows.Scan(&tx, &address, &asset, &assetE8)
		if err != nil {
			return nil, nil, fmt.Errorf("outbound tx lookup: %w", err)
		}

		// NOTE: Only out transactions that go to users are shown, so
		// internal double swap transaction is omitted.
		// Double swap middle transaction is the only native out tx (blank ID)
		// in that operation
		isDoubleSwap := result.eventType == "swap" && result.pool_2nd.Valid
		isSynth := record.GetCoinType([]byte(asset)) == record.AssetSynth
		if (tx.Valid || !isDoubleSwap) || (isSynth && isDoubleSwap) {
			txHash := blankTxId
			if tx.Valid {
				txHash = tx.String
			}
			outTx := transaction{
				address: address,
				coins:   coinList{{amount: assetE8, asset: asset}},
				txID:    txHash,
			}
			outTxs = append(outTxs, outTx)
		}
	}

	networkFees := coinList{}

	for networkFeeRows.Next() {
		var asset string
		var assetE8 int64

		err := networkFeeRows.Scan(&asset, &assetE8)
		if err != nil {
			return nil, nil, fmt.Errorf("network fee lookup: %w", err)
		}
		networkFee := coin{
			amount: assetE8,
			asset:  asset,
		}
		networkFees = append(networkFees, networkFee)
	}

	return outTxs, networkFees, nil
}

func init() {
	db.RegisterWatermarkedMaterializedView("swap_actions", `
		-- Simple swap (unique txid)
		SELECT
			tx,
			from_addr,
			'' as tx_2nd,
			'' as from_addr_2nd,
			to_addr,
			from_asset as asset,
			from_E8 as asset_E8,
			'' as asset_2nd,
			0 as asset_2nd_E8,
			pool,
			NULL as pool_2nd,
			liq_fee_in_rune_E8,
			0 as stake_units,
			swap_slip_BP,
			to_E8_min as swap_target,
			0 as asymmetry,
			0 as basis_points,
			0 as emit_asset_E8,
			0 as emit_rune_E8,
			'' as text,
			'swap' as type,
			block_timestamp
		FROM swap_events AS single_swaps
		WHERE NOT EXISTS (
			SELECT tx FROM swap_events WHERE block_timestamp = single_swaps.block_timestamp AND tx = single_swaps.tx AND from_asset <> single_swaps.from_asset
		)
		UNION ALL
		-- Double swap (same txid in different pools)
		SELECT
			swap_in.tx as tx,
			swap_in.from_addr as from_addr,
			'' as tx_2nd,
			'' as from_addr_2nd,
			swap_in.to_addr as to_addr,
			swap_in.from_asset as asset,
			swap_in.from_E8 as asset_E8,
			NULL as asset_2nd,
			0 as asset_2nd_E8,
			swap_in.pool as pool,
			swap_out.pool as pool_2nd,
			(swap_in.liq_fee_in_rune_E8 + swap_out.liq_fee_in_rune_E8) as liq_fee_E8,
			0 as stake_units,
			(swap_in.swap_slip_BP + swap_out.swap_slip_BP
				- (swap_in.swap_slip_BP*swap_out.swap_slip_BP)/10000) as swap_slip_BP,
			swap_out.to_E8_min as swap_target,
			0 as asymmetry,
			0 as basis_points,
			0 as emit_asset_E8,
			0 as emit_rune_E8,
			'' as text,
			'swap' as type,
			swap_in.block_timestamp as block_timestamp
		FROM swap_events AS swap_in
		INNER JOIN swap_events AS swap_out
		ON swap_in.tx = swap_out.tx AND swap_in.block_timestamp = swap_out.block_timestamp
		WHERE swap_in.from_asset <> swap_out.to_asset AND swap_in.to_asset = 'THOR.RUNE' AND swap_out.from_asset = 'THOR.RUNE'
	`)
}

// txIn select queries: list of queries that have inbound
// transactions as rows. They are given a type based on the operation they relate to.
// These queries are built using data from events sent by Thorchain
var txInSelectQueries = map[string][]string{
	"swap": {
		`SELECT * FROM midgard_agg.swap_actions_combined`,
	},
	"addLiquidity": {
		// Get liquidity already added to the pools
		`SELECT
			COALESCE(rune_tx, '') as tx,
			COALESCE(rune_addr, '') as from_addr,
			COALESCE(asset_tx, '') as tx_2nd,
			COALESCE(asset_addr, '') as from_addr_2nd,
			'' as to_addr,
			#RUNE# as asset,
			rune_E8 as asset_E8,
			pool as asset_2nd,
			asset_E8 as asset_2nd_E8,
			pool,
			NULL as pool_2nd,
			0 as liq_fee_E8,
			stake_units,
			0 as swap_slip_BP,
			0 as swap_target,
			0 as asymmetry,
			0 as basis_points,
			0 as emit_asset_E8,
			0 as emit_rune_E8,
			'' as text,
			'addLiquidity' as type,
			block_timestamp
		FROM stake_events`,
		// Get pending liquidity, it will be added when the other asset arrives.
		// There is no partial addition or withdraw of pending liquidity. Once a corresponding
		// add_liquidity or withdraw event arrives all of the pending asset is pushed to the depths
		// or returned, there is no need to check the amounts.
		`SELECT
			COALESCE(rune_tx, '') as tx,
			COALESCE(rune_addr, '') as from_addr,
			COALESCE(asset_tx, '') as tx_2nd,
			COALESCE(asset_addr, '') as from_addr_2nd,
			'' as to_addr,
			'THOR.RUNE' as asset,
			rune_E8 as asset_E8,
			pool as asset_2nd,
			asset_E8 as asset_2nd_E8,
			pool,
			NULL as pool_2nd,
			0 as liq_fee_E8,
			0 as stake_units,
			0 as swap_slip_BP,
			0 as swap_target,
			0 as asymmetry,
			0 as basis_points,
			0 as emit_asset_E8,
			0 as emit_rune_E8,
			'pending' as text,
			'addLiquidity' as type,
			block_timestamp
		FROM pending_liquidity_events AS p
		WHERE pending_type = 'add'
			AND NOT EXISTS(SELECT *
				FROM stake_events AS s
				WHERE
					p.rune_addr = s.rune_addr
					AND p.pool=s.pool
					AND p.block_timestamp <= s.block_timestamp)
			AND NOT EXISTS(SELECT *
				FROM pending_liquidity_events AS pw
				WHERE
					pw.pending_type = 'withdraw'
					AND p.rune_addr = pw.rune_addr
					AND p.pool = pw.pool
					AND p.block_timestamp <= pw.block_timestamp)`,
	},
	"withdraw": {
		`SELECT
			tx,
			from_addr,
			'' as tx_2nd,
			'' as from_addr_2nd,
			to_addr,
			asset,
			asset_E8,
			'' as asset_2nd,
			0 as asset_2nd_E8,
			pool,
			NULL as pool_2nd,
			0 as liq_fee_E8,
			(stake_units * -1) as stake_units,
			0 as swap_slip_BP,
			0 as swap_target,
			asymmetry,
			basis_points,
			emit_asset_E8,
			emit_rune_E8,
			'' as text,
			'withdraw' as type,
			block_timestamp
		FROM unstake_events`,
	},
	"donate": {
		`SELECT
			tx,
			from_addr,
			'' as tx_2nd,
			'' as from_addr_2nd,
			to_addr,
			asset,
			asset_E8,
			#RUNE# as asset_2nd,
			rune_E8 as asset_2nd_E8,
			pool,
			NULL as pool_2nd,
			0 as liq_fee_E8,
			0 as stake_units,
			0 as swap_slip_BP,
			0 as swap_target,
			0 as asymmetry,
			0 as basis_points,
			0 as emit_asset_E8,
			0 as emit_rune_E8,
			'' as text,
			'donate' as type,
			block_timestamp
		FROM add_events`,
	},
	"refund": {
		`SELECT
			tx,
			from_addr,
			'' as tx_2nd,
			'' as from_addr_2nd,
			to_addr,
			asset,
			asset_E8,
			asset_2nd,
			asset_2nd_E8,
			NULL as pool,
			NULL as pool_2nd,
			0 as liq_fee_E8,
			0 as stake_units,
			0 as swap_slip_BP,
			0 as swap_target,
			0 as asymmetry,
			0 as basis_points,
			0 as emit_asset_E8,
			0 as emit_rune_E8,
			reason as text,
			'refund' as type,
			block_timestamp
		FROM refund_events`,
	},
	"switch": {
		`SELECT
				'' as tx,
				from_addr,
				'' as tx_2nd,
				'' as from_addr_2nd,
				to_addr,
				burn_asset as asset,
				burn_e8 as asset_E8,
				'' as asset_2nd,
				0 as asset_2nd_E8,
				NULL as pool,
				NULL as pool_2nd,
				0 as liq_fee_E8,
				0 as stake_units,
				0 as swap_slip_BP,
				0 as swap_target,
				0 as asymmetry,
				0 as basis_points,
				0 as emit_asset_E8,
				0 as emit_rune_E8,
				'' as text,
				'switch' as type,
				block_timestamp
			FROM switch_events`,
	},
}
