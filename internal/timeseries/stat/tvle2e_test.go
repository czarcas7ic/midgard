package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/api"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func stringp(s string) *string {
	return &s
}

func TestTVLHistoryE2E(t *testing.T) {
	// Run the test with bonds enabled to test the whole logic
	api.ShowBonds = true

	testdb.InitTest(t)
	testdb.DeclarePools("ABC.ABC", "ABC.XYZ", "ABC.USD1", "ABC.USD2")
	config.Global.UsdPools = []string{"ABC.USD1", "ABC.USD2"}

	// This will be skipped because we query 01-09 to 01-14
	testdb.InsertBlockPoolDepth(t, "ABC.ABC", 1000, 1, "2020-01-14 12:00:00")

	// This will be the initial value
	// Not all pools existed before the start time
	testdb.InsertBlockPoolDepth(t, "ABC.XYZ", 10, 100, "2020-01-05 12:00:00")
	testdb.InsertBlockPoolDepth(t, "ABC.USD1", 100, 10, "2020-01-05 12:00:00")

	testdb.InsertBlockPoolDepth(t, "ABC.ABC", 10, 20, "2020-01-10 12:00:05")
	testdb.InsertBlockPoolDepth(t, "ABC.ABC", 20, 30, "2020-01-10 14:00:00")

	testdb.InsertBlockPoolDepth(t, "ABC.XYZ", 20, 150, "2020-01-11 14:00:00")

	testdb.InsertBlockPoolDepth(t, "ABC.USD1", 0, 0, "2020-01-13 07:00:00")
	testdb.InsertBlockPoolDepth(t, "ABC.USD2", 20, 10, "2020-01-13 08:00:00")
	testdb.InsertBlockPoolDepth(t, "ABC.ABC", 2, 5, "2020-01-13 09:00:00")
	testdb.InsertBlockPoolDepth(t, "ABC.ABC", 6, 18, "2020-01-13 10:00:00")

	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-14 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/tvl?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.TVLHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Meta.StartTime)
	require.Equal(t, epochStr("2020-01-14 00:00:00"), jsonResult.Meta.EndTime)
	require.Equal(t, "356", jsonResult.Meta.TotalValuePooled)
	require.Equal(t, stringp("0"), jsonResult.Meta.TotalValueBonded)
	require.Equal(t, stringp("356"), jsonResult.Meta.TotalValueLocked)
	require.Equal(t, "2", jsonResult.Meta.RunePriceUSD)
	require.Equal(t, 4, len(jsonResult.Meta.PoolsDepth))
	for _, item := range jsonResult.Meta.PoolsDepth {
		switch item.Pool {
		case "ABC.USD1":
			require.Equal(t, "0", item.TotalDepth)
		case "ABC.XYZ":
			require.Equal(t, "300", item.TotalDepth)
		case "ABC.ABC":
			require.Equal(t, "36", item.TotalDepth)
		case "ABC.USD2":
			require.Equal(t, "20", item.TotalDepth)
		}
	}

	require.Equal(t, 5, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-01-10 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, epochStr("2020-01-14 00:00:00"), jsonResult.Intervals[4].EndTime)

	require.Equal(t, "220", jsonResult.Intervals[0].TotalValuePooled) // from initial values
	require.Equal(t, "280", jsonResult.Intervals[1].TotalValuePooled)
	require.Equal(t, "380", jsonResult.Intervals[2].TotalValuePooled)
	require.Equal(t, "380", jsonResult.Intervals[3].TotalValuePooled) // gapfill
	require.Equal(t, "10", jsonResult.Intervals[3].RunePriceUSD)      // initial USD price
	require.Equal(t, "356", jsonResult.Intervals[4].TotalValuePooled)
}

func TestTVLHistoryBondsE2E(t *testing.T) {
	// Run the test with bonds enabled to test the whole logic
	api.ShowBonds = true

	testdb.InitTest(t)

	insertBondEvent := func(t *testing.T, event_type string, e8 int64, block_timestamp string) {
		testdb.InsertBondEvent(t, testdb.FakeBond{
			BondType:       event_type,
			E8:             e8,
			BlockTimestamp: block_timestamp,
		})
	}

	// This will be skipped because we query 01-09 to 01-14
	insertBondEvent(t, "bond_paid", 1000, "2020-01-14 12:00:00")

	// This will be the initial value
	insertBondEvent(t, "bond_paid", 100, "2020-01-05 12:00:00")

	insertBondEvent(t, "bond_cost", 20, "2020-01-10 12:00:00")
	insertBondEvent(t, "bond_reward", 10, "2020-01-10 14:00:00")

	insertBondEvent(t, "bond_returned", 50, "2020-01-12 09:00:00")
	insertBondEvent(t, "bond_paid", 100, "2020-01-12 16:00:00")

	db.RefreshAggregatesForTests()

	from := db.StrToSec("2020-01-09 00:00:00")
	to := db.StrToSec("2020-01-13 00:00:00")

	body := testdb.CallJSON(t, fmt.Sprintf(
		"http://localhost:8080/v2/history/tvl?interval=day&from=%d&to=%d", from, to))

	var jsonResult oapigen.TVLHistoryResponse
	testdb.MustUnmarshal(t, body, &jsonResult)

	require.Equal(t, oapigen.TVLHistoryItem{
		StartTime:        epochStr("2020-01-09 00:00:00"),
		EndTime:          epochStr("2020-01-13 00:00:00"),
		TotalValuePooled: "0",
		TotalValueBonded: stringp("140"),
		TotalValueLocked: stringp("140"),
		RunePriceUSD:     "NaN",
		PoolsDepth:       []oapigen.DepthHistoryItemPool{},
	}, jsonResult.Meta)
	require.Equal(t, 4, len(jsonResult.Intervals))
	require.Equal(t, epochStr("2020-01-09 00:00:00"), jsonResult.Intervals[0].StartTime)
	require.Equal(t, epochStr("2020-01-10 00:00:00"), jsonResult.Intervals[0].EndTime)
	require.Equal(t, epochStr("2020-01-13 00:00:00"), jsonResult.Intervals[3].EndTime)

	require.Equal(t, stringp("100"), jsonResult.Intervals[0].TotalValueBonded) // from initial values
	require.Equal(t, stringp("90"), jsonResult.Intervals[1].TotalValueBonded)
	require.Equal(t, stringp("90"), jsonResult.Intervals[2].TotalValueBonded) // gapfill
	require.Equal(t, stringp("140"), jsonResult.Intervals[3].TotalValueBonded)
}
