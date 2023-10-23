package main

import (
	"encoding/json"
	"os"
	"strings"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

type Genesis struct {
	AppState struct {
		Bank struct {
			Balances []GenesisBalance
		}
	} `json:"app_state"`
	InitialHeight string `json:"initial_height"`
}

type GenesisBalance struct {
	Address string
	Coins   []Coin
}

type Coin struct {
	Amount string
	Denom  string
}

func readThorBalances(thorGenesisPath string) (balances map[string]Balance, height int64, timestamp int64) {
	f, err := os.Open(thorGenesisPath)
	if err != nil {
		midlog.FatalE(err, "Error reading genesis json")
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var g Genesis
	if err := dec.Decode(&g); err != nil {
		midlog.FatalE(err, "Error parsing genesis json")
	}
	balances = g.getBalances()
	height = util.MustParseInt64(g.InitialHeight) - 1
	timestamp = queryTimestampAtHeight(height)
	return
}

func queryTimestampAtHeight(height int64) (timestamp int64) {
	err := db.TheDB.QueryRow(
		`SELECT timestamp FROM block_log WHERE height = $1`,
		height).
		Scan(&timestamp)
	if err != nil {
		midlog.FatalE(err, "Error reading block timestamp from db")
	}

	return
}

func (g Genesis) getBalances() map[string]Balance {
	balances := map[string]Balance{}
	for _, bal := range g.AppState.Bank.Balances {
		for _, coin := range bal.Coins {
			b := Balance{
				addr:     bal.Address,
				asset:    normalizeAsset(coin.Denom),
				amountE8: util.MustParseInt64(coin.Amount),
			}
			balances[b.key()] = b
		}
	}
	return balances
}

func normalizeAsset(asset string) string {
	switch asset {
	case "rune":
		return "THOR.RUNE"
	}
	return strings.ToUpper(asset)
}
