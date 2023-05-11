package timeseries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

const DefaultLimit = 50

func floatStr(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

type action struct {
	pools      []string
	actionType string
	status     string
	in         []transaction
	out        []transaction
	date       int64
	height     int64
	metadata   oapigen.Metadata
	eventId    int64
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
		Type:     oapigen.ActionType(a.actionType),
		Status:   oapigen.ActionStatus(a.status),
		In:       oapigenIn,
		Out:      oapigenOut,
		Date:     util.IntStr(a.date),
		Height:   util.IntStr(a.height),
		Metadata: a.metadata,
	}
}

type transaction struct {
	Address string   `json:"address"`
	Coins   coinList `json:"coins"`
	TxID    string   `json:"txID"`
}

func (tx transaction) toOapigen() oapigen.Transaction {
	return oapigen.Transaction{
		Address: tx.Address,
		TxID:    tx.TxID,
		Coins:   tx.Coins.toOapigen(),
	}
}

type transactionList []transaction

func (a *transactionList) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &a)
}

type coin struct {
	Asset  string `json:"asset"`
	Amount int64  `json:"amount"`
}

func (c coin) toOapigen() oapigen.Coin {
	return oapigen.Coin{
		Asset:  c.Asset,
		Amount: util.IntStr(c.Amount),
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

func (a *coinList) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &a)
}

// Combination of possible fields the jsonb `meta` column can have
type actionMeta struct {
	// refund:
	Reason string `json:"reason"`
	// withdraw:
	Asymmetry      float64 `json:"asymmetry"`
	BasisPoints    int64   `json:"basisPoints"`
	ImpLossProt    int64   `json:"impermanentLossProtection"`
	LiquidityUnits int64   `json:"liquidityUnits"`
	EmitAssetE8    int64   `json:"emitAssetE8"`
	EmitRuneE8     int64   `json:"emitRuneE8"`
	// swap:
	SwapSingle       bool   `json:"swapSingle"`
	LiquidityFee     int64  `json:"liquidityFee"`
	SwapTarget       int64  `json:"swapTarget"`
	SwapSlip         int64  `json:"swapSlip"`
	AffiliateFee     int64  `json:"affiliateFee"`
	AffiliateAddress string `json:"affiliateAddress"`
	Memo             string `json:"memo"`
	// addLiquidity:
	Status string `json:"status"`
	// also LiquidityUnits
}

// TODO(huginn): switch to using native pgx interface, this would allow us to scan
// jsonb and array data automatically, without writing these methods and using libpq.
// It's also more efficient.
func (a *actionMeta) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, &a)
	case nil:
		*a = actionMeta{}
		return nil
	}

	return errors.New("unsupported scan type for actionMeta")
}

type ActionsParams struct {
	Limit         string
	NextPageToken string
	PrevPageToken string
	Timestamp     string
	Height        string
	FromTimestamp string
	FromHeight    string
	Offset        string
	ActionType    string
	Address       string
	TXId          string
	Asset         string
	Affiliate     string
}

type parsedActionsParams struct {
	limit            uint64
	nextPageToken    uint64
	prevPageToken    uint64
	timestamp        uint64
	height           uint64
	fromTimestamp    uint64
	fromHeight       uint64
	offset           uint64
	types            []string
	addresses        []string
	txid             string
	assets           []string
	affiliateAddress string
	oldAction        bool
}

func isMutuallyExclusive(p map[string]string) error {
	count := 0
	errFmt := "Parameters "
	for key, val := range p {
		if val != "" {
			count++
		}
		errFmt += fmt.Sprintf("'%s' ", key)
	}
	if count > 1 {
		errFmt += "shouldn't be coming together"
		return errors.New(errFmt)
	}
	return nil
}

func (p parsedActionsParams) IsReverseLookup() bool {
	return p.prevPageToken != 0 || p.fromHeight != 0 || p.fromTimestamp != 0
}

