// End to end tests here are checking lookup functionality from Database to HTTP Api.
package timeseries_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestActionsE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BNB.TWT-123",
			RuneAddress: "thoraddr1",
			AssetAmount: 1000,
			RuneAmount:  2000,
		},
		testdb.PoolActivate{Pool: "BNB.TWT-123"},
	)

	blocks.NewBlock(t, "2020-09-02 00:00:00",
		testdb.Withdraw{
			Pool:      "BNB.TWT-123",
			EmitAsset: 10,
			EmitRune:  20,
			Coin:      "10 BNB.TWT-123",
			ToAddress: "thoraddr4",
		},
	)

	blocks.NewBlock(t, "2020-09-03 00:00:00",
		testdb.Swap{
			Coin:      "100000 BNB.BNB",
			EmitAsset: "10 THOR.RUNE",
			Pool:      "BNB.BNB",
		},
		testdb.PoolActivate{Pool: "BNB.BNB"},
	)

	// Basic request with no filters (should get all events ordered by height)
	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0")

	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)

	require.Equal(t, "3", v.Count)

	basicTx0 := v.Actions[0]
	basicTx1 := v.Actions[1]
	basicTx2 := v.Actions[2]

	if basicTx0.Type != "swap" || basicTx0.Height != "3" {
		t.Fatal("Results of results changed.")
	}
	if basicTx1.Type != "withdraw" || basicTx1.Height != "2" {
		t.Fatal("Results of results changed.")
	}
	if basicTx2.Type != "addLiquidity" || basicTx2.Height != "1" {
		t.Fatal("Results of results changed.")
	}

	// Filter by type request
	body = testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0&type=swap")

	testdb.MustUnmarshal(t, body, &v)

	require.Equal(t, "1", v.Count)
	typeTx0 := v.Actions[0]

	if typeTx0.Type != "swap" {
		t.Fatal("Results of results changed.")
	}

	// Filter by asset request
	body = testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0&asset=BNB.TWT-123")

	testdb.MustUnmarshal(t, body, &v)

	require.Equal(t, "2", v.Count)
	assetTx0 := v.Actions[0]
	assetTx1 := v.Actions[1]

	if assetTx0.Type != "withdraw" {
		t.Fatal("Results of results changed.")
	}
	if assetTx1.Type != "addLiquidity" {
		t.Fatal("Results of results changed.")
	}
}

func txResponseCount(t *testing.T, url string) string {
	body := testdb.CallJSON(t, url)

	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)
	return v.Count
}

func TestDepositStakeByTxIds(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.AddLiquidity{
			Pool:        "BNB.TWT-123",
			RuneAddress: "thoraddr1",
			RuneTxID:    "RUNETX1",
			AssetTxID:   "ASSETTX1",
			AssetAmount: 1000,
			RuneAmount:  2000,
		},
		testdb.PoolActivate{Pool: "BNB.TWT-123"})

	require.Equal(t, "1", txResponseCount(t,
		"http://localhost:8080/v2/actions?limit=50&offset=0"))
	require.Equal(t, "0", txResponseCount(t,
		"http://localhost:8080/v2/actions?txid=NOSUCHID&limit=50&offset=0"))
	require.Equal(t, "1", txResponseCount(t,
		"http://localhost:8080/v2/actions?txid=ASSETTX1&limit=50&offset=0"))
	require.Equal(t, "1", txResponseCount(t,
		"http://localhost:8080/v2/actions?txid=RUNETX1&limit=50&offset=0"))
}

func TestPendingAlone(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-01 00:00:00",
		testdb.PoolActivate{Pool: "BTC.BTC"},
		testdb.PoolActivate{Pool: "LTC.LTC"})

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.PendingLiquidity{
			Pool:         "BTC.BTC",
			RuneAddress:  "thoraddr1",
			AssetAddress: "btcaddr1",
			RuneTxID:     "RUNETX1",
			AssetAmount:  0,
			RuneAmount:   20,
		})

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0")

	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)

	require.Equal(t, 1, len(v.Actions))
	add := v.Actions[0]

	require.Equal(t, "addLiquidity", string(add.Type))
	require.Equal(t, "pending", string(add.Status))
}

