package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestDepthHistoryE2E(t *testing.T) {
	testdb.InitTest(t)
	testdb.DeclarePools("BNB.BNB")

	// This will be skipped because we query 01-09 to 01-13
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 1000, 1, "2020-01-13 12:00:00")

	// This will be the initial value
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 30, 3, "2020-01-05 12:00:00")

	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 10, 20, "2020-01-10 12:00:05")
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 20, 30, "2020-01-10 14:00:00")
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 2, 5, "2020-01-12 09:00:00")
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 6, 18, "2020-01-12 10:00:00")
	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BNB.BNB",
		StakeUnits:     1,
		BlockTimestamp: "2020-01-01 23:57:00",
		AssetAddress:   "bnbxxx",
	})
	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-13 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BNB.BNB?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, oapigen.DepthHistoryMeta{
		StartTime:        epochStr("2020-01-09 00:00:00"),
		EndTime:          epochStr("2020-01-13 00:00:00"),
		PriceShiftLoss:   "0.35336939193881683",
		LuviIncrease:     "1.0954451150103321",
		StartAssetDepth:  "30",
		StartLPUnits:     "1",
		StartSynthUnits:  "0",
		StartMemberCount: "1",
		StartRuneDepth:   "3",
		EndAssetDepth:    "6",
		EndLPUnits:       "1",
		EndSynthUnits:    "0",
		EndRuneDepth:     "18",
		EndMemberCount:   "1",
	}, jsonResult.Meta)
	require.Equal(t, 4, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-01-10 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, epochStr("2020-01-13 00:00:00"), jsonResult.Intervals[3].EndTime)

	// initial value correct
	jan9 := jsonResult.Intervals[0]
	require.Equal(t, "3", jan9.RuneDepth)

	jan10 := jsonResult.Intervals[1]
	require.Equal(t, "30", jan10.RuneDepth)
	require.Equal(t, "20", jan10.AssetDepth)
	require.Equal(t, "1.5", jan10.AssetPrice)

	// gapfill works.
	jan11 := jsonResult.Intervals[2]
	require.Equal(t, "1.5", jan11.AssetPrice)
}

func TestUSDHistoryE2E(t *testing.T) {
	testdb.InitTest(t)
	testdb.DeclarePools("BNB.BNB", "USDA", "USDB")

	config.Global.UsdPools = []string{"USDA", "USDB"}

	// assetPrice: 2, runePriceUSD: 2
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 1, 2, "2020-01-05 12:00:00")
	testdb.InsertBlockPoolDepth(t, "USDA", 200, 100, "2020-01-05 12:00:00")
	testdb.InsertBlockPoolDepth(t, "USDB", 30, 10, "2020-01-05 12:00:00")

	// runePriceUSD 3
	testdb.InsertBlockPoolDepth(t, "USDB", 3000, 1000, "2020-01-10 12:00:05")

	// runePriceUSD 2, back to USDA
	testdb.InsertBlockPoolDepth(t, "USDB", 10, 10, "2020-01-11 12:00:05")

	// assetPrice: 10
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 1, 10, "2020-01-13 12:00:00")
	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-14 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BNB.BNB?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, 5, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)

	require.Equal(t, "2", jsonResult.Intervals[0].AssetPrice)

	require.Equal(t, "4", jsonResult.Intervals[0].AssetPriceUSD)
	require.Equal(t, "6", jsonResult.Intervals[1].AssetPriceUSD)
	require.Equal(t, "4", jsonResult.Intervals[2].AssetPriceUSD)
	require.Equal(t, "4", jsonResult.Intervals[3].AssetPriceUSD)
	require.Equal(t, "20", jsonResult.Intervals[4].AssetPriceUSD)
}

func TestLiquidityUnitsHistoryE2E(t *testing.T) {
	testdb.InitTest(t)
	testdb.DeclarePools("BTC.BTC", "BNB.BNB")

	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BTC.BTC",
		StakeUnits:     10,
		BlockTimestamp: "2020-01-10 12:00:00",
	})

	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BTC.BTC",
		StakeUnits:     10, // total 20
		BlockTimestamp: "2020-01-20 12:00:00",
	})

	testdb.InsertWithdrawEvent(t, testdb.FakeWithdraw{
		Pool:           "BTC.BTC",
		StakeUnits:     5, // total 15
		BlockTimestamp: "2020-01-21 12:00:00",
	})

	// This will be skipped because it's a different pool
	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BNB.BNB",
		StakeUnits:     1000,
		BlockTimestamp: "2020-01-20 12:00:00",
	})

	from := db.StrToSec("2020-01-19 00:00:00")
	to := db.StrToSec("2020-01-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, 3, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-20 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, "10", jsonResult.Intervals[0].LiquidityUnits)

	require.Equal(t, epochStr("2020-01-21 00:00:00"), jsonResult.Intervals[1].EndTime)
	require.Equal(t, "20", jsonResult.Intervals[1].LiquidityUnits)

	require.Equal(t, epochStr("2020-01-22 00:00:00"), jsonResult.Intervals[2].EndTime)
	require.Equal(t, "15", jsonResult.Intervals[2].LiquidityUnits)
}