func (p ActionsParams) parse() (parsedActionsParams, error) {
	MaxLimit := config.Global.Endpoints.ActionParams.MaxLimit
	MaxAddresses := config.Global.Endpoints.ActionParams.MaxAddresses
	MaxAssets := config.Global.Endpoints.ActionParams.MaxAssets

	var limit uint64
	if p.Limit != "" {
		var err error
		limit, err = strconv.ParseUint(p.Limit, 10, 64)
		if err != nil || limit < 1 || MaxLimit < limit {
			return parsedActionsParams{}, miderr.BadRequestF(
				"'limit' must be an integer between 1 and %d",
				MaxLimit)
		}
	} else {
		limit = DefaultLimit
	}

	var nextPageToken uint64
	if p.NextPageToken != "" {
		var err error
		nextPageToken, err = strconv.ParseUint(p.NextPageToken, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'nextPageToken' must be a non negative integer")
		}
	} else {
		nextPageToken = 0
	}

	var timestamp uint64
	if p.Timestamp != "" {
		var err error
		timestamp, err = strconv.ParseUint(p.Timestamp, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'timestamp' must be a non negative integer")
		}
	} else {
		timestamp = 0
	}

	var height uint64
	if p.Height != "" {
		var err error
		height, err = strconv.ParseUint(p.Height, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'height' must be a non negative integer")
		}
	} else {
		height = 0
	}

	var prevPageToken uint64
	if p.PrevPageToken != "" {
		var err error
		prevPageToken, err = strconv.ParseUint(p.PrevPageToken, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'prevPageToken' must be a non negative integer")
		}
	} else {
		prevPageToken = 0
	}

	var fromTimestamp uint64
	if p.FromTimestamp != "" {
		var err error
		fromTimestamp, err = strconv.ParseUint(p.FromTimestamp, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'fromTimestamp' must be a non negative integer")
		}
	} else {
		fromTimestamp = 0
	}

	var fromHeight uint64
	if p.FromHeight != "" {
		var err error
		fromHeight, err = strconv.ParseUint(p.FromHeight, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'fromHeight' must be a non negative integer")
		}
	} else {
		fromHeight = 0
	}

	var offset uint64
	if p.Offset != "" {
		var err error
		offset, err = strconv.ParseUint(p.Offset, 10, 64)
		if err != nil {
			return parsedActionsParams{}, errors.New("'offset' must be a non-negative integer")
		}
	} else {
		offset = 0
	}

	nextParams := map[string]string{"NextPageToken": p.NextPageToken, "Timestamp": p.Timestamp, "Height": p.Height}
	err := isMutuallyExclusive(nextParams)
	if err != nil {
		return parsedActionsParams{}, err
	}

	prevParams := map[string]string{"PrevPageToken": p.PrevPageToken, "FromTimestamp": p.FromHeight, "FromHeight": p.FromHeight}
	err = isMutuallyExclusive(prevParams)
	if err != nil {
		return parsedActionsParams{}, err
	}

	// set default actions param to be on oldactions
	oldAction := true
	if nextPageToken != 0 || timestamp != 0 || height != 0 || fromHeight != 0 || fromTimestamp != 0 || prevPageToken != 0 {
		oldAction = false
	}

	types := make([]string, 0)
	if p.ActionType != "" {
		types = strings.Split(p.ActionType, ",")
	}

	// check if it's in valid types
	validActions := map[string]bool{"swap": true, "addLiquidity": true, "withdraw": true, "donate": true, "refund": true, "switch": true}
	for _, a := range types {
		if !validActions[a] {
			return parsedActionsParams{}, miderr.BadRequestF(
				"Your request for actions is '%s' and '%s' action type is unknown. Please see the docs",
				p.ActionType, a)
		}
	}

	var addresses []string
	if p.Address != "" {
		addresses = strings.Split(p.Address, ",")
		if MaxAddresses < len(addresses) {
			return parsedActionsParams{}, miderr.BadRequestF(
				"too many addresses: %d provided, maximum is %d",
				len(addresses), MaxAddresses)
		}
	}

	var assets []string
	if p.Asset != "" {
		assets = strings.Split(p.Asset, ",")
		if MaxAssets < len(assets) {
			return parsedActionsParams{}, miderr.BadRequestF(
				"too many assets: %d provided, maximum is %d",
				len(assets), MaxAssets)
		}
	}

	return parsedActionsParams{
		limit:            limit,
		nextPageToken:    nextPageToken,
		prevPageToken:    prevPageToken,
		timestamp:        timestamp,
		height:           height,
		fromTimestamp:    fromTimestamp,
		fromHeight:       fromHeight,
		offset:           offset,
		types:            types,
		addresses:        addresses,
		txid:             p.TXId,
		assets:           assets,
		affiliateAddress: p.Affiliate,
		oldAction:        oldAction,
	}, nil
}

