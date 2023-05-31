package timeseries

import (
	"context"

	"github.com/lib/pq"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

// GetMemberIds returns the ids of all known members.
//
// The id of a member is defined as their rune address if they are participating with their rune
// address, or as their asset address otherwise (for members with asset address only.)
//
// Member ids present in multiple pools will be only returned once.
func GetMemberIds(ctx context.Context, pool *string) (addrs []string, err error) {
	poolFilter := ""
	qargs := []interface{}{}
	if pool != nil {
		poolFilter = "pool = $1"
		qargs = append(qargs, pool)
	}

	q := "SELECT DISTINCT member_id FROM midgard_agg.members " + db.Where(poolFilter)

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var member string
		err := rows.Scan(&member)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, member)
	}

	return addrs, nil
}

func GetBorrowerIds(ctx context.Context, asset *string) (addrs []string, err error) {
	assetFilter := ""
	qargs := []interface{}{}
	if asset != nil {
		assetFilter = "collateral_asset = $1"
		qargs = append(qargs, asset)
	}

	q := "SELECT DISTINCT borrower_id FROM midgard_agg.borrowers " + db.Where(assetFilter)

	rows, err := db.Query(ctx, q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var borrower string
		err := rows.Scan(&borrower)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, borrower)
	}

	return addrs, nil
}

// TODO(HooriRn): this struct might not be needed since the graphql depracation. (delete-graphql)
// Info of a member in a specific pool.
type MemberPool struct {
	Pool           string
	RuneAddress    string
	AssetAddress   string
	LiquidityUnits int64
	AssetDeposit   int64
	RuneDeposit    int64
	RuneAdded      int64
	AssetAdded     int64
	RunePending    int64
	AssetPending   int64
	DateFirstAdded int64
	DateLastAdded  int64
	RuneWithdrawn  int64
	AssetWithdrawn int64
}

func (memberPool MemberPool) toOapigen() oapigen.MemberPool {
	return oapigen.MemberPool{
		Pool:           memberPool.Pool,
		RuneAddress:    memberPool.RuneAddress,
		AssetAddress:   memberPool.AssetAddress,
		LiquidityUnits: util.IntStr(memberPool.LiquidityUnits),
		RuneDeposit:    util.IntStr(memberPool.RuneDeposit),
		AssetDeposit:   util.IntStr(memberPool.AssetDeposit),
		RuneAdded:      util.IntStr(memberPool.RuneAdded),
		AssetAdded:     util.IntStr(memberPool.AssetAdded),
		RuneWithdrawn:  util.IntStr(memberPool.RuneWithdrawn),
		AssetWithdrawn: util.IntStr(memberPool.AssetWithdrawn),
		RunePending:    util.IntStr(memberPool.RunePending),
		AssetPending:   util.IntStr(memberPool.AssetPending),
		DateFirstAdded: util.IntStr(memberPool.DateFirstAdded),
		DateLastAdded:  util.IntStr(memberPool.DateLastAdded),
	}
}

func (memberPool MemberPool) toSavers() oapigen.SaverPool {
	return oapigen.SaverPool{
		Pool:           util.ConvertSynthPoolToNative(memberPool.Pool),
		AssetAddress:   memberPool.AssetAddress,
		AssetAdded:     util.IntStr(memberPool.AssetAdded),
		AssetDeposit:   util.IntStr(memberPool.AssetDeposit),
		SaverUnits:     util.IntStr(memberPool.LiquidityUnits),
		AssetWithdrawn: util.IntStr(memberPool.AssetWithdrawn),
		DateFirstAdded: util.IntStr(memberPool.DateFirstAdded),
		DateLastAdded:  util.IntStr(memberPool.DateLastAdded),
	}
}

// Pools data associated with a single member
type MemberPools []MemberPool

func (memberPools MemberPools) ToOapigen() []oapigen.MemberPool {
	ret := make([]oapigen.MemberPool, len(memberPools))
	for i, memberPool := range memberPools {
		ret[i] = memberPool.toOapigen()
	}

	return ret
}

func (memberPools MemberPools) ToSavers(poolRedeemValueMap map[string]int64) []oapigen.SaverPool {
	ret := make([]oapigen.SaverPool, len(memberPools))
	for i, memberPool := range memberPools {
		ret[i] = memberPool.toSavers()
		ret[i].AssetRedeem = util.IntStr(poolRedeemValueMap[memberPool.Pool])
	}

	return ret
}

type MemberPoolType int

const (
	RegularAndSaverPools MemberPoolType = iota // regular and synth pools too
	RegularPools                               // regular (non-synth) pools e.g. 'BTC.BTC'
	SaverPools                                 // LPs of synth pools e.g. 'BTC/BTC'
)

func PoolBasedOfType(poolName string, poolType MemberPoolType) bool {
	if poolType == RegularAndSaverPools {
		return true
	}
	poolCoinType := record.GetCoinType([]byte(poolName))
	if poolCoinType == record.AssetSynth && poolType == SaverPools {
		return true
	}
	if poolCoinType == record.AssetNative && poolType == RegularPools {
		return true
	}
	return false
}

func GetMemberPools(ctx context.Context, address []string, poolType MemberPoolType) (MemberPools, error) {
	q := `
		SELECT
			pool,
			COALESCE(rune_addr, ''),
			COALESCE(asset_addr, ''),
			lp_units_total,
			asset_e8_deposit,
			rune_e8_deposit,
			added_rune_e8_total,
			added_asset_e8_total,
			withdrawn_rune_e8_total,
			withdrawn_asset_e8_total,
			pending_rune_e8_total,
			pending_asset_e8_total,
			COALESCE(first_added_timestamp / 1000000000, 0),
			COALESCE(last_added_timestamp / 1000000000, 0)
		FROM midgard_agg.members
		WHERE member_id = ANY($1) OR asset_addr = ANY($1)
		ORDER BY pool
	`

	rows, err := db.Query(ctx, q, pq.Array(address))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results MemberPools
	for rows.Next() {
		var entry MemberPool
		err := rows.Scan(
			&entry.Pool,
			&entry.RuneAddress,
			&entry.AssetAddress,
			&entry.LiquidityUnits,
			&entry.AssetDeposit,
			&entry.RuneDeposit,
			&entry.RuneAdded,
			&entry.AssetAdded,
			&entry.RuneWithdrawn,
			&entry.AssetWithdrawn,
			&entry.RunePending,
			&entry.AssetPending,
			&entry.DateFirstAdded,
			&entry.DateLastAdded,
		)
		if err != nil {
			return nil, err
		}
		if PoolBasedOfType(entry.Pool, poolType) {
			results = append(results, entry)
		}
	}
	return results, nil
}

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