func TestMembersHistoryE2E(t *testing.T) {
	testdb.InitTest(t)
	testdb.DeclarePools("BTC.BTC", "BNB.BNB")

	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BTC.BTC",
		StakeUnits:     10,
		BlockTimestamp: "2020-01-10 12:00:00",
		AssetAddress:   "btc1",
	})

	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BTC.BTC",
		StakeUnits:     10,
		BlockTimestamp: "2020-01-20 12:00:00",
		AssetAddress:   "btc2",
		RuneAddress:    "thor1",
	})

	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BTC.BTC",
		StakeUnits:     10, // Total of 20
		BlockTimestamp: "2020-01-20 12:00:00",
		AssetAddress:   "btc1",
	})

	testdb.InsertWithdrawEvent(t, testdb.FakeWithdraw{
		Pool:           "BTC.BTC",
		StakeUnits:     20, // total 0
		BlockTimestamp: "2020-01-21 12:00:00",
		FromAddr:       "btc1",
	})

	testdb.InsertWithdrawEvent(t, testdb.FakeWithdraw{
		Pool:           "BTC.BTC",
		StakeUnits:     5, // total 5
		BlockTimestamp: "2020-01-21 12:00:00",
		FromAddr:       "thor1",
	})

	// This will be skipped because it's a different pool
	testdb.InsertStakeEvent(t, testdb.FakeStake{
		Pool:           "BNB.BNB",
		StakeUnits:     1000,
		BlockTimestamp: "2020-01-20 12:00:00",
		RuneAddress:    "thor1",
	})

	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-19 00:00:00")
	to := db.StrToSec("2020-01-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, 3, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-20 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, "1", jsonResult.Intervals[0].MembersCount)

	require.Equal(t, epochStr("2020-01-21 00:00:00"), jsonResult.Intervals[1].EndTime)
	require.Equal(t, "2", jsonResult.Intervals[1].MembersCount)

	require.Equal(t, epochStr("2020-01-22 00:00:00"), jsonResult.Intervals[2].EndTime)
	require.Equal(t, "1", jsonResult.Intervals[2].MembersCount)
}

func TestDepthAggregateE2E(t *testing.T) {
	testdb.InitTest(t)
	testdb.DeclarePools("A.A", "B.B")

	// This is to test that "sewing" together data from the continuous aggregate and
	// the live table works correctly.
	//
	// We have two relevant time buckets for this test:
	// - one ending on 2020-01-02 00:00:00 - data here should be coming from the continuous aggregate
	// - one starting on 2020-01-02 00:00:00 - data here should be coming only from the live table

	testdb.InsertBlockPoolDepth(t, "A.A", 1, 30, "2020-01-01 23:57:00")
	testdb.InsertBlockPoolDepth(t, "A.A", 1, 10, "2020-01-02 00:02:00")
	testdb.InsertBlockPoolDepth(t, "A.A", 1, 20, "2020-01-02 00:03:00")

	testdb.InsertBlockPoolDepth(t, "B.B", 1, 10, "2020-01-01 23:57:00")
	testdb.InsertBlockPoolDepth(t, "B.B", 1, 20, "2020-01-02 00:03:00")

	db.RefreshAggregatesForTests()

	to := db.StrToSec("2020-01-02 00:02:30")
	var jsonResult oapigen.DepthHistoryResponse

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/A.A?&to=%d", to))
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "10", jsonResult.Intervals[0].RuneDepth)

	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/B.B?&to=%d", to))
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "10", jsonResult.Intervals[0].RuneDepth)
}