func runActionsQuery(ctx context.Context, q preparedSqlStatement) ([]action, error) {
	rows, err := db.Query(ctx, q.Query, q.Values...)
	if err != nil {
		return nil, fmt.Errorf("actions query: %w", err)
	}
	defer rows.Close()

	actions := []action{}

	for rows.Next() {
		var result action
		var ins transactionList
		var outs transactionList
		var fees coinList
		var meta actionMeta
		err := rows.Scan(
			&result.eventId,
			&result.date,
			&result.actionType,
			pq.Array(&result.pools),
			&ins,
			&outs,
			&fees,
			&meta,
		)
		if err != nil {
			return nil, fmt.Errorf("actions read: %w", err)
		}

		result.height = db.HeightFromEventId(result.eventId)
		result.in = ins
		result.out = outs
		result.completeFromDBRead(&meta, fees)

		actions = append(actions, result)
	}

	return actions, nil
}

func (a *action) completeFromDBRead(meta *actionMeta, fees coinList) {
	if a.pools == nil {
		a.pools = []string{}
	}

	isDoubleSwap := a.actionType == "swap" && !meta.SwapSingle
	outs := []transaction{}
	for _, t := range a.out {
		// When we have a double swap: Asset1->Rune ; Rune->Asset2
		// then we have two outbound_events: one for the Rune and one for Asset2.
		// We hide the Rune outbound and show only the Asset2 outbound to the user.
		// TxID is "" for THOR.RUNE transactions.
		if !(t.TxID == "" && isDoubleSwap) {
			outs = append(outs, t)
		} else {
			isSynth := len(t.Coins) != 0 &&
				record.GetCoinType([]byte(t.Coins[0].Asset)) == record.AssetSynth
			if isSynth {
				outs = append(outs, t)
			}
		}
	}
	a.out = outs

	// process status
	a.status = "success"
	if meta.Status != "" {
		a.status = meta.Status
	}

	switch a.actionType {
	case "swap":
		// There might be multiple outs. Maybe we could check if the full sum was sent out.
		// toe8 of last swap (1st or 2nd) <= sum(outTxs.coin.amount) + networkfee.amount
		// We would need to query toe8 in txInSelectQueries.
		if len(a.out) == 0 {
			a.status = "pending"
		}
	case "refund":
		// success: either fee is greater than in amount or both
		// outbound and fees are present.
		// TODO(elfedy): Sometimes fee + outbound not equals in amount
		// The resons behind this must be investigated
		inBalances := make(map[string]int64)
		outBalances := make(map[string]int64)
		outFees := make(map[string]int64)

		for _, tx := range a.in {
			for _, coin := range tx.Coins {
				inBalances[coin.Asset] = coin.Amount
			}
		}
		for _, tx := range a.out {
			for _, coin := range tx.Coins {
				outBalances[coin.Asset] = coin.Amount
			}
		}
		for _, coin := range fees {
			outFees[coin.Asset] = coin.Amount
		}

		a.status = "success"
		for k, inBalance := range inBalances {
			if inBalance > outFees[k] && outBalances[k] == 0 {
				a.status = "pending"
				break
			}
		}
	case "withdraw":
		var runeOut, assetOut, runeFee, assetFee int64
		for _, tx := range a.out {
			for _, coin := range tx.Coins {
				if coin.Asset != "THOR.RUNE" {
					assetOut = coin.Amount
				} else {
					runeOut = coin.Amount
				}
			}
		}
		for _, coin := range fees {
			if coin.Asset != "THOR.RUNE" {
				assetFee = coin.Amount
			} else {
				runeFee = coin.Amount
			}
		}
		runeOk := meta.EmitRuneE8 <= runeFee || runeOut != 0
		assetOk := meta.EmitAssetE8 <= assetFee || assetOut != 0

		a.status = "pending"
		if runeOk && assetOk {
			a.status = "success"
		}
	default:
	}

	switch a.actionType {
	case "swap":
		a.metadata.Swap = &oapigen.SwapMetadata{
			LiquidityFee:     util.IntStr(meta.LiquidityFee),
			SwapSlip:         util.IntStr(meta.SwapSlip),
			SwapTarget:       util.IntStr(meta.SwapTarget),
			NetworkFees:      fees.toOapigen(),
			AffiliateFee:     util.IntStr(meta.AffiliateFee),
			AffiliateAddress: meta.AffiliateAddress,
			Memo:             meta.Memo,
		}
	case "addLiquidity":
		if meta.LiquidityUnits != 0 {
			a.metadata.AddLiquidity = &oapigen.AddLiquidityMetadata{
				LiquidityUnits: util.IntStr(meta.LiquidityUnits),
			}
		}
	case "withdraw":
		a.metadata.Withdraw = &oapigen.WithdrawMetadata{
			LiquidityUnits:            util.IntStr(meta.LiquidityUnits),
			Asymmetry:                 floatStr(meta.Asymmetry),
			BasisPoints:               util.IntStr(meta.BasisPoints),
			NetworkFees:               fees.toOapigen(),
			ImpermanentLossProtection: util.IntStr(meta.ImpLossProt),
			Memo:                      meta.Memo,
		}
	case "refund":
		a.metadata.Refund = &oapigen.RefundMetadata{
			NetworkFees:      fees.toOapigen(),
			Reason:           meta.Reason,
			Memo:             meta.Memo,
			AffiliateFee:     util.IntStr(meta.AffiliateFee),
			AffiliateAddress: meta.AffiliateAddress,
		}
	}
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

	parsedParams, err := params.parse()
	if err != nil {
		return oapigen.ActionsResponse{}, err
	}

	// Construct queries
	countPS, resultsPS, err := actionsPreparedStatements(moment, parsedParams)
	if err != nil {
		return oapigen.ActionsResponse{}, err
	}

	var totalCount int = -1
	if parsedParams.oldAction {
		// The actions endpoint is slow, often because the count query.
		// Until the new API is implemented we cancel the query if it's slow and return count=-1
		// https://discord.com/channels/838986635756044328/999334588583252078)
		countDeadlineCtx, _ := context.WithDeadline(ctx,
			time.Now().Add(config.Global.TmpActionsCountTimeout.Value()))
		countRows, err := db.Query(countDeadlineCtx, countPS.Query, countPS.Values...)
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				totalCount = -1
			} else {
				return oapigen.ActionsResponse{}, fmt.Errorf("actions count query: %w", err)
			}
		} else {
			defer countRows.Close()
			countRows.Next()
			err = countRows.Scan(&totalCount)
			if err != nil {
				return oapigen.ActionsResponse{}, fmt.Errorf("actions count read: %w", err)
			}
		}
	}

	// Get results
	actions, err := runActionsQuery(ctx, resultsPS)
	if err != nil {
		return oapigen.ActionsResponse{}, err
	}

	oapigenActions := make([]oapigen.Action, len(actions))
	metaData := oapigen.ActionMeta{}
	for i, action := range actions {
		if parsedParams.IsReverseLookup() {
			i = len(actions) - 1 - i
		}

		oapigenActions[i] = action.toOapigen()
		if i == len(actions)-1 {
			metaData.NextPageToken = util.IntStr(action.eventId)
		}
		if i == 0 {
			metaData.PrevPageToken = util.IntStr(action.eventId)
		}
	}

	totalCountStr := util.IntStr(int64(totalCount))
	totalCountPtr := &totalCountStr
	if !parsedParams.oldAction {
		totalCountPtr = nil
	}

	return oapigen.ActionsResponse{
		Count:   totalCountPtr,
		Actions: oapigenActions,
		Meta:    metaData}, nil
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

