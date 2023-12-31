package testdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

	"gitlab.com/thorchain/midgard/internal/api"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/timeseries"
)

var (
	testDBQuery func(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	testDBExec  func(query string, args ...interface{}) (sql.Result, error)
)

func init() {
	testDbPort := getEnvVariable("DB_PORT", "5433")
	testHost := getEnvVariable("DB_HOST", "localhost")
	testUser := getEnvVariable("DB_USER", "midgard")
	testDb := getEnvVariable("DB_NAME", "midgard")

	dbObj, err := sql.Open("pgx", fmt.Sprintf("user=%s dbname=%s sslmode=disable password=password host=%s port=%s", testUser, testDb, testHost, testDbPort))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}

	testDBQuery = dbObj.QueryContext
	testDBExec = dbObj.Exec

	db.UpdateDDLsIfNeeded(dbObj)
}

func SetupTestDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	db.Exec = testDBExec
	db.Query = testDBQuery
}

func DeleteTables(t *testing.T) {
	MustExec(t, "DELETE FROM block_log")
	MustExec(t, "DELETE FROM block_pool_depths")
	MustExec(t, "DELETE FROM stake_events")
	MustExec(t, "DELETE FROM pending_liquidity_events")
	MustExec(t, "DELETE FROM unstake_events")
	MustExec(t, "DELETE FROM switch_events")
	MustExec(t, "DELETE FROM swap_events")
	MustExec(t, "DELETE FROM fee_events")
	MustExec(t, "DELETE FROM rewards_events")
	MustExec(t, "DELETE FROM rewards_event_entries")
	MustExec(t, "DELETE FROM bond_events")
	MustExec(t, "DELETE FROM pool_events")
	MustExec(t, "DELETE FROM update_node_account_status_events")
	MustExec(t, "DELETE FROM active_vault_events")
	MustExec(t, "DELETE FROM set_mimir_events")
}

func InitTest(t *testing.T) {
	SetupTestDB(t)
	db.SetFirstBlockTimestamp(StrToNano("2000-01-01 00:00:00"))
	timeseries.SetLastTimeForTest(StrToNano("2030-01-01 00:00:00").ToSecond())

	DeleteTables(t)
}

// Use this when full blocks are added.
func InitTestBlocks(t *testing.T) *blockCreator {
	record.ResetRecorderForTest()
	SetupTestDB(t)
	DeleteTables(t)
	ret := blockCreator{}
	return &ret
}

func DeclarePools(pools ...string) {
	depths := []timeseries.Depth{}
	for _, pool := range pools {
		depths = append(depths, timeseries.Depth{Pool: pool, AssetDepth: 1, RuneDepth: 1})
	}
	timeseries.SetDepthsForTest(depths)
}

func MustUnmarshal(t *testing.T, data []byte, v interface{}) {
	err := json.Unmarshal(data, v)
	if err != nil {
		t.FailNow()
	}
}

func StrToSec(s string) db.Second {
	const format = "2006-01-02 15:04:05"
	t, err := time.Parse(format, s)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to parse date")
	}
	return db.TimeToSecond(t)
}

func StrToNano(s string) db.Nano {
	return StrToSec(s).ToNano()
}

func SecToString(s db.Second) string {
	return time.Unix(s.ToI(), 0).UTC().Format("2006-01-02 15:04:05")
}

func nanoWithDefault(fakeTimestamp string) db.Nano {
	var timestamp db.Second

	if fakeTimestamp == "" {
		timestamp = StrToSec("2000-01-01 00:00:00")
	} else {
		timestamp = StrToSec(fakeTimestamp)
	}

	return timestamp.ToNano()
}

// Execute a query on the database.
func MustExec(t *testing.T, query string, args ...interface{}) {
	_, err := db.Exec(query, args...)
	if err != nil {
		t.Fatal("db query failed. Did you `docker-compose up -d pg`? ", err, "query: ", query, "args: ", args)
	}
}

var apiOnce sync.Once

