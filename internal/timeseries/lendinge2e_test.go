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
			CollateralUp:           200,
			DebtUpTor:              14,
			CollateralAsset:        "BTC.BTC",
			CollateralizationRatio: 30000,
			TargetAsset:            "ETH.USDT",
		},
	)

	blocks.NewBlock(t, "2020-09-01 00:20:00",
		testdb.LoanRepayment{
			Owner:           "btcaddr1",
			CollateralDown:  50,
			DebtDownTor:     7,
			CollateralAsset: "BTC.BTC",
		},
	)

	blocks.NewBlock(t, "2020-09-02 00:10:00",
		testdb.LoanRepayment{
			Owner:           "btcaddr1",
			CollateralDown:  50,
			DebtDownTor:     7,
			CollateralAsset: "BTC.BTC",
		},
	)

	body := testdb.CallJSON(t, "http://localhost:8080/v2/borrower/btcaddr1")

	var jsonApiResult oapigen.BorrowerDetailsResponse
	testdb.MustUnmarshal(t, body, &jsonApiResult)

	require.Equal(t, 1, len(jsonApiResult.Pools))
	btcDebt := jsonApiResult.Pools[0]
	require.Equal(t, "BTC.BTC", btcDebt.CollateralAsset)
	require.Equal(t, "200", btcDebt.CollateralUp)
	require.Equal(t, "100", btcDebt.CollateralDown)
	require.Equal(t, "14", btcDebt.DebtUpTor)
	require.Equal(t, "14", btcDebt.DebtDownTor)
	require.Equal(t, "1598919000", btcDebt.LastOpenLoanTimestamp)
	require.Equal(t, "1599005400", btcDebt.LastRepayLoanTimestamp)
}
