package db_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

func bucketPass(t *testing.T, getParams string) (ret []string) {
	testdb.SetupTestDB(t)
	testdb.MustExec(t, "DELETE FROM swap_events")

	body := testdb.CallJSON(t, "http://localhost:8080/v2/history/swaps?"+getParams)

	var swapHistory oapigen.SwapHistoryResponse
	testdb.MustUnmarshal(t, body, &swapHistory)

	require.NotEmpty(t, swapHistory.Intervals)
	require.Equal(t, swapHistory.Meta.StartTime, swapHistory.Intervals[0].StartTime)
	require.Equal(t,
		swapHistory.Meta.EndTime,
		swapHistory.Intervals[len(swapHistory.Intervals)-1].EndTime)

	for _, interval := range swapHistory.Intervals {
		i, err := strconv.Atoi(interval.StartTime)
		require.Nil(t, err)
		ret = append(ret, testdb.SecToString(db.Second(i)))
	}
	return
}

func bucketFail(t *testing.T, getParams string, msg ...string) {
	testdb.SetupTestDB(t)
	testdb.MustExec(t, "DELETE FROM swap_events")
	testdb.CallFail(t, "http://localhost:8080/v2/history/swaps?"+getParams, msg...)
}

func TestYearExact(t *testing.T) {
	db.SetFirstBlockTimestamp(testdb.StrToNano("2010-01-01 00:00:00"))
	db.SetLastBlockTimestamp(testdb.StrToNano("2030-01-01 00:00:00"))
	t0 := testdb.StrToSec("2015-01-01 00:00:00")
	t1 := testdb.StrToSec("2018-01-01 00:00:00")
	starts := bucketPass(t, fmt.Sprintf("interval=year&from=%d&to=%d", t0, t1))
	require.Equal(t, []string{
		"2015-01-01 00:00:00",
		"2016-01-01 00:00:00",
		"2017-01-01 00:00:00",
	}, starts)
}

func TestYearInexact(t *testing.T) {
	db.SetFirstBlockTimestamp(testdb.StrToNano("2010-01-01 00:00:00"))
	db.SetLastBlockTimestamp(testdb.StrToNano("2030-01-01 00:00:00"))
	t0 := testdb.StrToSec("2015-06-01 00:00:00")
	t1 := testdb.StrToSec("2018-06-01 00:00:00")
	starts := bucketPass(t, fmt.Sprintf("interval=year&from=%d&to=%d", t0, t1))
	require.Equal(t, []string{
		"2015-01-01 00:00:00",
		"2016-01-01 00:00:00",
		"2017-01-01 00:00:00",
		"2018-01-01 00:00:00",
	}, starts)
}

func TestYearEmptyFail(t *testing.T) {
	t0 := testdb.StrToSec("2015-01-01 00:00:00")
	t1 := testdb.StrToSec("2015-01-01 00:00:00")
	bucketFail(t, fmt.Sprintf("interval=year&from=%d&to=%d", t0, t1),
		"no interval requested")
}

func intStrToTimeStr(t *testing.T, secStr string) string {
	i, err := strconv.Atoi(secStr)
	require.Nil(t, err)
	return testdb.SecToString(db.Second(i))
}

func TestIntervalMissing(t *testing.T) {
	testdb.SetupTestDB(t)
	testdb.MustExec(t, "DELETE FROM swap_events")

	// Insert one before and on in the interval.
	testdb.InsertSwapEvent(t, testdb.FakeSwap{
		Pool: "BNB.BTCB-1DE", FromAsset: "BNB.BTCB-1DE",
		BlockTimestamp: "2020-12-10 00:01:00",
	})
	testdb.InsertSwapEvent(t, testdb.FakeSwap{
		Pool: "BNB.BTCB-1DE", FromAsset: "BNB.BTCB-1DE",
		BlockTimestamp: "2020-12-10 02:00:00",
	})

	t0 := testdb.StrToSec("2020-12-10 01:02:03")
	t1 := testdb.StrToSec("2020-12-20 01:02:03")
	body := testdb.CallJSON(t, fmt.Sprintf("http://localhost:8080/v2/history/swaps?from=%d&to=%d", t0, t1))

	var swapHistory oapigen.SwapHistoryResponse
	testdb.MustUnmarshal(t, body, &swapHistory)

	require.Equal(t, "2020-12-10 01:02:03", intStrToTimeStr(t, swapHistory.Meta.StartTime))
	require.Equal(t, "2020-12-20 01:02:03", intStrToTimeStr(t, swapHistory.Meta.EndTime))
	require.Equal(t, "1", swapHistory.Meta.TotalCount)
}

func TestBadIntervalName(t *testing.T) {
	t0 := testdb.StrToSec("2015-01-01 00:00:00")
	t1 := testdb.StrToSec("2018-01-01 00:00:00")
	bucketFail(t, fmt.Sprintf("interval=century&from=%d&to=%d", t0, t1),
		"invalid", "century")
}