func initApi() {
	apiOnce.Do(func() {
		api.InitHandler("", []string{})
	})
}

// Make an HTTP call to the /v1 api, return the body which can be parsed as a JSON.
func CallJSON(t *testing.T, url string) (body []byte) {
	initApi()
	api.CacheLogger = zerolog.Nop()
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
		t.Fatal("Bad response status:", res.Status, ". Body: ", string(body))
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
	require.Equal(t, http.StatusBadRequest, res.StatusCode,
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
	const insertq = `INSERT INTO bond_events ` +
		`(tx, chain, from_addr, to_addr, asset, asset_E8, memo, bond_type, E8, block_timestamp) ` +
		`VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''), $8, $9, $10)`

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
	RuneE8         int64
	StakeUnits     int64
	RuneAddress    string
	AssetAddress   string
	AssetTx        string
	RuneTx         string
}

func InsertStakeEvent(t *testing.T, fake FakeStake) {
	const insertq = `INSERT INTO stake_events ` +
		`(pool, asset_tx, asset_chain, asset_addr, asset_E8, rune_tx, rune_addr, rune_E8, stake_units, block_timestamp) ` +
		`VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''), $8, $9, $10)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)

	MustExec(t, insertq,
		fake.Pool, fake.AssetTx, "chain", fake.AssetAddress, fake.AssetE8,
		fake.RuneTx, fake.RuneAddress, fake.RuneE8,
		fake.StakeUnits, timestamp)
}

type FakeUnstake struct {
	Asset               string
	FromAddr            string
	ToAddr              string
	BlockTimestamp      string
	StakeUnits          int64
	Pool                string
	EmitAssetE8         int64
	EmitRuneE8          int64
	ImpLossProtectionE8 int64
}

func InsertUnstakeEvent(t *testing.T, fake FakeUnstake) {
	const insertq = `
		INSERT INTO unstake_events
			(tx, chain, from_addr, to_addr, asset, asset_E8, emit_asset_E8, emit_rune_E8,
				memo, pool, stake_units, basis_points, asymmetry, imp_loss_protection_E8,
				block_timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)
	MustExec(t, insertq,
		"tx", "chain", fake.FromAddr, fake.ToAddr,
		fake.Asset, 1, fake.EmitAssetE8, fake.EmitRuneE8, "memo",
		fake.Pool, fake.StakeUnits, 3, 4,
		fake.ImpLossProtectionE8, timestamp)
}

type FakeFee struct {
	Tx             string
	Asset          string
	AssetE8        int64
	PoolDeduct     int64
	BlockTimestamp string
}

func InsertFeeEvent(t *testing.T, fake FakeFee) {
	const insertq = `INSERT INTO fee_events ` +
		`(tx, asset, asset_e8, pool_deduct, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4, $5)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)
	MustExec(t, insertq, fake.Tx, fake.Asset, fake.AssetE8, fake.PoolDeduct, timestamp)
}

type FakeSwap struct {
	Tx             string
	Pool           string
	FromAsset      string
	FromE8         int64
	FromAddr       string
	ToAsset        string
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
		`(tx, chain, from_addr, to_addr, from_asset, from_E8, to_asset, to_E8, memo, pool, to_E8_min, swap_slip_BP,
			liq_fee_E8, liq_fee_in_rune_E8, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)
	var toAsset string
	if fake.ToAsset != "" {
		toAsset = fake.ToAsset
	} else {
		toAsset = "to_asset"
	}
	MustExec(t, insertq,
		fake.Tx, "chain", fake.FromAddr, fake.ToAddr, fake.FromAsset, fake.FromE8, toAsset, fake.ToE8,
		"memo", fake.Pool, fake.ToE8Min, fake.SwapSlipBP, fake.LiqFeeE8, fake.LiqFeeInRuneE8, timestamp)
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
		`(from_addr, to_addr, burn_asset, burn_e8, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4, $5)`

	timestamp := nanoWithDefault(fake.BlockTimestamp)
	MustExec(t, insertq,
		fake.FromAddr, fake.ToAddr, fake.BurnAsset, fake.BurnE8, timestamp)
}

func InsertRewardsEvent(t *testing.T, bondE8 int64, fakeTimestamp string) {
	const insertq = `INSERT INTO rewards_events ` +
		`(bond_e8, block_timestamp) ` +
		`VALUES ($1, $2)`

	timestamp := nanoWithDefault(fakeTimestamp)
	MustExec(t, insertq, bondE8, timestamp)
}

func InsertRewardsEventEntry(t *testing.T, bondE8 int64, pool, fakeTimestamp string) {
	const insertq = `INSERT INTO rewards_event_entries ` +
		`(rune_e8, block_timestamp, pool) ` +
		`VALUES ($1, $2, $3)`

	timestamp := nanoWithDefault(fakeTimestamp)
	MustExec(t, insertq, bondE8, timestamp, pool)
}

func InsertBlockLog(t *testing.T, height int64, fakeTimestamp string) {
	const insertq = `INSERT INTO block_log ` +
		`(height, timestamp, hash) ` +
		`VALUES ($1, $2, $3)`

	timestamp := nanoWithDefault(fakeTimestamp)
	MustExec(t, insertq, height, timestamp, fmt.Sprintf("%d-%d", height, timestamp))
}

func InsertPoolEvents(t *testing.T, pool, status string) {
	const insertq = `INSERT INTO  pool_events` +
		`(asset, status, block_timestamp) ` +
		`VALUES ($1, $2, 1)`

	MustExec(t, insertq, pool, status)
}

func InsertBlockPoolDepth(t *testing.T, pool string, assetE8, runeE8 int64, blockTimestamp string) {
	const insertq = `INSERT INTO block_pool_depths ` +
		`(pool, asset_e8, rune_e8, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4)`

	timestamp := nanoWithDefault(blockTimestamp)
	MustExec(t, insertq, pool, assetE8, runeE8, timestamp)
}

type FakeNodeStatus struct {
	NodeAddr string
	Former   string
	Current  string
}

func InsertUpdateNodeAccountStatusEvent(t *testing.T, fake FakeNodeStatus, blockTimestamp string) {
	const insertq = `INSERT INTO update_node_account_status_events ` +
		`(node_addr, former, current, block_timestamp) ` +
		`VALUES ($1, $2, $3, $4)`

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

func InsertActiveVaultEvent(t *testing.T, address string, blockTimestamp string) {
	const insertq = `INSERT INTO active_vault_events ` +
		`(add_asgard_addr, block_timestamp) ` +
		`VALUES ($1, $2)`

	timestamp := nanoWithDefault(blockTimestamp)
	MustExec(t, insertq, address, timestamp)
}

type FakeThornodeConstants struct {
	EmissionCurve      int64
	BlocksPerYear      int64
	ChurnInterval      int64
	ChurnRetryInterval int64
	PoolCycle          int64
}

func SetThornodeConstants(t *testing.T, constants *FakeThornodeConstants, timestamp string) {
	insertMimirEvent(t, "EmissionCurve", constants.EmissionCurve, timestamp)
	insertMimirEvent(t, "BlocksPerYear", constants.BlocksPerYear, timestamp)
	insertMimirEvent(t, "ChurnInterval", constants.ChurnInterval, timestamp)
	insertMimirEvent(t, "ChurnRetryInterval", constants.ChurnRetryInterval, timestamp)
	insertMimirEvent(t, "PoolCycle", constants.PoolCycle, timestamp)
}

func insertMimirEvent(t *testing.T, key string, value int64, blockTimestamp string) {
	const insertq = `INSERT INTO set_mimir_events ` +
		`(key, value, block_timestamp) ` +
		`VALUES ($1, $2, $3)`

	timestamp := nanoWithDefault(blockTimestamp)
	MustExec(t, insertq, key, strconv.FormatInt(value, 10), timestamp)
}