func TestPendingWithAdd(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-01 00:00:00",
		testdb.PoolActivate{Pool: "BTC.BTC"},
		testdb.PoolActivate{Pool: "LTC.LTC"})

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.PendingLiquidity{
			Pool:         "BTC.BTC",
			RuneAddress:  "thoraddr1",
			AssetAddress: "btcaddr1",
			RuneTxID:     "RUNETX1",
			AssetAmount:  0,
			RuneAmount:   20,
		},
		testdb.AddLiquidity{
			Pool:        "BTC.BTC",
			RuneAddress: "thoraddr1",
			RuneTxID:    "RUNETX1",
			AssetTxID:   "ASSETTX1",
			AssetAmount: 10,
			RuneAmount:  20,
		})

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0")

	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)

	require.Equal(t, 1, len(v.Actions))
	add := v.Actions[0]

	require.Equal(t, "addLiquidity", string(add.Type))
	require.Equal(t, "success", string(add.Status))
}

func TestPendingWithdrawn(t *testing.T) {
	// TODO(muninn): report withdraws of pending too. Currently we simply don't show anything.
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-01-01 00:00:00",
		testdb.PoolActivate{Pool: "BTC.BTC"},
		testdb.PoolActivate{Pool: "LTC.LTC"})

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.PendingLiquidity{
			Pool:         "BTC.BTC",
			RuneAddress:  "thoraddr1",
			AssetAddress: "btcaddr1",
			RuneTxID:     "RUNETX1",
			AssetAmount:  0,
			RuneAmount:   20,
		})

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.PendingLiquidity{
			Pool:         "BTC.BTC",
			RuneAddress:  "thoraddr1",
			AssetAddress: "btcaddr1",
			RuneTxID:     "RUNETX1",
			AssetAmount:  0,
			RuneAmount:   20,
			PendingType:  testdb.PendingWithdraw,
		})

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0")

	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)

	require.Equal(t, 0, len(v.Actions))
}

func TestDoubleSwap(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-03 00:00:00",
		testdb.AddLiquidity{
			Pool: "POOL1.A", AssetAmount: 1000, RuneAmount: 2000, AssetAddress: "thoraddr1"},
		testdb.PoolActivate{Pool: "POOL1.A"})

	blocks.NewBlock(t, "2020-09-02 00:00:00",
		testdb.Swap{
			TxID:         "double",
			Coin:         "100000 BNB.BNB",
			EmitAsset:    "10 THOR.RUNE",
			Pool:         "BNB.BNB",
			Slip:         100,
			LiquidityFee: 10000,
		},
		testdb.Swap{
			TxID:         "double",
			Coin:         "10 THOR.RUNE",
			EmitAsset:    "55000 BTC.BTC",
			Pool:         "BTC.BTC",
			Slip:         200,
			LiquidityFee: 20000,
			PriceTarget:  50000,
		},
		testdb.PoolActivate{Pool: "BNB.BNB"},
		testdb.PoolActivate{Pool: "BTC.BTC"},
	)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0&type=swap")

	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)

	doubleSwap := v.Actions[0]
	metadata := doubleSwap.Metadata.Swap
	require.Equal(t, "298", metadata.SwapSlip) // 100+200-(100*200)/10000
	require.Equal(t, "30000", metadata.LiquidityFee)
	require.Equal(t, "50000", metadata.SwapTarget)
}

