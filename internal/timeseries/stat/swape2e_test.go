package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

// Testing conversion between different pools and gapfill
func TestSwapsHistoryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BNB.BNB", AssetAmount: 1000, RuneAmount: 2000},
		testdb.AddLiquidity{Pool: "BNB.BTCB-1DE", AssetAmount: 1000, RuneAmount: 2000},
	)

	// Swapping BTCB-1DE to 8 rune (4 to, 4 fee) and selling 15 rune on 3rd of September/
	// total fee=4; average slip=2
	blocks.NewBlock(t, "2020-09-03 12:00:00", testdb.Swap{
		Pool:               "BNB.BTCB-1DE",
		EmitAsset:          "6 THOR.RUNE",
		Coin:               "0 BNB.BTCB-1DE",
		LiquidityFeeInRune: 2,
		Slip:               1,
	}, testdb.Swap{
		Pool:               "BNB.BTCB-1DE",
		EmitAsset:          "0 BNB.BTCB-1DE",
		Coin:               "15 THOR.RUNE",
		LiquidityFeeInRune: 4,
		Slip:               3,
	})

	// Swapping BNB to 20 RUNE and selling 50 RUNE on 5th of September
	// total fee=13; average slip=3
	blocks.NewBlock(t, "2020-09-05 12:00:00", testdb.Swap{
		Pool:               "BNB.BNB",
		EmitAsset:          "15 THOR.RUNE",
		Coin:               "0 BNB.BNB",
		LiquidityFeeInRune: 5,
		Slip:               1,
	}, testdb.Swap{
		Pool:               "BNB.BNB",
		EmitAsset:          "0 BNB.BNB",
		Coin:               "50 THOR.RUNE",
		LiquidityFeeInRune: 8,
		Slip:               5,
	})

	from := db.StrToSec("2020-09-03 00:00:00")
	to := db.StrToSec("2020-09-05 23:00:00")
	{
		// Check all pools
		body := testdb.CallJSON(t, fmt.Sprintf(
			"http://localhost:8080/v2/history/swaps?interval=day&from=%d&to=%d", from, to))

		var jsonResult oapigen.SwapHistoryResponse
		testdb.MustUnmarshal(t, body, &jsonResult)

		require.Equal(t, epochStr("2020-09-03 00:00:00"), jsonResult.Meta.StartTime)
		require.Equal(t, epochStr("2020-09-06 00:00:00"), jsonResult.Meta.EndTime)
		require.Equal(t, "28", jsonResult.Meta.ToRuneVolume)
		require.Equal(t, "65", jsonResult.Meta.ToAssetVolume)
		require.Equal(t, util.IntStr(28+65), jsonResult.Meta.TotalVolume)

		require.Equal(t, 3, len(jsonResult.Intervals))
		require.Equal(t, epochStr("2020-09-03 00:00:00"), jsonResult.Intervals[0].StartTime)
		require.Equal(t, epochStr("2020-09-04 00:00:00"), jsonResult.Intervals[0].EndTime)
		require.Equal(t, epochStr("2020-09-05 00:00:00"), jsonResult.Intervals[2].StartTime)

		require.Equal(t, "15", jsonResult.Intervals[0].ToAssetVolume)
		require.Equal(t, "8", jsonResult.Intervals[0].ToRuneVolume)
		require.Equal(t, "23", jsonResult.Intervals[0].TotalVolume)

		require.Equal(t, "0", jsonResult.Intervals[1].TotalVolume)

		require.Equal(t, "50", jsonResult.Intervals[2].ToAssetVolume)
		require.Equal(t, "20", jsonResult.Intervals[2].ToRuneVolume)

		// fees were 2,4 ; 5,8
		require.Equal(t, "4", jsonResult.Intervals[0].ToAssetFees)
		require.Equal(t, "2", jsonResult.Intervals[0].ToRuneFees)
		require.Equal(t, "6", jsonResult.Intervals[0].TotalFees)
		require.Equal(t, "19", jsonResult.Meta.TotalFees)

		require.Equal(t, "3", jsonResult.Intervals[0].ToAssetAverageSlip)
		require.Equal(t, "1", jsonResult.Intervals[0].ToRuneAverageSlip)
		require.Equal(t, "2", jsonResult.Intervals[0].AverageSlip)
		require.Equal(t, "2.5", jsonResult.Meta.AverageSlip)

	}

	{
		// Check only BNB.BNB pool
		body := testdb.CallJSON(t, fmt.Sprintf(
			"http://localhost:8080/v2/history/swaps?interval=day&from=%d&to=%d&pool=BNB.BNB", from, to))

		var jsonResult oapigen.SwapHistoryResponse
		testdb.MustUnmarshal(t, body, &jsonResult)

		require.Equal(t, 3, len(jsonResult.Intervals))
		require.Equal(t, "0", jsonResult.Intervals[0].TotalVolume)
		require.Equal(t, "50", jsonResult.Intervals[2].ToAssetVolume)
		require.Equal(t, "20", jsonResult.Intervals[2].ToRuneVolume)
	}

}

func TestSwapsCloseToBoundaryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00")

	// Swapping to rune 50 in the beginning of the year and 100 at the end of the year
	blocks.NewBlock(t, "2020-01-01 00:01:00", testdb.Swap{
		Pool:               "BNB.BTCB-1DE",
		EmitAsset:          "49 THOR.RUNE",
		Coin:               "0 BNB.BTCB-1DE",
		LiquidityFeeInRune: 1,
	})

	blocks.NewBlock(t, "2020-12-31 23:59:00", testdb.Swap{
		Pool:               "BNB.BTCB-1DE",
		EmitAsset:          "97 THOR.RUNE",
		Coin:               "0 BNB.BTCB-1DE",
		LiquidityFeeInRune: 3,
	})

	blocks.NewBlock(t, "2030-01-01 00:00:00")

	from := db.StrToSec("2019-01-01 00:00:00")
	to := db.StrToSec("2022-01-01 00:00:00")
	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/history/swaps?interval=year&from=%d&to=%d", from, to))

	var swapHistory oapigen.SwapHistoryResponse
	testdb.MustUnmarshal(t, body, &swapHistory)

	// We check if both first and last minute was attributed to the same year
	require.Equal(t, "150", swapHistory.Meta.ToRuneVolume)
	require.Equal(t, 3, len(swapHistory.Intervals))
	require.Equal(t, epochStr("2020-01-01 00:00:00"), swapHistory.Intervals[1].StartTime)
	require.Equal(t, "150", swapHistory.Intervals[1].ToRuneVolume)
}

func TestMinute5(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00")

	// Swapping 50 and 100 rune
	blocks.NewBlock(t, "2020-01-01 00:01:00", testdb.Swap{
		Pool:               "BNB.BTCB-1DE",
		EmitAsset:          "49 THOR.RUNE",
		Coin:               "0 BNB.BTCB-1DE",
		LiquidityFeeInRune: 1,
	})

	blocks.NewBlock(t, "2020-01-01 00:12:00", testdb.Swap{
		Pool:               "BNB.BTCB-1DE",
		EmitAsset:          "97 THOR.RUNE",
		Coin:               "0 BNB.BTCB-1DE",
		LiquidityFeeInRune: 3,
	})

	blocks.NewBlock(t, "2030-01-01 00:00:00")

	from := db.StrToSec("2020-01-01 00:00:00")
	to := db.StrToSec("2020-01-01 00:15:00")
	body := testdb.CallJSON(t, fmt.Sprintf("http://localhost:8080/v2/history/swaps?interval=5min&from=%d&to=%d", from, to))

	var swapHistory oapigen.SwapHistoryResponse
	testdb.MustUnmarshal(t, body, &swapHistory)

	require.Equal(t, "150", swapHistory.Meta.ToRuneVolume)
	require.Equal(t, 3, len(swapHistory.Intervals))
	require.Equal(t, epochStr("2020-01-01 00:00:00"), swapHistory.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-01-01 00:05:00"), swapHistory.Intervals[1].StartTime)
	require.Equal(t, epochStr("2020-01-01 00:10:00"), swapHistory.Intervals[2].StartTime)
	require.Equal(t, "50", swapHistory.Intervals[0].ToRuneVolume)
	require.Equal(t, "100", swapHistory.Intervals[2].ToRuneVolume)
}

func TestSwapUsdPrices(t *testing.T) {
	config.Global.UsdPools = []string{"USDA", "USDB"}

	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2019-12-25 12:00:00", testdb.AddLiquidity{
		Pool: "USDB", AssetAmount: 30, RuneAmount: 10,
	})

	blocks.NewBlock(t, "2020-01-01 13:00:00", testdb.Swap{
		Pool:               "BTC.BTC",
		EmitAsset:          "2 THOR.RUNE",
		Coin:               "0 BTC.BTC",
		LiquidityFeeInRune: 1,
	})

	blocks.NewBlock(t, "2020-01-02 12:00:00", testdb.AddLiquidity{
		Pool: "USDA", AssetAmount: 200, RuneAmount: 100,
	})

	blocks.NewBlock(t, "2020-01-03 13:00:00", testdb.Swap{
		Pool:               "BTC.BTC",
		EmitAsset:          "4 THOR.RUNE",
		Coin:               "0 BTC.BTC",
		LiquidityFeeInRune: 2,
	})

	blocks.NewBlock(t, "2030-01-01 00:00:00")

	from := db.StrToSec("2020-01-01 00:00:00")
	to := db.StrToSec("2020-01-06 00:00:00")
	body := testdb.CallJSON(t, fmt.Sprintf("http://localhost:8080/v2/history/swaps?interval=day&from=%d&to=%d", from, to))

	var swapHistory oapigen.SwapHistoryResponse
	testdb.MustUnmarshal(t, body, &swapHistory)

	require.Equal(t, 5, len(swapHistory.Intervals))
	require.Equal(t, epochStr("2020-01-01 00:00:00"), swapHistory.Intervals[0].StartTime)
	require.Equal(t, "3", swapHistory.Intervals[0].ToRuneVolume)
	require.Equal(t, "3", swapHistory.Intervals[0].RunePriceUSD) // 30 / 10
	require.Equal(t, epochStr("2020-01-02 00:00:00"), swapHistory.Intervals[1].StartTime)
	require.Equal(t, "2", swapHistory.Intervals[1].RunePriceUSD)
	require.Equal(t, epochStr("2020-01-03 00:00:00"), swapHistory.Intervals[2].StartTime)
	require.Equal(t, "2", swapHistory.Intervals[2].RunePriceUSD)
	require.Equal(t, "2", swapHistory.Meta.RunePriceUSD)
}

