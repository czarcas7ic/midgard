package decimal

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

//go:embed decimals.json
var decimalString string
var poolsDecimal util.NativeDecimalMap

func init() {
	err := json.Unmarshal([]byte(decimalString), &poolsDecimal)
	if err != nil {
		midlog.FatalE(err, "There is no decimals.json file to open. please run the decimal script first: `go run ./cmd/decimal` to get the native decimal values in the pools endpoint")
	}
}

// This function will overwrite nativeDecimals in the decimals.json file by reading from config.
func AddConfigDecimals() {
	envDecimals := config.Global.PoolsDecimal

	for pool, decimal := range envDecimals {
		// default constructed poolDecimal just works too
		poolDecimal := poolsDecimal[pool]
		poolsDecimal[pool] = util.NativeDecimalSingle{
			NativeDecimals: decimal,
			AssetSeen:      append(poolDecimal.AssetSeen, "enviroment"),
			DecimalSource:  append(poolDecimal.DecimalSource, "enviroment"),
		}
		midlog.InfoF("%s pool decimals has been overwritten by config to %d", pool, decimal)
	}
}

func PoolsDecimal() util.NativeDecimalMap {
	return poolsDecimal
}

func ServeDecimalsDebug(resp http.ResponseWriter, req *http.Request) {
	resp.Header().Set("Content-Type", "application/json")
	e := json.NewEncoder(resp)
	e.SetIndent("", "\t")

	// Error is discarded
	_ = e.Encode(poolsDecimal)
}
