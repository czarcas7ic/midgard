package decimal

import (
	_ "embed"
	"encoding/json"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

//go:embed decimals.json
var decimalString string
var poolsDecimal map[string]SingleResult

type SingleResult struct {
	NativeDecimals int64    `json:"decimals"` // -1 means that only the asset name was observed without the decimal count.
	AssetSeen      []string `json:"asset_seen"`
	DecimalSource  []string `json:"decimal_source"`
}

func init() {
	err := json.Unmarshal([]byte(decimalString), &poolsDecimal)
	if err != nil {
		midlog.FatalE(err, "There is no decimals.json file to open. please run the decimal script first: `go run ./cmd/decimal` to get the native decimal values in the pools endpoint")
	}
}

//This function will overwrite nativeDecimals in the decimals.json file by reading from config.
func AddConfigDecimals() {
	envDecimals := config.Global.PoolsDecimal

	for pool, decimal := range envDecimals {
		// default constructed poolDecimal just works too
		poolDecimal := poolsDecimal[pool]
		poolsDecimal[pool] = SingleResult{
			NativeDecimals: decimal,
			AssetSeen:      append(poolDecimal.AssetSeen, "enviroment"),
			DecimalSource:  append(poolDecimal.DecimalSource, "enviroment"),
		}
		midlog.InfoF("%s pool decimals has been overwritten by config.", pool)
	}
}

func PoolsDecimal() map[string]SingleResult {
	return poolsDecimal
}