func TestSwitch(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-02 00:00:00",
		testdb.Switch{
			FromAddress: "B2",
			ToAddress:   "THOR2",
			Burn:        "200 BNB.RUNE-B1A",
		})

	blocks.NewBlock(t, "2020-09-03 00:00:00",
		testdb.Switch{
			FromAddress: "A1",
			ToAddress:   "THOR1",
			Burn:        "100 BNB.RUNE-B1A",
		})

	body := testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=50&offset=0&type=switch")

	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)

	require.Len(t, v.Actions, 2)

	switch0 := v.Actions[0]
	require.Equal(t, "switch", string(switch0.Type))
	require.Equal(t, "A1", switch0.In[0].Address)
	require.Equal(t, "100", switch0.In[0].Coins[0].Amount)
	require.Equal(t, "THOR1", switch0.Out[0].Address)
	require.Equal(t, "THOR.RUNE", switch0.Out[0].Coins[0].Asset)
	require.Equal(t, "100", switch0.Out[0].Coins[0].Amount)

	switch2 := v.Actions[1]
	require.Equal(t, "B2", switch2.In[0].Address)
	require.Equal(t, "200", switch2.In[0].Coins[0].Amount)

	// address filter
	body = testdb.CallJSON(t,
		"http://localhost:8080/v2/actions?limit=50&offset=0&type=switch&address=B2")
	testdb.MustUnmarshal(t, body, &v)
	require.Len(t, v.Actions, 1)
	require.Equal(t, "B2", v.Actions[0].In[0].Address)

	// address filter 2
	body = testdb.CallJSON(t,
		"http://localhost:8080/v2/actions?limit=50&offset=0&type=switch&address=THOR2")
	testdb.MustUnmarshal(t, body, &v)
	require.Len(t, v.Actions, 1)
	require.Equal(t, "B2", v.Actions[0].In[0].Address)
}

func checkFilter(t *testing.T, urlPostfix string, expectedResultsPool []string) {
	body := testdb.CallJSON(t,
		"http://localhost:8080/v2/actions?limit=50&offset=0"+urlPostfix)
	var v oapigen.ActionsResponse
	testdb.MustUnmarshal(t, body, &v)

	require.Equal(t, strconv.Itoa(len(expectedResultsPool)), v.Count)
	for i, pool := range expectedResultsPool {
		require.Equal(t, []string{pool}, v.Actions[i].Pools)
	}
}

func TestAdderessFilter(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.AddLiquidity{
			Pool: "POOL1.A", AssetAmount: 1000, RuneAmount: 2000, AssetAddress: "thoraddr1"},
		testdb.PoolActivate{Pool: "POOL1.A"})

	blocks.NewBlock(t, "2020-09-02 00:00:00",
		testdb.Swap{
			Pool:        "POOL2.A",
			Coin:        "20 POOL2.A",
			EmitAsset:   "10 THOR.RUNE",
			FromAddress: "thoraddr2",
			ToAddress:   "thoraddr3",
		},
		testdb.PoolActivate{Pool: "POOL2.A"})

	blocks.NewBlock(t, "2020-09-03 00:00:00",
		testdb.Withdraw{
			Pool:      "POOL3.A",
			EmitAsset: 10,
			EmitRune:  20,
			ToAddress: "thoraddr4",
		},
		testdb.PoolActivate{Pool: "POOL3.A"})

	checkFilter(t, "", []string{"POOL3.A", "POOL2.A", "POOL1.A"})
	checkFilter(t, "&address=thoraddr1", []string{"POOL1.A"})
	checkFilter(t, "&address=thoraddr2", []string{"POOL2.A"})
	checkFilter(t, "&address=thoraddr4", []string{"POOL3.A"})

	checkFilter(t, "&address=thoraddr1,thoraddr4", []string{"POOL3.A", "POOL1.A"})
}

func TestAddLiquidityAddress(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.AddLiquidity{
			Pool: "POOL1.A", AssetAmount: 1000, RuneAmount: 2000, AssetAddress: "thoraddr1"},
		testdb.PoolActivate{Pool: "POOL1.A"})

	checkFilter(t, "&address=thoraddr1", []string{"POOL1.A"})
}

