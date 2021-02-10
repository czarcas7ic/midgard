package stat

import (
	"fmt"
	"math"
	"net/http"

	"gitlab.com/thorchain/midgard/internal/timeseries"
)

var usdPoolWhitelist = []string{"BNB.BUSD-BAF", "ETH.USDT-0X62E273709DA575835C7F6AEF4A31140CA5B1D190"}

// Returns the 1/price from the depest whitelisted pool.
func RunePriceUSD() float64 {
	ret := math.NaN()
	var maxdepth int64 = -1

	state := timeseries.Latest.GetState()
	for _, pool := range usdPoolWhitelist {
		poolInfo := state.PoolInfo(pool)
		if poolInfo != nil && maxdepth < poolInfo.RuneDepth {
			maxdepth = poolInfo.RuneDepth
			ret = 1 / poolInfo.Price()
		}
	}
	return ret
}

func ServeUSDDebug(resp http.ResponseWriter, req *http.Request) {
	state := timeseries.Latest.GetState()
	for _, pool := range usdPoolWhitelist {
		poolInfo := state.PoolInfo(pool)
		if poolInfo == nil {
			fmt.Fprintf(resp, "%s - pool not found\n", pool)
		} else {
			depth := float64(poolInfo.RuneDepth) / 1e8
			runePrice := 1 / poolInfo.Price()
			fmt.Fprintf(resp, "%s - runeDepth: %.0f runePriceUsd: %.2f\n", pool, depth, runePrice)
		}
	}

	fmt.Fprintf(resp, "\n\nrunePriceUSD: %v", RunePriceUSD())
}