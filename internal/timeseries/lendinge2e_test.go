package timeseries_test

import (
	"testing"

	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func TestSimpleBorrowersE2E(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:10:00",
		testdb.LoanOpen{
			Owner:                  "bnbaddr2",
			CollateralUp:           2,
			DebtUp:                 1,
			CollateralAsset:        "BNB.ASSET1",
			CollateralizationRatio: 50000,
			TargetAsset:            "BNB.ASSET2",
		},
	)

	blocks.NewBlock(t, "2020-09-01 00:20:00",
		testdb.LoanRepayment{
			Owner:           "bnbaddr2",
			CollateralDown:  2,
			DebtDown:        1,
			CollateralAsset: "BNB.ASSET1",
		},
	)

	blocks.NewBlock(t, "2020-09-02 00:10:00",
		testdb.LoanOpen{
			Owner:                  "bnbaddr1",
			CollateralUp:           2,
			DebtUp:                 1,
			CollateralAsset:        "BNB.ASSET1",
			CollateralizationRatio: 50000,
			TargetAsset:            "BNB.ASSET2",
		},
	)

	blocks.NewBlock(t, "2020-09-02 00:20:00",
		testdb.LoanOpen{
			Owner:                  "bnbaddr1",
			CollateralUp:           2,
			DebtUp:                 1,
			CollateralAsset:        "BNB.ASSET1",
			CollateralizationRatio: 50000,
			TargetAsset:            "BNB.ASSET2",
		},
		testdb.LoanOpen{
			Owner:                  "bnbaddr2",
			CollateralUp:           2,
			DebtUp:                 1,
			CollateralAsset:        "BNB.ASSET1",
			CollateralizationRatio: 50000,
			TargetAsset:            "BNB.ASSET2",
		},
	)

	midlog.Debug("Here is a test.")

}
