package timeseries_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func TestSimpleBorrowersE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:10:00",
		testdb.LoanOpen{
			Owner:                  "btcaddr1",
			CollateralUp:           20,
			DebtUp:                 10,
			CollateralAsset:        "BTC.BTC",
			CollateralizationRatio: 50000,
			TargetAsset:            "ETH.USDT",
		},
	)

	blocks.NewBlock(t, "2020-09-01 00:20:00",
		testdb.LoanRepayment{
			Owner:           "btcaddr1",
			CollateralDown:  10,
			DebtDown:        5,
			CollateralAsset: "BTC.BTC",
		},
	)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/borrower/btcaddr1")

	var jsonApiResult oapigen.BorrowerDetailsResponse
	testdb.MustUnmarshal(t, body, &jsonApiResult)

	require.Equal(t, 1, len(jsonApiResult.Pools))
	btcDebt := jsonApiResult.Pools[0]
	require.Equal(t, "BTC.BTC", btcDebt.CollateralAsset)
	require.Equal(t, "30", btcDebt.CollateralUp)
	require.Equal(t, "10", btcDebt.CollateralDown)
	require.Equal(t, "10", btcDebt.DebtUpTor)
	require.Equal(t, "5", btcDebt.DebtDownTor)
	require.Equal(t, "1598919000", btcDebt.LastOpenLoanTimestamp)
	require.Equal(t, "1598919600", btcDebt.LastRepayLoanTimestamp)
}