func TestAffiliateFee(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	tx1 := "TX1"
	tx2 := "TX2"

	t.Run("database", func(t *testing.T) {
		testdb.InitTest(t)

		bts := "2020-09-01 00:00:00"
		testdb.InsertBlockLog(t, 1, bts)
		testdb.InsertSwapEvent(t, testdb.FakeSwap{Tx: tx1, FromAsset: "ETH.ETH", FromE8: 9990000, ToAsset: "THOR.RUNE", ToE8: 2141935865, BlockTimestamp: bts})
		testdb.InsertSwapEvent(t, testdb.FakeSwap{Tx: tx1, FromAsset: "ETH.ETH", FromE8: 10000, ToAsset: "THOR.RUNE", ToE8: 2144079, BlockTimestamp: bts})
		testdb.InsertFeeEvent(t, testdb.FakeFee{Tx: tx1, Asset: "THOR.RUNE", BlockTimestamp: bts})
		testdb.InsertFeeEvent(t, testdb.FakeFee{Tx: tx1, Asset: "THOR.RUNE", BlockTimestamp: bts})
		testdb.InsertSwapEvent(t, testdb.FakeSwap{Tx: tx2, FromAsset: "BNB.BNB", FromE8: 654321, ToAsset: "THOR.RUNE", ToE8: 555555555, BlockTimestamp: bts})
		testdb.InsertSwapEvent(t, testdb.FakeSwap{Tx: tx2, FromAsset: "BNB.BNB", FromE8: 4242, ToAsset: "THOR.RUNE", ToE8: 1111111, BlockTimestamp: bts})
		testdb.InsertFeeEvent(t, testdb.FakeFee{Tx: tx2, Asset: "THOR.RUNE", BlockTimestamp: bts})
		testdb.InsertFeeEvent(t, testdb.FakeFee{Tx: tx2, Asset: "THOR.RUNE", BlockTimestamp: bts})

		// Request for one transaction only
		body := testdb.CallJSON(t, fmt.Sprintf("http://localhost:8080/v2/actions?limit=10&offset=0&txid=%s", tx1))

		var v oapigen.ActionsResponse
		testdb.MustUnmarshal(t, body, &v)

		if v.Count != "2" {
			t.Fatal("Expected two values")
		}

		// Ought to be one network fee per swap
		if len(v.Actions[0].Metadata.Swap.NetworkFees) != 1 {
			t.Fatalf("Expected 1 fee per swap, got %d", len(v.Actions[0].Metadata.Swap.NetworkFees))
		}
		if len(v.Actions[1].Metadata.Swap.NetworkFees) != 1 {
			t.Fatalf("Expected 1 fee per swap, got %d", len(v.Actions[1].Metadata.Swap.NetworkFees))
		}

		// Request for two transactions
		body = testdb.CallJSON(t, "http://localhost:8080/v2/actions?limit=10&offset=0")
		testdb.MustUnmarshal(t, body, &v)

		if v.Count != "4" {
			t.Fatalf("Expected 4 values, got %s", v.Count)
		}

		// Ought to be one network fee per swap
		for i := range v.Actions {
			if len(v.Actions[i].Metadata.Swap.NetworkFees) != 1 {
				t.Fatalf("Expected 1 fee per swap, got %d", len(v.Actions[i].Metadata.Swap.NetworkFees))
			}
		}
	})

	t.Run("blocks", func(t *testing.T) {
		// Pool         string
		// Coin         string
		// EmitAsset    string
		// LiquidityFee int64
		// Slip         int64
		// FromAddress  string
		// ToAddress    string
		// TxID         string
		// PriceTarget  int64
		blocks.NewBlock(t, "2020-09-01 00:00:00",
			testdb.Swap{Pool: "ETH.ETH", TxID: tx1, Coin: "9990000 ETH.ETH", EmitAsset: "2141935865 THOR.RUNE"},
			testdb.Swap{Pool: "ETH.ETH", TxID: tx1, Coin: "10000 ETH.ETH", EmitAsset: "2144079 THOR.RUNE"},
			// testdb.Fee{TxID: tx1, Asset: "THOR.RUNE"},
			// testdb.Fee{TxID: tx1, Asset: "THOR.RUNE"},
			// testdb.Swap{Pool: "BNB.BNB", TxID: tx2, Coin: "654321 BNB.BNB", EmitAsset: "555555555 THOR.RUNE"},
			// testdb.Swap{Pool: "BNB.BNB", TxID: tx2, Coin: "4242 BNB.BNB", EmitAsset: "1111111 THOR.RUNE"},
			// testdb.Fee{TxID: tx2, Asset: "THOR.RUNE"},
			// testdb.Fee{TxID: tx2, Asset: "THOR.RUNE"},
		)
		// TODO: Second block with second/third transaction

		checkFilter(t, fmt.Sprintf("&txid=%s", tx1), []string{"foo", "BaR"})

		checkFilter(t, "", []string{})
	})
}
