package testdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/api"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	testDbPort, err := strconv.Atoi(getEnvVariable("DB_PORT", "5433"))
	if err != nil {
		log.Fatal().Err(err).Msg("DB_PORT must be a number")
	}

	config.Global.TimeScale = config.TimeScale{
		Host:     getEnvVariable("DB_HOST", "localhost"),
		Port:     testDbPort,
		Database: "midgard",
		UserName: "midgard",
		Password: "password",
		Sslmode:  "disable",
	}

	dbinit.Setup()

	// TODO(huginn): create tests that test the two kind of inserters separately
	if getEnvVariable("TEST_IMMEDIATE_INSERTER", "") == "1" {
		db.Inserter = db.TheImmediateInserter
	}
}

func SetupTestDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
}

func DeleteTables(t *testing.T) {
	MustExec(t, "DELETE FROM block_log")
	MustExec(t, "DELETE FROM block_pool_depths")
	MustExec(t, "DELETE FROM stake_events")
	MustExec(t, "DELETE FROM pending_liquidity_events")
	MustExec(t, "DELETE FROM withdraw_events")
	MustExec(t, "DELETE FROM switch_events")
	MustExec(t, "DELETE FROM swap_events")
	MustExec(t, "DELETE FROM rewards_events")
	MustExec(t, "DELETE FROM rewards_event_entries")
	MustExec(t, "DELETE FROM bond_events")
	MustExec(t, "DELETE FROM pool_events")
	MustExec(t, "DELETE FROM update_node_account_status_events")
	MustExec(t, "DELETE FROM active_vault_events")
	MustExec(t, "DELETE FROM set_mimir_events")
	MustExec(t, "DELETE FROM thorname_change_events")
	MustExec(t, "DELETE FROM outbound_events")
	MustExec(t, "DELETE FROM fee_events")
	MustExec(t, "DELETE FROM add_events")
	MustExec(t, "DELETE FROM refund_events")
	MustExec(t, "DELETE FROM transfer_events")

	clearAggregates(t)
}

func clearAggregates(t *testing.T) {
	MustExec(t, "UPDATE midgard_agg.watermarks SET watermark = 0")
	for _, table := range db.WatermarkedMaterializedTables() {
		MustExec(t, "DELETE FROM "+table)
	}
	MustExec(t, "DELETE FROM midgard_agg.actions")
	MustExec(t, "DELETE FROM midgard_agg.current_balances")
	MustExec(t, "DELETE FROM midgard_agg.balances")
	MustExec(t, "DELETE FROM midgard_agg.members_log")
	MustExec(t, "DELETE FROM midgard_agg.members")
	MustExec(t, "DELETE FROM midgard_agg.members_count")
}

func InitTest(t *testing.T) {
	HideTestLogs(t)
	db.ResetGlobalVarsForTests()
	SetupTestDB(t)
	db.FirstBlock.Set(1, StrToNano("2000-01-01 00:00:00"))
	timeseries.SetLastTimeForTest(StrToNano("2030-01-01 00:00:00").ToSecond())

	DeleteTables(t)
}

// Show test logs only on failure
func HideTestLogs(t *testing.T) {
	midlog.SetExitFunctionForTest(t.FailNow)
	b := bytes.Buffer{}
	midlog.SetGlobalOutput(&b, true)

	t.Cleanup(func() {
		if t.Failed() {
			_, err := io.Copy(os.Stdout, &b)
			if err != nil {
				fmt.Println("Error writing test output")
			}
		}
	})
}

// Use this when full blocks are added.
func InitTestBlocks(t *testing.T) *blockCreator {
	// TODO(muninn): create a RegisterGlobalResetForTest(func()) and register the global variables
	//   in an init function or something similar
	HideTestLogs(t)
	db.ResetGlobalVarsForTests()
	record.ResetRecorderForTest()
	timeseries.ResetLatestStateForTest()
	timeseries.ResetDepthManagerForTest()
	SetupTestDB(t)
	DeleteTables(t)
	ret := blockCreator{}
	return &ret
}

func DeclarePools(pools ...string) {
	depths := []timeseries.Depth{}
	for _, pool := range pools {
		depths = append(depths, timeseries.Depth{Pool: pool, AssetDepth: 1, RuneDepth: 1, SynthDepth: 0})
	}
	timeseries.SetDepthsForTest(depths)
}

func MustUnmarshal(t *testing.T, data []byte, v interface{}) {
	err := json.Unmarshal(data, v)
	if err != nil {
		require.FailNow(t, "Failed to unmarshal", "Data: %v \nError: %s", string(data), err.Error())
	}
}

func StrToNano(s string) db.Nano {
	return db.StrToSec(s).ToNano()
}

func SecToString(s db.Second) string {
	return time.Unix(s.ToI(), 0).UTC().Format("2006-01-02 15:04:05")
}

func nanoWithDefault(fakeTimestamp string) db.Nano {
	var timestamp db.Second

	if fakeTimestamp == "" {
		timestamp = db.StrToSec("2000-01-01 00:00:00")
	} else {
		timestamp = db.StrToSec(fakeTimestamp)
	}

	return timestamp.ToNano()
}

