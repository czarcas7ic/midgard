package timeseries

import (
	"context"

	"github.com/lib/pq"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

type Borrower struct {
	CollateralAsset        string
	TargetAssets           []string
	DebtUpTor              int64
	DebtDownTor            int64
	CollateralUp           int64
	CollateralDown         int64
	LastOpenLoanTimestamp  int64
	LastRepayLoanTimestamp int64
}

func (borrower Borrower) toOapigen() oapigen.BorrowerPool {
	return oapigen.BorrowerPool{
		CollateralAsset:        borrower.CollateralAsset,
		TargetAssets:           borrower.TargetAssets,
		DebtUpTor:              util.IntStr(borrower.DebtUpTor),
		DebtDownTor:            util.IntStr(borrower.DebtDownTor),
		CollateralUp:           util.IntStr(borrower.CollateralUp),
		CollateralDown:         util.IntStr(borrower.CollateralDown),
		LastOpenLoanTimestamp:  util.IntStr(borrower.LastOpenLoanTimestamp),
		LastRepayLoanTimestamp: util.IntStr(borrower.LastRepayLoanTimestamp),
	}
}

type Borrowers []Borrower

func (borrowers Borrowers) ToOapigen() []oapigen.BorrowerPool {
	ret := make([]oapigen.BorrowerPool, len(borrowers))
	for i, borrower := range borrowers {
		ret[i] = borrower.toOapigen()
	}

	return ret
}

func GetBorrower(ctx context.Context, address []string) (Borrowers, error) {
	q := `
		SELECT
			collateral_asset,
			target_assets,
			debt_up,
			debt_down,
			collateral_up,
			collateral_down,
			COALESCE(last_open_loan_timestamp / 1000000000, 0),
			COALESCE(last_repay_loan_timestamp / 1000000000, 0)
		FROM midgard_agg.borrowers
		WHERE borrower_id = ANY($1)
		ORDER BY collateral_asset
	`

	rows, err := db.Query(ctx, q, pq.Array(address))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results Borrowers
	for rows.Next() {
		var entry Borrower
		err := rows.Scan(
			&entry.CollateralAsset,
			pq.Array(&entry.TargetAssets),
			&entry.DebtUpTor,
			&entry.DebtDownTor,
			&entry.CollateralUp,
			&entry.CollateralDown,
			&entry.LastOpenLoanTimestamp,
			&entry.LastRepayLoanTimestamp,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, entry)
	}
	return results, nil
}

type LendingInfo struct {
	TotalCollateral    int64
	TotalDebtTor       int64
	TotalCollateralTor int64
}

func GetLendingData(ctx context.Context) (map[string]LendingInfo, error) {
	q := `
		SELECT
			collateral_asset,
			SUM(collateral_up) - SUM(collateral_down) AS total_collateral, 
			SUM(debt_up) - SUM(debt_down) as total_debt_tor,
			SUM(total_collateral_tor) AS total_collateral_tor
		FROM midgard_agg.borrowers
		GROUP BY collateral_asset
	`

	mapLendingInfo := make(map[string]LendingInfo)
	rows, err := db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var asset string
		lendingInfo := LendingInfo{}
		err := rows.Scan(
			&asset,
			&lendingInfo.TotalCollateral,
			&lendingInfo.TotalDebtTor,
			&lendingInfo.TotalCollateralTor,
		)
		if err != nil {
			return nil, err
		}
		mapLendingInfo[asset] = lendingInfo
	}

	return mapLendingInfo, nil
}
