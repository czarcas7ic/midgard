// In 2021-04 ThorNode had two bug when withdrawing with impermanent loss.
// Bigger pool units were reported for withdraws than the actual remove:
// https://gitlab.com/thorchain/thornode/-/issues/912
//
// Sometimes when withdrawing the pool units of a member went up, not down:
// https://gitlab.com/thorchain/thornode/-/issues/896
//
// This script fetches ThorNode at each point where we had a withdraw with impermanent loss
// and prints out the corrections to those withdraws.
//
// There would be two ways of querying ThorNode for pool units.
//  1. Go to the pool page and read the total units of the pool. Sum up the withdraws and adds
//     form MidgardDb, and if there is a discrepancy with the previous block assume it's because
//     of the withdraw with impermanent loss.
//     https://thornode.thorchain.info/thorchain/pool/BNB.BNB?height=50000
//  2. Go to the liquidity_providers page and check how much the pool units of that individual
//     member changes.
//     https://thornode.thorchain.info/thorchain/pool/BNB.BNB/liquidity_providers?height=81054
//
// Here we do 1, but mannually was checked that 2 would give consistent results.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func main() {
	midlog.LogCommandLine()
	config.ReadGlobal()

	ctx := context.Background()

	dbinit.Setup()

	db.InitializeChainVarsFromThorNode()
	db.EnsureDBMatchesChain()

	summaries := withdrawsWithImpermanentLoss(ctx)
	midlog.InfoF("Withdraws count with impermanent loss protection: %d ", len(summaries))

	type WithdrawCorrection struct {
		TX          string
		ActualUnits int64
	}
	sortedWithdrawKeys := []int64{}
	correctWithdraws := map[int64]WithdrawCorrection{}

	type AddCorrection struct {
		RuneAddr string
		Units    int64
		Pool     string
	}
	adds := map[int64]AddCorrection{}
	sortedAddKeys := []int64{}
	for _, summary := range summaries {
		checkWithdrawsIsAlone(ctx, summary)
		readAdds(ctx, &summary)
		fetchNodeUnits(&summary)
		nodeWithdraw := summary.NodeDiff + summary.Adds
		if nodeWithdraw == summary.MidgardWithrawUnits {
			// No problems with this withdraw
			continue
		}
		if summary.Adds != 0 {
			midlog.Info("Pool has add in the same block")
		}
		diff := float64(nodeWithdraw-summary.MidgardWithrawUnits) / float64(summary.MidgardWithrawUnits)
		midlog.InfoF("Diff %f Pool: %s height %d -- [ %d vs %d ]",
			diff, summary.Pool, summary.Height, summary.MidgardWithrawUnits, (nodeWithdraw))
		if -0.2 <= diff && diff <= 0 {
			correctWithdraws[summary.Height] = WithdrawCorrection{
				TX:          summary.TX,
				ActualUnits: nodeWithdraw,
			}
			sortedWithdrawKeys = append(sortedWithdrawKeys, summary.Height)
		} else {
			midlog.Warn("Big impermanent loss change, creating append")
			if -1 <= diff {
				midlog.Fatal("Big impermanent loss but not addition yet")
			}
			adds[summary.Height] = AddCorrection{
				RuneAddr: summary.FromAddr,
				Units:    summary.MidgardWithrawUnits - nodeWithdraw, // nodeWithdraw is negative
				Pool:     summary.Pool,
			}
			sortedAddKeys = append(sortedAddKeys, summary.Height)
		}
	}
	midlog.InfoF("correct withdraws: %v", correctWithdraws)
	midlog.InfoF("adds: %v", adds)
	withdrawString := "var withdrawUnitCorrections = map[int64]withdrawUnitCorrection{\n"

	for _, k := range sortedWithdrawKeys {
		v := correctWithdraws[k]
		withdrawString += fmt.Sprintf(
			"\t%d: {\"%s\", %d},\n", k, v.TX, v.ActualUnits)
	}
	withdrawString += "}\n"
	midlog.WarnF("Correct withdraws:\n%s", withdrawString)

	addString := "var addInsteadWithdrawMap = map[int64]addInsteadWithdraw{\n"
	for _, k := range sortedAddKeys {
		v := adds[k]
		addString += fmt.Sprintf(
			"\t%d: {\"%s\", \"%s\", %d},\n", k, v.Pool, v.RuneAddr, v.Units)
	}
	addString += "}\n"
	midlog.WarnF("Additional adds:\n%s", addString)
}

type UnitsSummary struct {
	Pool                string
	TX                  string
	MidgardWithrawUnits int64
	FromAddr            string
	Adds                int64
	NodeDiff            int64
	Timestamp           db.Nano
	Height              int64
}

func withdrawsWithImpermanentLoss(ctx context.Context) []UnitsSummary {
	ret := []UnitsSummary{}
	q := `
		SELECT
			x.pool,
			x.tx,
			x.stake_units,
			x.from_addr,
			x.block_timestamp,
			b.height
		FROM withdraw_events AS x JOIN block_log AS b ON x.block_timestamp = b.timestamp
		WHERE imp_loss_protection_e8 <> 0
		ORDER BY height
	`
	rows, err := db.Query(ctx, q)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	for rows.Next() {
		var w UnitsSummary
		err := rows.Scan(
			&w.Pool, &w.TX, &w.MidgardWithrawUnits, &w.FromAddr, &w.Timestamp, &w.Height)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
		ret = append(ret, w)
	}
	return ret
}

func checkWithdrawsIsAlone(ctx context.Context, summary UnitsSummary) {
	q := `select count(*) from withdraw_events where block_timestamp = $1`

	rows, err := db.Query(ctx, q, summary.Timestamp)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	if !rows.Next() {
		midlog.Fatal("Expected one row from count")
	}
	var count int
	err = rows.Scan(&count)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	if count != 1 {
		midlog.FatalF(
			"Multiple withdraws at timestamp %d, height %d",
			summary.Timestamp, summary.Height)
	}
}

func readAdds(ctx context.Context, summary *UnitsSummary) {
	q := `
		SELECT COALESCE(SUM(stake_units), 0)
		FROM deposit_events
		WHERE pool=$1 AND block_timestamp = $2`

	rows, err := db.Query(ctx, q, summary.Pool, summary.Timestamp)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer rows.Close()

	if !rows.Next() {
		midlog.Fatal("Expected one row adds")
	}
	err = rows.Scan(&summary.Adds)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	midlog.DebugF("Add: %d", summary.Adds)
}

type ThorNodeUnits struct {
	TotalUnits int64 `json:"pool_units,string"`
}

func NodeUnits(thorNodeUrl string, urlPath string, height int64) int64 {
	url := thorNodeUrl + urlPath + "?height=" + strconv.FormatInt(height, 10)
	midlog.DebugF("Querying thornode: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		midlog.FatalE(err, "Error fetching ThorNode")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	var result ThorNodeUnits
	err = json.Unmarshal(body, &result)
	if err != nil {
		midlog.FatalE(err, "Error unmarshaling ThorNode response")
	}
	return result.TotalUnits
}

func fetchNodeUnits(summary *UnitsSummary) {
	thorNodeUrl := config.Global.ThorChain.ThorNodeURL
	unitsBefore := NodeUnits(thorNodeUrl, "/pool/"+summary.Pool, summary.Height-1)
	unitsAfter := NodeUnits(thorNodeUrl, "/pool/"+summary.Pool, summary.Height)
	summary.NodeDiff = unitsBefore - unitsAfter
	midlog.DebugF("before %d  after %d  diff %d", unitsBefore, unitsAfter, summary.NodeDiff)
}