func RoughlyEqual(t *testing.T, expected float64, actual string) {
	actualFloat, err := strconv.ParseFloat(actual, 64)
	require.Nil(t, err, "not float: %s", actual)
	delta := expected * 0.0001
	if delta < 0 {
		delta *= -1
	}
	require.InDelta(t, expected, actualFloat, delta)
}

// Execute a query on the database.
func MustExec(t *testing.T, query string, args ...interface{}) {
	_, err := db.TheDB.Exec(query, args...)
	if err != nil {
		t.Fatal("db query failed. Did you `docker-compose up -d pg`? ", err, "query: ", query, "args: ", args)
	}
}

var apiOnce sync.Once

func initApi() {
	apiOnce.Do(func() {
		api.InitHandler("")
	})
}

// Make an HTTP call to the /v1 api, return the body which can be parsed as a JSON.
func CallJSON(t *testing.T, url string) (body []byte) {
	initApi()
	api.GlobalCacheStore.RefreshAll(context.Background())
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	api.Handler.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal("error reading body:", res.Body)
	}

	if res.Status != "200 OK" {
		t.Fatal("Bad response status: ", res.Status, "\n URL: ", url, "\n Body: ", string(body))
	}

	return body
}

func JSONFailGeneral(t *testing.T, url string) {
	initApi()
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	api.Handler.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()
	if res.Status == "200 OK" {
		t.Fatal("Expected to fail, but didn't:", url)
	}
}

func CallFail(t *testing.T, url string, msg ...string) {
	initApi()
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	api.Handler.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()
	require.Equal(t, res.StatusCode < 300, false,
		"Expected to fail, but didn't:", url)
	bodyb, err := ioutil.ReadAll(res.Body)
	body := strings.ToLower(string(bodyb))
	require.Nil(t, err)
	for _, m := range msg {
		require.Contains(t, body, strings.ToLower(m))
	}
}

type FakeBond struct {
	Tx             string
	Chain          string
	FromAddr       string
	ToAddr         string
	Asset          string
	AssetE8        int64 // Asset quantity times 100 M
	Memo           string
	BondType       string
	E8             int64
	BlockTimestamp string
}

func InsertBondEvent(t *testing.T, fake FakeBond) {
	const insertq = `
		INSERT INTO bond_events
			(tx, chain, from_addr, to_addr, asset, asset_e8, memo, bond_type, e8,
				event_id, block_timestamp)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''), $8, $9, 0, $10)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)

	MustExec(t, insertq,
		fake.Tx, fake.Chain, fake.FromAddr, fake.ToAddr, fake.Asset,
		fake.AssetE8, fake.Memo, fake.BondType,
		fake.E8, timestamp)
}

type FakeStake struct {
	Pool           string
	BlockTimestamp string
	AssetE8        int64
	AssetInRuneE8  int64
	RuneE8         int64
	StakeUnits     int64
	RuneAddress    string
	AssetAddress   string
	AssetTx        string
	RuneTx         string
}

func InsertStakeEvent(t *testing.T, fake FakeStake) {
	const insertq = `
		INSERT INTO stake_events
			(pool, asset_tx, asset_chain, asset_addr, asset_E8, _asset_in_rune_E8,
				rune_tx, rune_addr, rune_E8, stake_units, event_id, block_timestamp)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, 0, $11)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)

	MustExec(t, insertq,
		fake.Pool, fake.AssetTx, "chain", fake.AssetAddress, fake.AssetE8, fake.AssetInRuneE8,
		fake.RuneTx, fake.RuneAddress, fake.RuneE8,
		fake.StakeUnits, timestamp)
}

type FakeWithdraw struct {
	Asset               string
	FromAddr            string
	ToAddr              string
	BlockTimestamp      string
	StakeUnits          int64
	Pool                string
	EmitAssetE8         int64
	EmitAssetInRuneE8   int64
	EmitRuneE8          int64
	ImpLossProtectionE8 int64
}