func TestAverageNaN(t *testing.T) {
	testdb.InitTest(t)

	// No swaps
	from := db.StrToSec("2020-01-01 00:00:00")
	to := db.StrToSec("2020-01-02 00:00:00")
	body := testdb.CallJSON(t, fmt.Sprintf("http://localhost:8080/v2/history/swaps?interval=day&from=%d&to=%d", from, to))

	var swapHistory oapigen.SwapHistoryResponse
	testdb.MustUnmarshal(t, body, &swapHistory)

	require.Equal(t, "0", swapHistory.Meta.AverageSlip)
}

// Parse string as date and return the unix epoch int value as string.
func epochStr(t string) string {
	return util.IntStr(db.StrToSec(t).ToI())
}

func TestVolume24h(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 12:00:00", testdb.AddLiquidity{
		Pool: "BNB.BNB", AssetAmount: 1000, RuneAmount: 2000,
	}, testdb.PoolActivate("BNB.BNB"))

	// swap 25h ago
	blocks.NewBlock(t, "2021-01-01 12:00:00", testdb.Swap{
		Pool:               "BNB.BNB",
		EmitAsset:          "8 THOR.RUNE",
		Coin:               "0 BNB.BNB",
		LiquidityFeeInRune: 2,
		Slip:               1,
	})

	// swap 22h ago
	blocks.NewBlock(t, "2021-01-01 15:00:00", testdb.Swap{
		Pool:               "BNB.BNB",
		EmitAsset:          "28 THOR.RUNE",
		Coin:               "0 BNB.BNB",
		LiquidityFeeInRune: 2,
		Slip:               1,
	}, testdb.Swap{
		Pool:               "BNB.BNB",
		EmitAsset:          "0 BNB.BNB",
		Coin:               "40 THOR.RUNE",
		LiquidityFeeInRune: 2,
		Slip:               1,
	})

	blocks.NewBlock(t, "2021-01-02 13:00:00")

	var pools oapigen.PoolsResponse
	testdb.MustUnmarshal(t, testdb.CallJSON(t,
		"http://localhost:8080/v2/pools"), &pools)
	require.Len(t, pools, 1)
	require.Equal(t, "BNB.BNB", pools[0].Asset)
	require.Equal(t, "70", pools[0].Volume24h)
}

func TestSwapsHistorySynths(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			RuneAddress: "thoraddr1",
			AssetAmount: 1000,
			RuneAmount:  10000,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2020-01-01 00:01:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "10 THOR.RUNE",
			EmitAsset:          "1 BTC.BTC",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "2 BTC.BTC",
			EmitAsset:          "20 THOR.RUNE",
			LiquidityFeeInRune: 2,
			Slip:               6,
		},
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "30 THOR.RUNE",
			EmitAsset:          "3 BTC/BTC",
			LiquidityFeeInRune: 3,
			Slip:               7,
		},
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "4 BTC/BTC",
			EmitAsset:          "40 THOR.RUNE",
			LiquidityFeeInRune: 4,
			Slip:               8,
		},
	)

	blocks.NewBlock(t, "2030-01-01 00:00:00")

	from := db.StrToSec("2020-01-01 00:00:00")
	to := db.StrToSec("2021-01-01 00:00:00")
	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/history/swaps?interval=year&from=%d&to=%d", from, to))

	var swapHistory oapigen.SwapHistoryResponse
	testdb.MustUnmarshal(t, body, &swapHistory)

	require.Equal(t, "4", swapHistory.Meta.TotalCount)

	require.Equal(t, "10", swapHistory.Meta.ToAssetVolume)
	require.Equal(t, "22", swapHistory.Meta.ToRuneVolume)
	require.Equal(t, "30", swapHistory.Meta.SynthMintVolume)
	require.Equal(t, "44", swapHistory.Meta.SynthRedeemVolume)
	require.Equal(t, "106", swapHistory.Meta.TotalVolume)

	require.Equal(t, "1", swapHistory.Meta.ToAssetFees)
	require.Equal(t, "2", swapHistory.Meta.ToRuneFees)
	require.Equal(t, "3", swapHistory.Meta.SynthMintFees)
	require.Equal(t, "4", swapHistory.Meta.SynthRedeemFees)
	require.Equal(t, "10", swapHistory.Meta.TotalFees)

	require.Equal(t, "5", swapHistory.Meta.ToAssetAverageSlip)
	require.Equal(t, "6", swapHistory.Meta.ToRuneAverageSlip)
	require.Equal(t, "7", swapHistory.Meta.SynthMintAverageSlip)
	require.Equal(t, "8", swapHistory.Meta.SynthRedeemAverageSlip)
	require.Equal(t, "6.5", swapHistory.Meta.AverageSlip)
}

