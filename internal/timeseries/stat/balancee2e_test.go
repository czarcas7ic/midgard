package stat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

const BALANCE_URL = "http://localhost:8080/v2/balance/"

type expectation struct {
	slug string
	json string
}

func TestUnknownKey(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"testAddr1?badkey=123", "badkey")
}

func TestBadTsE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?timestamp=xxx", "error parsing timestamp xxx")
}

func TestBadHeightE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=xxx", "error parsing height xxx")
}

func TestTooManyParamsE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=1&timestamp="+ts("2000-01-01 00:00:00"), "only one of height or timestamp can be specified, not both")
}

func TestNoDataE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	testdb.InitTest(t)
	db.LastAggregatedBlock.Set(db.FirstBlock.Get().Height, db.FirstBlock.Get().Timestamp)

	timestamp := db.FirstBlock.Get().Timestamp.ToSecond().ToI()

	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=0", "no data for height 0, available height range is [1,1]")
	testdb.CallFail(t, BALANCE_URL+"thorAddr1?height=2", "no data for height 2, available height range is [1,1]")
	testdb.CallFail(t, BALANCE_URL+"thorAddr1?timestamp="+util.IntStr(timestamp-1), "no data for timestamp 946684799000000000, timestamp range is [946684800000000000,946684800000000000]")
	testdb.CallFail(t, BALANCE_URL+"thorAddr1?timestamp="+util.IntStr(timestamp+1), "no data for timestamp 946684801000000000, timestamp range is [946684800000000000,946684800000000000]")
}

func TestZeroBalanceE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:00",
		testdb.Transfer{
			FromAddr:    "thorAddr1",
			ToAddr:      "thorAddr2",
			AssetAmount: "1 THOR.RUNE",
		},
	)

	blocks.NewBlock(t, "2000-01-01 00:00:01",
		testdb.Transfer{
			FromAddr:    "thorAddr2",
			ToAddr:      "thorAddr1",
			AssetAmount: "1 THOR.RUNE",
		},
	)

	checkExpected(t, []expectation{
		{"thorAddr1", `{"height": "2", "timestamp": "946684801000000000", "coins": [{"amount":"0","asset":"THOR.RUNE"}]}`},
		{"thorAddr2", `{"height": "2", "timestamp": "946684801000000000", "coins": [{"amount":"0","asset":"THOR.RUNE"}]}`},
	})
}

func TestBalanceE2E(t *testing.T) {
	config.Global.EventRecorder.OnTransferEnabled = true

	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2000-01-01 00:00:01",
		testdb.Transfer{
			FromAddr:    "thorAddr1",
			ToAddr:      "thorAddr2",
			AssetAmount: "1 THOR.RUNE",
		},
	)
	blocks.NewBlock(t, "2000-01-01 00:00:02",
		testdb.Transfer{
			FromAddr:    "thorAddr1",
			ToAddr:      "thorAddr2",
			AssetAmount: "20 THOR.RUNE",
		},
	)
	blocks.NewBlock(t, "2000-01-01 00:00:03",
		testdb.Transfer{
			FromAddr:    "thorAddr2",
			ToAddr:      "thorAddr1",
			AssetAmount: "3 THOR.RUNE",
		},
	)
	blocks.NewBlock(t, "2000-01-01 00:00:04",
		testdb.Transfer{
			FromAddr:    "thorAddr2",
			ToAddr:      "thorAddr1",
			AssetAmount: "300 BTC/BTC",
		},
	)

	db.LastAggregatedBlock.Set(db.LastCommittedBlock.Get().Height, db.LastCommittedBlock.Get().Timestamp)

	checkExpected(t, []expectation{
		{"thorAddr0",
			// TODO(freki) discuss adding address to the response
			`{
				"height": "4",
				"timestamp": "946684804000000000",
				"coins": []
			}`,
		},
		{"thorAddr1",
			`{
				"height": "4",
				"timestamp": "946684804000000000",
				"coins": [
					{"amount":"300", "asset":"BTC/BTC"},
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?height=3",
			`{
				"height": "3",
				"timestamp": "946684803000000000",
				"coins": [
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?height=4",
			`{
				"height": "4",
				"timestamp": "946684804000000000",
				"coins": [
					{"amount":"300", "asset":"BTC/BTC"},
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?timestamp=" + ts("2000-01-01 00:00:01"),
			`{
				"height": "1",
				"timestamp": "946684801000000000",
				"coins": [
					{"amount":"-1", "asset":"THOR.RUNE"}
				]
			}`,
		},
		{"thorAddr1?timestamp=" + ts("2000-01-01 00:00:04"),
			`{
				"height": "4",
				"timestamp": "946684804000000000",
				"coins": [
					{"amount":"300", "asset":"BTC/BTC"},
					{"amount":"-18", "asset":"THOR.RUNE"}
				]
			}`,
		},
	})

}

func checkExpected(t *testing.T, expectations []expectation) {
	for i, e := range expectations {
		actualJson := testdb.CallJSON(t, BALANCE_URL+e.slug)
		var expectedBalance oapigen.BalanceResponse
		testdb.MustUnmarshal(t, []byte(e.json), &expectedBalance)
		var actualBalance oapigen.BalanceResponse
		testdb.MustUnmarshal(t, actualJson, &actualBalance)
		require.Equal(t, expectedBalance, actualBalance, fmt.Sprintf("expectation %d failed", i))
	}
}

func ts(date string) string {
	return fmt.Sprintf("%d", db.StrToSec(date))
}