func InsertWithdrawEvent(t *testing.T, fake FakeWithdraw) {
	const insertq = `
		INSERT INTO withdraw_events
			(tx, chain, from_addr, to_addr, asset, asset_E8,
				emit_asset_E8, _emit_asset_in_rune_E8, emit_rune_E8,
				memo, pool, stake_units, basis_points, asymmetry, imp_loss_protection_E8,
				event_id, block_timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, 0, $16)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)
	MustExec(t, insertq,
		"tx", "chain", fake.FromAddr, fake.ToAddr,
		fake.Asset, 1, fake.EmitAssetE8, fake.EmitAssetInRuneE8, fake.EmitRuneE8, "memo",
		fake.Pool, fake.StakeUnits, 3, 4,
		fake.ImpLossProtectionE8, timestamp)
}

// TODO(muninn): Remove, migrate remaining tests to FakeBlocks.
type FakeSwap struct {
	Tx             string
	Pool           string
	FromAsset      string
	FromE8         int64
	FromAddr       string
	ToE8           int64
	ToAddr         string
	LiqFeeInRuneE8 int64
	LiqFeeE8       int64
	SwapSlipBP     int64
	ToE8Min        int64
	BlockTimestamp string
}

func InsertSwapEvent(t *testing.T, fake FakeSwap) {
	const insertq = `INSERT INTO swap_events ` +
		`(tx, chain, from_addr, to_addr, from_asset, from_E8, to_asset, to_E8, memo, pool, to_E8_min,
			swap_slip_BP, liq_fee_E8, liq_fee_in_rune_E8, _direction, event_id, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, 0, $16)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)

	// Hardcoded (probably incorrect) _direction. Use fake blocks and Full E2E for proper direction.
	MustExec(t, insertq,
		fake.Tx, "chain", fake.FromAddr, fake.ToAddr, fake.FromAsset, fake.FromE8, "to_asset", fake.ToE8,
		"memo", fake.Pool, fake.ToE8Min, fake.SwapSlipBP, fake.LiqFeeE8, fake.LiqFeeInRuneE8,
		db.RuneToAsset, timestamp)
}

type FakeSwitch struct {
	FromAddr       string
	ToAddr         string
	BurnAsset      string
	BurnE8         int64
	BlockTimestamp string
}

func InsertSwitchEvent(t *testing.T, fake FakeSwitch) {
	const insertq = `INSERT INTO switch_events ` +
		`(from_addr, to_addr, burn_asset, burn_e8, event_id, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4, 0, $5)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)
	MustExec(t, insertq,
		fake.FromAddr, fake.ToAddr, fake.BurnAsset, fake.BurnE8, timestamp)
}

func InsertRewardsEvent(t *testing.T, bondE8 int64, fakeTimestamp string) {
	const insertq = `INSERT INTO rewards_events ` +
		`(bond_e8, event_id, block_timestamp) ` +
		`VALUES ($1, 0, $2)`

	timestamp := nanoWithDefault(fakeTimestamp)
	MustExec(t, insertq, bondE8, timestamp)
}

func InsertRewardsEventEntry(t *testing.T, bondE8 int64, pool, fakeTimestamp string) {
	const insertq = `INSERT INTO rewards_event_entries ` +
		`(rune_e8, saver_e8, pool, event_id, block_timestamp) ` +
		`VALUES ($1, 0, $2, 0, $3)`

	timestamp := nanoWithDefault(fakeTimestamp)
	MustExec(t, insertq, bondE8, pool, timestamp)
}

func InsertBlockLog(t *testing.T, height int64, fakeTimestamp string) (hash string) {
	const insertq = `INSERT INTO block_log ` +
		`(height, timestamp, hash) ` +
		`VALUES ($1, $2, $3)`

	timestamp := nanoWithDefault(fakeTimestamp)
	hash = fmt.Sprintf("%d-%d", height, timestamp)
	MustExec(t, insertq, height, timestamp, hash)
	return
}

func InsertPoolEvents(t *testing.T, pool, status string) {
	const insertq = `INSERT INTO pool_events` +
		`(asset, status, event_id, block_timestamp) ` +
		`VALUES ($1, $2, 0, 1)`

	MustExec(t, insertq, pool, status)
}

func InsertBlockPoolDepth(t *testing.T, pool string, assetE8, runeE8 int64, blockTimestamp string) {
	const insertq = `INSERT INTO block_pool_depths ` +
		`(pool, asset_e8, rune_e8, synth_e8, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4, $5)`

	timestamp := nanoWithDefault(blockTimestamp)
	MustExec(t, insertq, pool, assetE8, runeE8, 0, timestamp)
}

type FakeNodeStatus struct {
	NodeAddr string
	Former   string
	Current  string
}

func InsertUpdateNodeAccountStatusEvent(t *testing.T, fake FakeNodeStatus, blockTimestamp string) {
	const insertq = `INSERT INTO update_node_account_status_events ` +
		`(node_addr, former, current, event_id, block_timestamp) ` +
		`VALUES ($1, $2, $3, 0, $4)`

	timestamp := nanoWithDefault(blockTimestamp)
	MustExec(t, insertq, fake.NodeAddr, fake.Former, fake.Current, timestamp)
}

func getEnvVariable(key, def string) string {
	value := os.Getenv(key)

	if value == "" {
		value = def
	}

	return value
}

func SetThornodeConstant(t *testing.T, name string, value int64, timestamp string) {
	insertMimirEvent(t, name, value, timestamp)
}

func insertMimirEvent(t *testing.T, key string, value int64, blockTimestamp string) {
	const insertq = `INSERT INTO set_mimir_events ` +
		`(key, value, event_id, block_timestamp) ` +
		`VALUES ($1, $2, 0, $3)`

	timestamp := nanoWithDefault(blockTimestamp)
	MustExec(t, insertq, key, strconv.FormatInt(value, 10), timestamp)
}