func TestStatsSwapsDirection(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	testdb.ScenarioTenSwaps(t, blocks)

	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/stats"))

	var result oapigen.StatsResponse
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, "10", result.SwapCount)
	require.Equal(t, "4", result.ToAssetCount)
	require.Equal(t, "3", result.ToRuneCount)
	require.Equal(t, "2", result.SynthMintCount)
	require.Equal(t, "1", result.SynthBurnCount)
	require.Equal(t, "11203340", result.SwapVolume)
}

func TestPoolSwapVolume(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	testdb.ScenarioTenSwaps(t, blocks)

	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/pool/BTC.BTC"))

	var result oapigen.PoolDetail
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, "11203340", result.Volume24h)
}

func TestPoolsSwapVolume(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	testdb.ScenarioTenSwaps(t, blocks)

	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/pools"))

	var result oapigen.PoolDetails
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, 1, len(result))
	require.Equal(t, "11203340", result[0].Volume24h)
}

func TestStreamingSwap(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			RuneAddress: "thoraddr1",
			AssetAmount: 1000,
			RuneAmount:  10000,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2020-01-01 00:01:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "100 THOR.RUNE",
			EmitAsset:          "10 BTC.BTC",
			Memo:               "=:BTC.BTC:btc1addr:90000000/1/0",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
	)

	blocks.NewBlock(t, "2020-01-01 00:01:05",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "100 THOR.RUNE",
			EmitAsset:          "10 BTC.BTC",
			Memo:               "=:BTC.BTC:btc1addr:90000000/1/0",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
		testdb.Outbound{
			TxID:      "outTXID",
			Coin:      "19 BTC.BTC",
			ToAddress: "btc1addr",
		},
	)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0")
	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)
}

func TestL1StreamingSwap(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			RuneAddress: "thoraddr1",
			AssetAmount: 1000,
			RuneAmount:  10000,
		},
		testdb.AddLiquidity{
			Pool:        "ETH.ETH",
			RuneAddress: "thoraddr1",
			AssetAmount: 2000,
			RuneAmount:  10000,
		},
		testdb.PoolActivate("ETH.ETH"),
	)

	blocks.NewBlock(t, "2020-01-01 00:01:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "10 BTC.BTC",
			EmitAsset:          "100 THOR.RUNE",
			Memo:               "=:ETH.ETH:eth1addr:18/1/0",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
		testdb.Outbound{
			TxID:      "outTXID",
			Coin:      "100 THOR.RUNE",
			ToAddress: "eth1addr",
		},
		testdb.Swap{
			Pool:               "ETH.ETH",
			Coin:               "100 THOR.RUNE",
			EmitAsset:          "20 ETH.ETH",
			Memo:               "=:ETH.ETH:eth1addr:18/1/0",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
	)

	blocks.NewBlock(t, "2020-01-01 00:01:05",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "10 BTC.BTC",
			EmitAsset:          "100 THOR.RUNE",
			Memo:               "=:BTC.BTC:eth1addr:18/1/0",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
		testdb.Outbound{
			TxID:      "outTXID",
			Coin:      "100 THOR.RUNE",
			ToAddress: "eth1addr",
		},
		testdb.Swap{
			Pool:               "ETH.ETH",
			Coin:               "100 THOR.RUNE",
			EmitAsset:          "20 ETH.ETH",
			Memo:               "=:ETH.ETH:eth1addr:18/1/0",
			LiquidityFeeInRune: 1,
			Slip:               5,
		},
	)

	blocks.NewBlock(t, "2020-01-01 00:01:10",
		testdb.Outbound{
			TxID:      "outTXID",
			Coin:      "39 ETH.ETH",
			ToAddress: "eth1addr",
		},
	)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0")
	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)
}
