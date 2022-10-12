package stat_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestWithdrawsLookupE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	// btc price: 100
	// bnb price: 10
	blocks.NewBlock(t, "2021-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 1e10, RuneAmount: 1e12,
			RuneAddress: "R1"},
		testdb.PoolActivate("BTC.BTC"),
		testdb.AddLiquidity{Pool: "BNB.BNB", AssetAmount: 1e10, RuneAmount: 1e11,
			RuneAddress: "R1"},
		testdb.PoolActivate("BNB.BNB"))

	blocks.NewBlock(t, "2021-01-01 00:00:05",
		testdb.Withdraw{Pool: "BTC.BTC", EmitAsset: 2, EmitRune: 2, FromAddress: "R1"},
		testdb.Withdraw{Pool: "BNB.BNB", EmitAsset: 3, EmitRune: 3, FromAddress: "R1"})

	body := testdb.CallJSON(t, "http://localhost:8080/v2/stats")
	var jsonResult oapigen.StatsData
	testdb.MustUnmarshal(t, body, &jsonResult)

	assert.Equal(t, "2", jsonResult.WithdrawCount)
	assert.Equal(t, "235", jsonResult.WithdrawVolume)
}

func TestWithdrawsEmpty(t *testing.T) {
	testdb.InitTest(t)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/stats")
	var jsonResult oapigen.StatsData
	testdb.MustUnmarshal(t, body, &jsonResult)

	assert.Equal(t, "0", jsonResult.WithdrawCount)
	assert.Equal(t, "0", jsonResult.WithdrawVolume)
}

func TestWithdrawAllAssets(t *testing.T) {
	testdb.InitTest(t)

	testdb.InsertWithdrawEvent(t, testdb.FakeWithdraw{
		Pool: "BNB.BNB", EmitAssetE8: 10, EmitRuneE8: 0, BlockTimestamp: "2021-01-12 12:30:00",
	})
	testdb.InsertBlockPoolDepth(t, "BNB.BNB", 0, 20, "2021-01-12 12:30:00")

	body := testdb.CallJSON(t, "http://localhost:8080/v2/stats")
	var jsonResult oapigen.StatsData
	testdb.MustUnmarshal(t, body, &jsonResult)

	assert.Equal(t, "1", jsonResult.WithdrawCount)
	assert.Equal(t, "0", jsonResult.WithdrawVolume)
}

func TestDepositsLookupE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	// btc price: 100
	// bnb price: 10
	blocks.NewBlock(t, "2021-01-01 00:00:00",
		testdb.AddLiquidity{Pool: "BTC.BTC", AssetAmount: 1, RuneAmount: 100,
			RuneAddress: "R1"},
		testdb.PoolActivate("BTC.BTC"),
		testdb.AddLiquidity{Pool: "BNB.BNB", AssetAmount: 1, RuneAmount: 10,
			RuneAddress: "R1"},
		testdb.PoolActivate("BNB.BNB"))

	body := testdb.CallJSON(t, "http://localhost:8080/v2/stats")
	var jsonResult oapigen.StatsData
	testdb.MustUnmarshal(t, body, &jsonResult)

	assert.Equal(t, "2", jsonResult.AddLiquidityCount)
	assert.Equal(t, "220", jsonResult.AddLiquidityVolume)
}
