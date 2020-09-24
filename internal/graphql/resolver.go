//go:generate go run github.com/99designs/gqlgen

package graphql

import (
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
)

var (
	getAssetAndRuneDepths = timeseries.AssetAndRuneDepths
	getPoolStatus         = timeseries.PoolStatus
	getPools              = timeseries.Pools

	poolStakesLookup   = stat.PoolStakesLookup
	poolUnstakesLookup = stat.PoolUnstakesLookup

	poolStakesBucketsLookup = stat.PoolStakesBucketsLookup

	poolSwapsFromRuneBucketsLookup = stat.PoolSwapsFromRuneBucketsLookup
	poolSwapsToRuneBucketsLookup   = stat.PoolSwapsToRuneBucketsLookup
)

type Resolver struct {
}

//TODO cache repeated db calls to improve efficiency like stat.PoolStakesLookup, UnstakeLookup etc
