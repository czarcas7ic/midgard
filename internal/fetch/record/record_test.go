package record_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func checkDepths(t *testing.T, pool string, assetE8, runeE8 int64) {
	body := testdb.CallJSON(t, "http://localhost:8080/v2/pool/"+pool)
	var jsonApiResponse oapigen.PoolResponse
	testdb.MustUnmarshal(t, body, &jsonApiResponse)

	require.Equal(t, "BTC.BTC", jsonApiResponse.Asset)

	assert.Equal(t, strconv.FormatInt(assetE8, 10), jsonApiResponse.AssetDepth, "Bad Asset depth")
	assert.Equal(t, strconv.FormatInt(runeE8, 10), jsonApiResponse.RuneDepth, "Bad Rune depth")
}

func TestSimpleSwap(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 1000, RuneAmount: 2000},
		testdb.PoolActivate{Pool: "BTC.BTC"})
	checkDepths(t, "BTC.BTC", 1000, 2000)

	blocks.NewBlock(t, "2021-01-02 00:00:00", testdb.Swap{
		Pool:      "BTC.BTC",
		Coin:      "100 BTC.BTC",
		EmitAsset: "200 THOR.RUNE",
	})
	checkDepths(t, "BTC.BTC", 1100, 1800)

	blocks.NewBlock(t, "2021-01-03 00:00:00", testdb.Swap{
		Pool:      "BTC.BTC",
		Coin:      "20 THOR.RUNE",
		EmitAsset: "10 BTC.BTC",
	})
	checkDepths(t, "BTC.BTC", 1090, 1820)
}

func TestSynthSwap(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 1000, RuneAmount: 2000},
		testdb.PoolActivate{Pool: "BTC.BTC"})
	checkDepths(t, "BTC.BTC", 1000, 2000)

	blocks.NewBlock(t, "2021-01-02 00:00:00", testdb.Swap{
		Pool:      "BTC.BTC",
		Coin:      "100 BTC/BTC",
		EmitAsset: "200 THOR.RUNE",
	})
	checkDepths(t, "BTC.BTC", 1000, 1800)

	blocks.NewBlock(t, "2021-01-03 00:00:00", testdb.Swap{
		Pool:      "BTC.BTC",
		Coin:      "100 THOR.RUNE",
		EmitAsset: "50 BTC/BTC",
	})
	checkDepths(t, "BTC.BTC", 1000, 1900)
}

func TestSwapErrors(t *testing.T) {
	// TODO(muninn): disable error logging

	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 1000, RuneAmount: 2000},
		testdb.PoolActivate{Pool: "BTC.BTC"})
	checkDepths(t, "BTC.BTC", 1000, 2000)

	// Unkown from pool
	blocks.NewBlock(t, "2021-01-02 00:00:00",
		// Unkown from pool
		testdb.Swap{
			Pool:      "BTC.BTC",
			Coin:      "1 BTC?BTC",
			EmitAsset: "2 THOR.RUNE",
		},
		// Both is rune
		testdb.Swap{
			Pool:      "BTC.BTC",
			Coin:      "10 THOR.RUNE",
			EmitAsset: "20 THOR.RUNE",
		},
		// None is rune
		testdb.Swap{
			Pool:      "BTC.BTC",
			Coin:      "100 BTC.BTC",
			EmitAsset: "200 BTC/BTC",
		},
	)

	// Depths didn't change
	checkDepths(t, "BTC.BTC", 1000, 2000)
}