func TestLiqUnitValueIndexWithInterval(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 23:57:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100,
			RuneAmount:             1000,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2010-02-01 23:57:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "550 THOR.RUNE",
			EmitAsset:          "50 BTC.BTC",
			LiquidityFeeInRune: 10,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 50 btc, 1550 rune

	blocks.NewBlock(t, "2010-02-01 23:57:01",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "170 BTC.BTC",
			EmitAsset:          "1000 THOR.RUNE",
			LiquidityFeeInRune: 1,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 220 btc, 550 rune

	blocks.NewBlock(t, "2010-02-22 00:00:01")

	from := db.StrToSec("2010-01-01 00:00:00")
	to := db.StrToSec("2010-02-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "220", jsonResult.Intervals[51].AssetDepth)
	require.Equal(t, "550", jsonResult.Intervals[51].RuneDepth)

	//sqrt(100 * 1000) / 10, for both intervals, as we did not increase / decrease liquidity
	testdb.RoughlyEqual(t, 31.622776, jsonResult.Intervals[0].Luvi)
	testdb.RoughlyEqual(t, 31.622776, jsonResult.Intervals[1].Luvi)
	//sqrt(220 * 550) / 10
	testdb.RoughlyEqual(t, 34.78505, jsonResult.Intervals[51].Luvi)

	//edge case, since we added the block pool depth, the depth will be 0, so the priceshift loss is not present at all
	require.Equal(t, "NaN", jsonResult.Meta.PriceShiftLoss)

	from = db.StrToSec("2010-01-02 00:00:00")
	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?interval=day&from=%d&to=%d", from, to))

	testdb.MustUnmarshal(t, body, &jsonResult)
	//this should be 2*sqrt(0.5)/1.5
	testdb.RoughlyEqual(t, 0.8, jsonResult.Meta.PriceShiftLoss)
	testdb.RoughlyEqual(t, 1.1, jsonResult.Meta.LuviIncrease) //minimal luvi decrease
}

func TestLiqUnitValueIndexWithoutInterval(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 23:57:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr1",
			AssetAmount:            100,
			RuneAmount:             1000,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2010-02-01 23:57:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "550 THOR.RUNE",
			EmitAsset:          "50 BTC.BTC",
			LiquidityFeeInRune: 10,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 50 btc, 1550 rune

	blocks.NewBlock(t, "2010-02-01 23:57:01",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "170 BTC.BTC",
			EmitAsset:          "1000 THOR.RUNE",
			LiquidityFeeInRune: 1,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 220 btc, 550 rune

	blocks.NewBlock(t, "2010-02-22 00:00:01")

	from := db.StrToSec("2010-01-01 00:00:00")
	to := db.StrToSec("2010-02-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	//sqrt(100 * 1000), for both intervals, as we did not increase / decrease liquidity
	require.Equal(t, 1, len(jsonResult.Intervals))
	testdb.RoughlyEqual(t, 34.78505426185217, jsonResult.Intervals[0].Luvi)

	//edge case, since we added the block pool depth, the depth will be 0, so the priceshift loss is not present at all
	require.Equal(t, "NaN", jsonResult.Meta.PriceShiftLoss)

	from = db.StrToSec("2010-01-02 00:00:00")
	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/BTC.BTC?from=%d&to=%d", from, to))

	testdb.MustUnmarshal(t, body, &jsonResult)
	//this should be 2*sqrt(0.5)/1.5
	testdb.RoughlyEqual(t, 0.8, jsonResult.Meta.PriceShiftLoss)
	testdb.RoughlyEqual(t, 1.1, jsonResult.Meta.LuviIncrease) //minimal luvi decrease
}

func TestLiqUnitValueIndexSynths(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-01 23:57:00",
		testdb.AddLiquidity{Pool: "ETH.ETH", AssetAmount: 100 * 100000000, RuneAmount: 1000 * 100000000, LiquidityProviderUnits: 1},
		testdb.PoolActivate("ETH.ETH"),
	)

	blocks.NewBlock(t, "2020-02-21 23:57:00", testdb.Swap{
		Pool:      "ETH.ETH",
		Coin:      "100000000 THOR.RUNE",
		EmitAsset: "42 ETH/ETH",
	})

	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-01 00:00:00")
	to := db.StrToSec("2020-02-22 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/ETH.ETH?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.DepthHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	//sqrt(100*100000000 * 1000*100000000), for both intervals, as we did not increase / decrease liquidity
	require.Equal(t, "31622776601.683792", jsonResult.Intervals[0].Luvi)
	require.Equal(t, "31622776601.683792", jsonResult.Intervals[1].Luvi)
	//sqrt(100*100000000 * 1001*100000000), we have 1 rune in synth, needs to be included
	require.Equal(t, "31638584039.11275", jsonResult.Intervals[51].Luvi)

	from = db.StrToSec("2020-01-02 00:00:00")
	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/depths/ETH.ETH?interval=day&from=%d&to=%d", from, to))

	testdb.MustUnmarshal(t, body, &jsonResult)
	//sqrt(100*100000000 * 1001*100000000) / sqrt(100*100000000 * 1000*100000000)
	require.Equal(t, "1.000499875062461", jsonResult.Meta.LuviIncrease) //minimal luvi decrease

	body = testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/pool/ETH.ETH"))

	var poolResult oapigen.PoolDetail
	testdb.MustUnmarshal(t, body, &poolResult)

	testdb.RoughlyEqual(t, 0.000499875062461*365/30, poolResult.AnnualPercentageRate)
	testdb.RoughlyEqual(t, 0.000499875062461*365/30, poolResult.PoolAPY)

}
