package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestLiquidityHistoryE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			AssetAmount: 10000000,
			RuneAmount:  20000000,
		},
		testdb.PoolActivate("BTC.BTC"),
		testdb.AddLiquidity{
			Pool:        "BNB.BNB",
			AssetAmount: 10000000,
			RuneAmount:  30000000,
		},
		testdb.PoolActivate("BNB.BNB"),
	)

	// 3rd of September
	blocks.NewBlock(t, "2020-09-03 12:30:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 1, RuneAmount: 2},
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 3, RuneAmount: 4},
		testdb.Withdraw{Pool: "BTC.BTC", EmitAsset: 5, EmitRune: 6,
			LiquidityProviderUnits: 1},
	)

	// 5th of September
	blocks.NewBlock(t, "2020-09-05 12:30:00",
		testdb.AddLiquidity{Pool: "BNB.BNB", AssetAmount: 7, RuneAmount: 8},
		testdb.Withdraw{Pool: "BNB.BNB", EmitAsset: 9, EmitRune: 10,
			LiquidityProviderUnits: 1},
		testdb.Withdraw{Pool: "BNB.BNB", EmitAsset: 11, EmitRune: 12,
			LiquidityProviderUnits: 1},
	)

	from := db.StrToSec("2020-09-03 00:00:00").ToI()
	to := db.StrToSec("2020-09-06 00:00:00").ToI()

	expectedBTCDeposits := int64(1*2 + 2 + 3*2 + 4)
	expectedBNBDeposits := int64(7*3 + 8)
	expectedBTCWithdrawals := int64(5*2 + 6)
	expectedBNBWithdrawals := int64(9*3 + 10 + 11*3 + 12)
	// Check all pools
	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/liquidity_changes?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.LiquidityHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, epochStr("2020-09-03 00:00:00"), jsonResult.Meta.StartTime)
	require.Equal(t, util.IntStr(to), jsonResult.Meta.EndTime)
	require.Equal(t, util.IntStr(expectedBTCDeposits+expectedBNBDeposits), jsonResult.Meta.AddLiquidityVolume)
	require.Equal(t, util.IntStr(expectedBTCWithdrawals+expectedBNBWithdrawals), jsonResult.Meta.WithdrawVolume)
	require.Equal(t, "3", jsonResult.Meta.AddLiquidityCount)
	require.Equal(t, "3", jsonResult.Meta.WithdrawCount)

	require.Equal(t, 3, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-09-03 00:00:00"), jsonResult.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-09-04 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, epochStr("2020-09-05 00:00:00"), jsonResult.Intervals[2].StartTime)
	require.Equal(t, util.IntStr(to), jsonResult.Intervals[2].EndTime)

	require.Equal(t, util.IntStr(expectedBTCDeposits), jsonResult.Intervals[0].AddLiquidityVolume)
	require.Equal(t, util.IntStr(expectedBTCWithdrawals), jsonResult.Intervals[0].WithdrawVolume)
	require.Equal(t, "2", jsonResult.Intervals[0].AddLiquidityCount)
	require.Equal(t, "1", jsonResult.Intervals[0].WithdrawCount)

	require.Equal(t, "0", jsonResult.Intervals[1].AddLiquidityVolume)
	require.Equal(t, "0", jsonResult.Intervals[1].WithdrawVolume)

	require.Equal(t, util.IntStr(expectedBNBDeposits), jsonResult.Intervals[2].AddLiquidityVolume)
	require.Equal(t, util.IntStr(expectedBNBWithdrawals), jsonResult.Intervals[2].WithdrawVolume)

	// Check single pool
	body = testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/liquidity_changes?interval=day&from=%d&to=%d&pool=BNB.BNB", from, to))

	testdb.MustUnmarshal(t, body, &jsonResult)
	require.Equal(t, util.IntStr(expectedBNBDeposits), jsonResult.Meta.AddLiquidityVolume)
	require.Equal(t, util.IntStr(expectedBNBWithdrawals), jsonResult.Meta.WithdrawVolume)
}

func TestLiquidityAddOnePoolOnly(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 100 - 1, RuneAmount: 200 - 2},
		testdb.PoolActivate("BTC.BTC"),
		testdb.AddLiquidity{Pool: "BNB.BNB", AssetAmount: 100, RuneAmount: 300},
		testdb.PoolActivate("BNB.BNB"),
	)

	blocks.NewBlock(t, "2020-01-01 12:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 1, RuneAmount: 2},
	)

	// Having a 2 assetPrice is important for the assertions below.
	depths := timeseries.Latest.GetState().Pools["BTC.BTC"]
	require.Equal(t, int64(100), depths.AssetDepth)
	require.Equal(t, int64(200), depths.RuneDepth)

	from := db.StrToSec("2020-01-01 00:00:00").ToI()
	to := db.StrToSec("2020-01-02 00:00:00").ToI()

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/liquidity_changes?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.LiquidityHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "4", jsonResult.Meta.AddLiquidityVolume)
	require.Equal(t, "1", jsonResult.Meta.AddLiquidityCount)
}