// Builds SQL statements for Actions lookup. Two queries are needed, one to get the count
// of the total entries for the query, and one to get the subset that will actually be
// returned to the caller.
func actionsPreparedStatements(moment time.Time,
	params parsedActionsParams,
) (preparedSqlStatement, preparedSqlStatement, error) {
	var countPS, resultsPS preparedSqlStatement
	// Initialize query param slices (to dynamically insert query params)
	baseValues := make([]namedSqlValue, 0)
	subsetValues := make([]namedSqlValue, 0)

	baseValues = append(baseValues, namedSqlValue{"#MOMENT#", moment.UnixNano()})
	subsetValues = append(subsetValues, namedSqlValue{"#LIMIT#", params.limit}, namedSqlValue{"#OFFSET#", params.offset})

	forceMainQuerySeparateEvaluation := false

	// build WHERE which is common to both queries, based on filter arguments
	// (types, txid, address, asset)
	whereQuery := `
		WHERE event_id <= nano_event_id_up(#MOMENT#)`

	if len(params.types) != 0 {
		baseValues = append(baseValues, namedSqlValue{"#TYPE#", params.types})
		whereQuery += `
			AND action_type = ANY(#TYPE#)`
	}

	if params.txid != "" {
		forceMainQuerySeparateEvaluation = true
		baseValues = append(baseValues, namedSqlValue{"#TXID#", strings.ToUpper(params.txid)})
		whereQuery += `
			AND transactions @> ARRAY[#TXID#]`
	}

	if len(params.addresses) != 0 {
		baseValues = append(baseValues, namedSqlValue{"#ADDRESSES#", params.addresses})
		whereQuery += `
			AND addresses && #ADDRESSES#`
	}

	if len(params.assets) != 0 {
		baseValues = append(baseValues, namedSqlValue{"#ASSET#", pq.Array(params.assets)})
		whereQuery += `
			AND assets @> #ASSET#`
	}

	if params.affiliateAddress != "" {
		baseValues = append(baseValues, namedSqlValue{"#AFFILIATE#", params.affiliateAddress})
		whereQuery += `
			AND meta->'affiliateAddress' ? #AFFILIATE#`
	}

	if params.nextPageToken != 0 {
		baseValues = append(baseValues, namedSqlValue{"#NEXTPAGETOKEN#", params.nextPageToken})
		whereQuery += `
			AND event_id < #NEXTPAGETOKEN#`
	}

	if params.timestamp != 0 {
		baseValues = append(baseValues, namedSqlValue{"#TIMESTAMP#", params.timestamp})
		whereQuery += `
			AND block_timestamp < #TIMESTAMP#`
	}

	if params.height != 0 {
		baseValues = append(baseValues, namedSqlValue{"#HEIGHT#", db.HeightToEventId(params.height)})
		whereQuery += `
			AND event_id < #HEIGHT#`
	}

	if params.prevPageToken != 0 {
		baseValues = append(baseValues, namedSqlValue{"#PREVPAGETOKEN#", params.prevPageToken})
		whereQuery += `
			AND event_id > #PREVPAGETOKEN#`
	}

	if params.fromTimestamp != 0 {
		baseValues = append(baseValues, namedSqlValue{"#FROMTIMESTAMP#", params.fromTimestamp})
		whereQuery += `
			AND block_timestamp > #FROMTIMESTAMP#`
	}

	if params.fromHeight != 0 {
		baseValues = append(baseValues, namedSqlValue{"#FROMHEIGHT#", db.HeightToEventId(params.fromHeight)})
		whereQuery += `
			AND event_id > #FROMHEIGHT#`
	}

	// build and return final queries
	countQuery := `SELECT count(1) FROM midgard_agg.actions` + whereQuery
	countQueryValues := make([]interface{}, 0)
	for i, queryValue := range baseValues {
		position := i + 1
		positionLabel := fmt.Sprintf("$%d", position)
		countQuery = strings.ReplaceAll(countQuery, queryValue.QueryKey, positionLabel)
		countQueryValues = append(countQueryValues, queryValue.Value)
	}
	countPS = preparedSqlStatement{countQuery, countQueryValues}

	mainQuery := `
		SELECT
			event_id,
			block_timestamp,
			action_type,
			pools,
			ins,
			outs,
			fees,
			meta
		FROM midgard_agg.actions
	` + whereQuery

	// The Postgres' query planner is kinda dumb when we have a `txid` specified.
	// Because we also want to order by `event_id` and limit the number of results,
	// it chooses to do a scan on `event_id` index and filter for rows that have the given
	// txid, instead of using the `transactions` index and sorting afterwards. This is a very bad
	// decision in this case.
	// See https://gitlab.com/thorchain/midgard/-/issues/45 for details.
	//
	// The `OFFSET 0` in a sub-query is a semi-officially blessed hack to stop Postgres from
	// inlining a sub-query; thus forcing it to create an independent plan for it. In which case
	// it obviously uses the index on `transactions`.
	if forceMainQuerySeparateEvaluation {
		mainQuery = `WITH relevant_actions AS (` + mainQuery + `
			OFFSET 0
		)
		SELECT * FROM relevant_actions
		`
	}

	orderQuery := `
		ORDER BY event_id DESC
		LIMIT #LIMIT#
		OFFSET #OFFSET#
	`

	if params.IsReverseLookup() {
		orderQuery = `
			ORDER BY event_id
			LIMIT #LIMIT#
			OFFSET #OFFSET#
		`
	}

	resultsQuery := mainQuery + orderQuery

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