func TestTooWideFromTo(t *testing.T) {
	t0 := testdb.StrToSec("2015-01-01 00:00:00")
	t1 := testdb.StrToSec("2018-01-01 00:00:00")
	bucketFail(t, fmt.Sprintf("interval=5min&from=%d&to=%d", t0, t1),
		"too wide range")
}

func TestCountTo(t *testing.T) {
	t1 := testdb.StrToSec("2018-06-01 00:00:00")
	count := 3
	starts := bucketPass(t, fmt.Sprintf("interval=year&to=%d&count=%d", t1, count))
	require.Equal(t, []string{
		"2016-01-01 00:00:00",
		"2017-01-01 00:00:00",
		"2018-01-01 00:00:00",
	}, starts)
}

func TestCountManyMonthsTo(t *testing.T) {
	t1 := testdb.StrToSec("2020-12-02 00:00:00")
	count := 12 * 8 // 8 years
	starts := bucketPass(t, fmt.Sprintf("interval=month&to=%d&count=%d", t1, count))
	require.Len(t, starts, 12*8)
	require.Equal(t, "2020-12-01 00:00:00", starts[len(starts)-1])
	require.Equal(t, "2013-01-01 00:00:00", starts[0])
}

func TestCountManyMonthsFrom(t *testing.T) {
	t0 := testdb.StrToSec("2013-01-02 00:00:00")
	count := 12 * 8 // 8 years
	starts := bucketPass(t, fmt.Sprintf("interval=month&from=%d&count=%d", t0, count))
	require.Len(t, starts, 12*8)
	require.Equal(t, "2020-12-01 00:00:00", starts[len(starts)-1])
	require.Equal(t, "2013-01-01 00:00:00", starts[0])
}

func TestCount1From(t *testing.T) {
	t0 := testdb.StrToSec("2020-01-01 00:00:00")
	count := 1
	starts := bucketPass(t, fmt.Sprintf("interval=year&from=%d&count=%d", t0, count))
	require.Equal(t, []string{
		"2020-01-01 00:00:00",
	}, starts)
}

func TestCount1To(t *testing.T) {
	t1 := testdb.StrToSec("2020-01-01 00:00:00")
	count := 1
	starts := bucketPass(t, fmt.Sprintf("interval=year&to=%d&count=%d", t1, count))
	require.Equal(t, []string{
		"2019-01-01 00:00:00",
	}, starts)
}

func TestBeforeFirstBlock(t *testing.T) {
	db.SetFirstBlockTimestamp(testdb.StrToNano("2020-01-01 00:00:00"))
	db.SetLastBlockTimestamp(testdb.StrToNano("2030-01-01 00:00:00"))
	t1 := testdb.StrToSec("2018-06-01 00:00:00")
	count := 3
	starts := bucketPass(t, fmt.Sprintf("interval=year&to=%d&count=%d", t1, count))
	require.Equal(t, []string{
		"2018-01-01 00:00:00",
	}, starts)
}

func TestAfterLastBlock(t *testing.T) {
	db.SetFirstBlockTimestamp(testdb.StrToNano("2000-01-01 00:00:00"))
	db.SetLastBlockTimestamp(testdb.StrToNano("2010-01-01 00:00:00"))
	t1 := testdb.StrToSec("2015-06-01 00:00:00")
	count := 3
	starts := bucketPass(t, fmt.Sprintf("interval=year&from=%d&count=%d", t1, count))
	require.Equal(t, []string{
		"2015-01-01 00:00:00",
	}, starts)
}

func TestLoadFirstBlockFromDB(t *testing.T) {
	testdb.SetupTestDB(t)
	testdb.MustExec(t, "DELETE FROM block_log")
	testdb.InsertBlockLog(t, 1, "2015-06-01 00:00:00")
	db.LoadFirstBlockFromDB(context.Background())

	db.SetLastBlockTimestamp(testdb.StrToNano("2018-06-01 00:00:00"))
	t1 := testdb.StrToSec("2020-06-01 00:00:00")
	count := 10
	starts := bucketPass(t, fmt.Sprintf("interval=year&to=%d&count=%d", t1, count))
	require.Equal(t, []string{
		"2015-01-01 00:00:00",
		"2016-01-01 00:00:00",
		"2017-01-01 00:00:00",
		"2018-01-01 00:00:00",
	}, starts)
}

func TestBucketFailures(t *testing.T) {
	bucketFail(t, "interval=year&count=10&from=1&to=100", "specify max 2")
	bucketFail(t, "interval=year&count=500&to=100", "count out of range")
	bucketFail(t, "count=123&from=1&to=100", "count", "provided", "no interval")
}