func TestLiquidityAssymetric(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 100 - 10 + 1, RuneAmount: 200 - 2 + 1},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2020-01-01 12:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 10, RuneAmount: 2},
		testdb.Withdraw{Pool: "BTC.BTC", EmitAsset: 1, EmitRune: 1,
			LiquidityProviderUnits: 1},
	)

	// Having a 2 assetPrice is important for the assertions below.
	depths := timeseries.Latest.GetState().Pools["BTC.BTC"]
	require.Equal(t, int64(100), depths.AssetDepth)
	require.Equal(t, int64(200), depths.RuneDepth)

	from := db.StrToSec("2020-01-01 00:00:00").ToI()
	to := db.StrToSec("2020-01-02 00:00:00").ToI()

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/liquidity_changes?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.LiquidityHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, "20", jsonResult.Meta.AddAssetLiquidityVolume)
	require.Equal(t, "2", jsonResult.Meta.AddRuneLiquidityVolume)
	require.Equal(t, "22", jsonResult.Meta.AddLiquidityVolume)
	require.Equal(t, "1", jsonResult.Meta.AddLiquidityCount)

	require.Equal(t, "2", jsonResult.Meta.WithdrawAssetVolume)
	require.Equal(t, "1", jsonResult.Meta.WithdrawRuneVolume)
	require.Equal(t, "3", jsonResult.Meta.WithdrawVolume)
	require.Equal(t, "1", jsonResult.Meta.WithdrawCount)
}

func TestImpermanentLoss(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			AssetAmount: 100 - 10 + 5,
			RuneAmount:  200 - 2 + 100 - 42,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2020-01-01 12:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 10, RuneAmount: 2},
		testdb.Withdraw{Pool: "BTC.BTC",
			EmitAsset: 5, EmitRune: 100, ImpLossProtection: 42,
			LiquidityProviderUnits: 1,
		},
	)

	// Having a 2 assetPrice is important for the assertions below.
	depths := timeseries.Latest.GetState().Pools["BTC.BTC"]
	require.Equal(t, int64(100), depths.AssetDepth)
	require.Equal(t, int64(200), depths.RuneDepth)

	from := db.StrToSec("2020-01-01 00:00:00").ToI()
	to := db.StrToSec("2020-01-02 00:00:00").ToI()

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/liquidity_changes?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.LiquidityHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Len(t, jsonResult.Intervals, 1)
	require.Equal(t, "42", jsonResult.Intervals[0].ImpermanentLossProtectionPaid)

	require.Equal(t, "42", jsonResult.Meta.ImpermanentLossProtectionPaid)
}

// This testcase tests an edge case where saver with affiliate is happening and midgard get its
// affiliate from swap event that is created afterward.
func TestSaverAddAffiliate(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			AssetAmount: 100,
			RuneAmount:  200,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2020-01-01 01:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC/BTC",
			AssetAmount: 10,
			RuneAmount:  0,
		},
		testdb.Swap{
			TxID:               "7F95AE00E74118E5EE87A90FFC0EE4FB3D9236F6246BE7235DCA1090D0B2F83C",
			Pool:               "BTC.BTC",
			Coin:               "10 BTC.BTC",
			EmitAsset:          "20 THOR.RUNE",
			LiquidityFeeInRune: 1,
			Slip:               0,
			Memo:               "+:BTC/BTC::thor1xmaggkcln5m5fnha2780xrdrulmplvfrz6wj3l:30",
		},
		testdb.Outbound{
			Chain:  "THOR",
			Coin:   "20 THOR.RUNE",
			InTxID: "7F95AE00E74118E5EE87A90FFC0EE4FB3D9236F6246BE7235DCA1090D0B2F83C",
			Memo:   "+:BTC/BTC::thor1xmaggkcln5m5fnha2780xrdrulmplvfrz6wj3l:30",
		},
		testdb.Swap{
			TxID:               "7F95AE00E74118E5EE87A90FFC0EE4FB3D9236F6246BE7235DCA1090D0B2F83C",
			Pool:               "BTC.BTC",
			Coin:               "20 THOR.RUNE",
			EmitAsset:          "10 BTC/BTC",
			LiquidityFeeInRune: 1,
			Slip:               0,
			Memo:               "+:BTC/BTC::thor1xmaggkcln5m5fnha2780xrdrulmplvfrz6wj3l:30",
		},
	)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?txid=7F95AE00E74118E5EE87A90FFC0EE4FB3D9236F6246BE7235DCA1090D0B2F83C")

	var jsonResult oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, *jsonResult.Count, "1")
	require.Equal(t, jsonResult.Actions[0].Metadata.Swap.AffiliateAddress, "thor1xmaggkcln5m5fnha2780xrdrulmplvfrz6wj3l")
	require.Equal(t, jsonResult.Actions[0].Metadata.Swap.AffiliateFee, "30")
}
