// End to end tests here are checkning lookup funcionality from Database to HTTP Api.

package timeseries_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"

	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func callPools(t *testing.T, url string) map[string]oapigen.PoolDetail {
	body := testdb.CallJSON(t, url)

	var response oapigen.PoolsResponse
	testdb.MustUnmarshal(t, body, &response)
	sortedResp := map[string]oapigen.PoolDetail{}

	for _, poolDetail := range response {
		sortedResp[poolDetail.Asset] = poolDetail
	}
	return sortedResp
}

func TestPoolsE2E(t *testing.T) {
	testdb.InitTest(t)

	testdb.InsertStakeEvent(t, testdb.FakeStake{Pool: "BNB.BNB", BlockTimestamp: "2020-01-01 00:00:00"})
	testdb.InsertStakeEvent(t, testdb.FakeStake{Pool: "POOL2"})
	testdb.InsertStakeEvent(t, testdb.FakeStake{Pool: "POOL3"})

	testdb.InsertPoolEvents(t, "BNB.BNB", "Available")
	testdb.InsertPoolEvents(t, "POOL2", "Available")
	testdb.InsertPoolEvents(t, "POOL3", "Staged")

	depths := []timeseries.Depth{
		{"BNB.BNB", 2, 1, 0},
		{"POOL2", 2, 1, 0},
		{"POOL3", 2, 1, 0},
	}
	timeseries.SetDepthsForTest(depths)

	sortedResp := callPools(t, "http://localhost:8080/v2/pools")

	require.Equal(t, len(sortedResp), 3)
	require.Equal(t, sortedResp["POOL2"].AssetDepth, "2")
	require.Equal(t, sortedResp["POOL2"].RuneDepth, "1")
	require.Equal(t, sortedResp["POOL2"].AssetPrice, "0.5")
	_, has_pool3 := sortedResp["POOL3"]
	require.Equal(t, has_pool3, true) // Without filter we have the Staged pool

	// check filtering
	sortedResp = callPools(t, "http://localhost:8080/v2/pools?status=available")
	require.Equal(t, len(sortedResp), 2)
	_, has_pool3 = sortedResp["POOL3"]
	require.Equal(t, has_pool3, false)

	// Check bad requests fail.
	testdb.JSONFailGeneral(t, "http://localhost:8080/v2/pools?status=badname")
}

func TestKnownPoolsE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			RuneAddress: "thoraddr1",
			AssetAmount: 100,
			RuneAmount:  1000,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2010-01-01 00:01:00",
		testdb.AddLiquidity{
			Pool:        "ETH.ETH",
			RuneAddress: "thoraddr1",
			AssetAmount: 100,
			RuneAmount:  1000,
		},
		testdb.PoolStatus{
			Pool:   "ETH.ETH",
			Status: testdb.StatusStaged,
		},
	)

	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/knownpools"))

	var result map[string]string
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, len(result), 2)
	require.Equal(t, result["BTC.BTC"], strings.ToLower(string(testdb.StatusAvailable)))
	require.Equal(t, result["ETH.ETH"], strings.ToLower(string(testdb.StatusStaged)))
}

