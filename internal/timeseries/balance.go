package timeseries

import (
	"context"
	"database/sql"
	"strconv"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

type BalanceParams struct {
	Height    string
	Timestamp string
}

func (b BalanceParams) GetNano() (db.Nano, error) {
	time, err := strconv.ParseInt(b.Timestamp, 10, 64)
	if err != nil {
		return 0, err
	}
	return db.Second(time).ToNano(), nil
}

func (b BalanceParams) GetHeight() (int64, error) {
	h, err := strconv.ParseInt(b.Height, 10, 64)
	if err != nil {
		return 0, err
	}
	return h, err
}

func GetBalances(ctx context.Context, address string, params BalanceParams) (*oapigen.Balance, error) {
	var err error
	var rows *sql.Rows
	firstBlock := db.FirstBlock.Get()
	lastBlock := db.LastAggregatedBlock.Get()
	if params.Height != "" {
		queryHeight, e := params.GetHeight()
		if e != nil {
			return nil, e
		}
		firstHeight := firstBlock.Height
		lastHeight := lastBlock.Height
		if queryHeight < firstHeight || lastBlock.Height < queryHeight {
			return nil, miderr.NotFoundF("no data for height %d, available height range is [%d,%d]", queryHeight, firstHeight, lastHeight)
		}
		rows, err = queryHistoricalBalanceByHeight(ctx, address, queryHeight)
	} else if params.Timestamp != "" {
		queryNano, e := params.GetNano()
		if e != nil {
			return nil, e
		}
		firstNano := firstBlock.Timestamp
		lastNano := lastBlock.Timestamp
		if queryNano < firstNano || lastNano < queryNano {
			return nil, miderr.NotFoundF("no data for timestamp %d, timestamp range is [%d,%d]", queryNano, firstNano, lastNano)
		}
		rows, err = queryHistoricalBalanceByTimestamp(ctx, address, queryNano)
	} else {
		rows, err = queryCurrentBalance(ctx, address)
	}

	if err != nil {
		return nil, err
	}

	return parseBalanceRows(rows)
}

func queryHistoricalBalanceByHeight(ctx context.Context, address string, height int64) (*sql.Rows, error) {
	var time db.Nano
	err := db.TheDB.QueryRow("SELECT timestamp FROM block_log WHERE height = $1", height).Scan(&time)
	if err != nil {
		return nil, err
	}
	return queryHistoricalBalanceByTimestamp(ctx, address, time)
}

func queryHistoricalBalanceByTimestamp(ctx context.Context, address string, time db.Nano) (*sql.Rows, error) {
	return db.Query(ctx,
		heightstampQuery("$2")+`
		UNION ALL
		SELECT
			c.asset asset,
			b.amount_e8 amount_e8,
			0 AS height,
			0 AS block_timestamp
		FROM
			midgard_agg.current_balances AS c,
			LATERAL (
				SELECT amount_e8
				FROM midgard_agg.balances
				WHERE addr = $1
					AND asset = c.asset
					AND block_timestamp <= $2
				ORDER BY block_timestamp DESC LIMIT 1
			) AS b
		WHERE c.addr = $1
		ORDER BY asset`,
		address,
		time,
	)
}

func queryCurrentBalance(ctx context.Context, address string) (*sql.Rows, error) {
	return db.Query(ctx,
		watermarkQuery()+`
		UNION ALL
		SELECT
			asset,
			amount_e8,
			0 AS height,
			0 AS block_timestamp
		FROM midgard_agg.current_balances
		WHERE addr = $1
		ORDER BY asset`,
		address,
	)
}

func parseBalanceRows(rows *sql.Rows) (*oapigen.Balance, error) {
	balance := oapigen.Balance{Coins: []oapigen.Coin{}}

	if rows.Next() {
		var ignoreAsset, ignoreTs string
		err := rows.Scan(&ignoreAsset, &ignoreTs, &balance.Height, &balance.Timestamp)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var asset, amount, ignoreHeight, ignoreTs string
			err := rows.Scan(&asset, &amount, &ignoreHeight, &ignoreTs)
			if err != nil {
				return nil, err
			}
			balance.Coins = append(balance.Coins, oapigen.Coin{
				Amount: amount,
				Asset:  asset,
			})
		}
	}
	return &balance, nil
}

func watermarkQuery() string {
	return heightstampQuery("(SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = 'balances')")
}

func heightstampQuery(timeFilter string) string {
	return `
	(SELECT
		'' AS asset,
		0 AS amount_e8,
		height,
		timestamp AS block_timestamp
	FROM block_log
	WHERE timestamp <= ` + timeFilter + `
	ORDER BY timestamp DESC
	LIMIT 1)`
}