func TestDecimalPool(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			RuneAddress: "thoraddr1",
			AssetAmount: 100,
			RuneAmount:  1000,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2010-01-01 00:01:00",
		testdb.AddLiquidity{
			Pool:        "ETH.ETH",
			RuneAddress: "thoraddr1",
			AssetAmount: 100,
			RuneAmount:  1000,
		},
		testdb.PoolStatus{
			Pool:   "ETH.ETH",
			Status: testdb.StatusStaged,
		},
	)

	blocks.NewBlock(t, "2009-01-01 00:02:00",
		testdb.AddLiquidity{
			Pool:        "BTC.USDT",
			RuneAddress: "thoraddr0",
			AssetAmount: 9,
			RuneAmount:  999,
		},
		testdb.PoolActivate("BTC.USDT"),
	)

	sortedResp := callPools(t, "http://localhost:8079/v2/pools")

	require.Equal(t, len(sortedResp), 3)
	// Test known asset pools from decimal.json
	require.Equal(t, sortedResp["BTC.BTC"].NativeDecimal, "8")
	require.Equal(t, sortedResp["ETH.ETH"].NativeDecimal, "18")
	// Test missing asset pool from decimal.json should return -1
	require.Equal(t, sortedResp["BTC.USDT"].NativeDecimal, "-1")

	// Test specific pool endpoint
	body := testdb.CallJSON(t, "http://localhost:8080/v2/pool/BTC.BTC")

	var result oapigen.PoolDetail
	testdb.MustUnmarshal(t, body, &result)
	require.Equal(t, result.NativeDecimal, "8")
}

func TestGenesisNodeGoesOut(t *testing.T) {
	testdb.InitTest(t)
	testdb.InsertUpdateNodeAccountStatusEvent(t,
		testdb.FakeNodeStatus{NodeAddr: "node1", Former: "Standby", Current: "Active"},
		"2020-09-02 12:00:00")
	testdb.InsertUpdateNodeAccountStatusEvent(t,
		testdb.FakeNodeStatus{NodeAddr: "node2", Former: "Standby", Current: "Active"},
		"2020-09-02 12:00:00")
	testdb.InsertUpdateNodeAccountStatusEvent(t,
		testdb.FakeNodeStatus{NodeAddr: "genesisNode", Former: "Active", Current: "Standby"},
		"2020-09-03 12:00:00")

	n, err := timeseries.ActiveNodeCount(context.Background(),
		db.StrToSec("2020-09-10 12:00:00").ToNano())
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
}

func TestAnnualPercentageRate(t *testing.T) {
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

	blocks.NewBlock(t, "2010-01-29 23:57:00",
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

	blocks.NewBlock(t, "2010-01-30 23:57:00",
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

	blocks.NewBlock(t, "2010-02-03 23:57:00")

	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/pool/BTC.BTC?period=30d"))

	var result oapigen.PoolDetail
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, "220", result.AssetDepth)
	require.Equal(t, "550", result.RuneDepth)
	testdb.RoughlyEqual(t, 0.1*365/30, result.AnnualPercentageRate)
	testdb.RoughlyEqual(t, 0.1*365/30, result.PoolAPY)
}

func TestNegativeAPR(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2010-01-01 23:57:00",
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			RuneAddress:            "thoraddr1",
			AssetAmount:            220,
			RuneAmount:             550,
			LiquidityProviderUnits: 10,
		},
		testdb.PoolActivate("BTC.BTC"),
	)

	blocks.NewBlock(t, "2010-01-30 23:57:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "1000 THOR.RUNE",
			EmitAsset:          "170 BTC.BTC",
			LiquidityFeeInRune: 1,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 50 btc, 1550 rune

	blocks.NewBlock(t, "2010-01-29 23:57:00",
		testdb.Swap{
			Pool:               "BTC.BTC",
			Coin:               "50 BTC.BTC",
			EmitAsset:          "550 THOR.RUNE",
			LiquidityFeeInRune: 10,
			LiquidityFee:       1,
			Slip:               42,
		},
	)
	// Pool balance after: 100 btc, 1000 rune

	blocks.NewBlock(t, "2010-02-03 23:57:00")

	body := testdb.CallJSON(t,
		fmt.Sprintf("http://localhost:8080/v2/pool/BTC.BTC?period=30d"))

	var result oapigen.PoolDetail
	testdb.MustUnmarshal(t, body, &result)

	require.Equal(t, "100", result.AssetDepth)
	require.Equal(t, "1000", result.RuneDepth)
	testdb.RoughlyEqual(t, -0.09090909090*365/30, result.AnnualPercentageRate)
	testdb.RoughlyEqual(t, 0, result.PoolAPY)
}
