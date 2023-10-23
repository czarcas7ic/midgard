package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
	"gopkg.in/yaml.v3"
)

type Duration time.Duration

type Config struct {
	ListenPort      int      `yaml:"listen_port" split_words:"true"`
	ShutdownTimeout Duration `yaml:"shutdown_timeout" split_words:"true"`

	// ReadTimeout and WriteTimeout refer to the webserver timeouts
	ReadTimeout  Duration `yaml:"read_timeout" split_words:"true"`
	WriteTimeout Duration `yaml:"write_timeout" split_words:"true"`

	// v2/health:InSync is true if Now - LastAvailableBlock < MaxBlockAge
	MaxBlockAge Duration `yaml:"max_block_age" split_words:"true"`

	ThorChain ThorChain `yaml:"thorchain"`

	BlockStore BlockStore

	Genesis Genesis

	// TODO(muninn): Renaming this to DB whenever config values are renamed in coordination with SREs.
	TimeScale TimeScale `yaml:"timescale"`

	Websockets Websockets `yaml:"websockets" split_words:"true"`

	UsdPools []string `yaml:"usdpools" split_words:"true"`

	// you can specify on the command line with: MIDGARD_POOLS_DECIMAL="A.A:8,B.B:18"
	PoolsDecimal map[string]int64 `yaml:"pools_decimal" split_words:"true"`

	// These are filtered addresses that will be ignored by the action endpoint
	// Address: Label of the address,also it can be empty string
	FilteredAddresses map[string]string `yaml:"filtered_addresses" split_words:"true"`

	EventRecorder EventRecorder `yaml:"event_recorder" split_words:"true"`

	CaseInsensitiveChains map[string]bool `yaml:"case_insensitive_chains" split_words:"true"`

	Logs midlog.LogConfig `yaml:"logs" split_words:"true"`

	// Temporary solution, will be field will be removed in the future.
	TmpActionsCountTimeout Duration `yaml:"tmp_actions_count_timeout" split_words:"true"`

	Endpoints Endpoints `yaml:"endpoints" split_words:"true"`

	Debug Debug `yaml:"debug" split_words:"true"`
}

type Genesis struct {
	Local              string `yaml:"local" split_words:"true"`
	InitialBlockHash   string `yaml:"initial_block_hash" split_words:"true"`
	InitialBlockHeight int64  `yaml:"initial_block_height" split_words:"true"`
}

type BlockStore struct {
	Local                  string `yaml:"local" split_words:"true"`
	Remote                 string `yaml:"remote" split_words:"true"`
	BlocksPerChunk         int64  `yaml:"blocks_per_chunk" split_words:"true"`
	CompressionLevel       int    `yaml:"compression_level" split_words:"true"`
	ChunkHashesPath        string `yaml:"chunk_hashes_path" split_words:"true"`
	DownloadFullChunksOnly bool   `yaml:"download_full_chunks_only" split_words:"true"`
}

type EventRecorder struct {
	OnTransferEnabled bool `yaml:"on_transfer_enabled" split_words:"true"`
	OnMessageEnabled  bool `yaml:"on_message_enabled" split_words:"true"`
}

type ThorChain struct {
	TendermintURL  string `yaml:"tendermint_url" split_words:"true"`
	ThorNodeURL    string `yaml:"thornode_url" split_words:"true"`
	FetchBatchSize int    `yaml:"fetch_batch_size" split_words:"true"`
	Parallelism    int    `yaml:"parallelism" split_words:"true"`

	// Timeout for fetch requests to ThorNode
	ReadTimeout Duration `yaml:"read_timeout" split_words:"true"`

	// If fetch from ThorNode fails, wait this much before retrying
	LastChainBackoff Duration `yaml:"last_chain_backoff" split_words:"true"`

	// Entries found in the config are appended to the compiled-in entries from `chainancestry.go`
	// (ie., they override the compiled-in values if there is a definition for the same ChainId
	// in both.)
	//
	// Parent chains should come before their children.
	ForkInfos []ForkInfo `yaml:"fork_infos" split_words:"true"`

	// MaxStatusRetries is the number of times to fetch Thornode status before fatal.
	MaxStatusRetries int `yaml:"max_status_retries" split_words:"true"`

	// StatusRetryBackoff is the time to wait between Thornode status retries.
	StatusRetryBackoff Duration `yaml:"status_retry_backoff" split_words:"true"`
}

// Both `EarliestBlockHash` and `EarliestBlockHeight` are optional and mostly just used for sanity
// checking.
//
// `EarliestBlockHeight` defaults to 1, if it has no parent. Or to `parent.HardForkHeight + 1`
// otherwise.
//
// If `EarliestBlockHash` is unset then its consistency with DB is not checked.
//
// If `HardForkHeight` is set for the current chain Midgard will stop there. This height will be
// the last written to the DB.
//
// When a fork is coming up it's useful to prevent Midgard from writing out data from the old chain
// beyond the fork height.
type ForkInfo struct {
	ChainId             string `yaml:"chain_id" split_words:"true"`
	ParentChainId       string `yaml:"parent_chain_id" split_words:"true"`
	EarliestBlockHash   string `yaml:"earliest_block_hash" split_words:"true"`
	EarliestBlockHeight int64  `yaml:"earliest_block_height" split_words:"true"`
	HardForkHeight      int64  `yaml:"hard_fork_height" split_words:"true"`
}

type TimeScale struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	UserName string `yaml:"user_name"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	Sslmode  string `yaml:"sslmode"`

	// -1 sets it to infinite
	MaxOpenConns    int `yaml:"max_open_conns"`
	CommitBatchSize int `yaml:"commit_batch_size"`

	// If DDL mismatch is detected exit with error instead of resetting the schema
	NoAutoUpdateDDL bool `yaml:"no_auto_update_ddl"`
	// If DDL mismatch for aggregates is detected exit with error instead of resetting
	// the aggregates. Implies `NoAutoUpdateDDL`
	NoAutoUpdateAggregatesDDL bool `yaml:"no_auto_update_aggregates_ddl"`
}

type Websockets struct {
	Enable          bool `yaml:"enable" split_words:"true"`
	ConnectionLimit int  `yaml:"connection_limit" split_words:"true"`
}

type Endpoints struct {
	ActionParams ActionParams `yaml:"action_params" split_words:"true"`
}

// Actions endpoint parameters maximum number possible to query
type ActionParams struct {
	MaxLimit     uint64 `yaml:"max_limit" split_words:"true"`
	MaxAddresses int    `yaml:"max_addresses" split_words:"true"`
	MaxAssets    int    `yaml:"max_assets" split_words:"true"`
}

type Debug struct {
	EnableAggregationTimer bool `yaml:"enable_aggregation_timer" split_words:"true"`
}

var defaultConfig = Config{
	ListenPort: 8080,
	ThorChain: ThorChain{
		ThorNodeURL:      "http://localhost:1317/thorchain",
		TendermintURL:    "http://localhost:26657/websocket",
		ReadTimeout:      Duration(8 * time.Second),
		LastChainBackoff: Duration(7 * time.Second),

		// NOTE(huginn): numbers are chosen to give a good performance on an "average" desktop
		// machine with a 4 core CPU. With more cores it might make sense to increase the
		// parallelism, though care should be taken to not overload the Thornode.
		// See `docs/parallel_batch_bench.md` for measurments to guide selection of these parameters.
		FetchBatchSize: 100, // must be divisible by BlockFetchParallelism
		Parallelism:    4,

		MaxStatusRetries:   10,
		StatusRetryBackoff: Duration(5 * time.Second),
	},
	BlockStore: BlockStore{
		BlocksPerChunk:   10000,
		CompressionLevel: 1, // 0 means no compression
	},
	TimeScale: TimeScale{
		MaxOpenConns:    80,
		CommitBatchSize: 100,
	},
	ShutdownTimeout: Duration(20 * time.Second),
	ReadTimeout:     Duration(20 * time.Second),
	WriteTimeout:    Duration(20 * time.Second),
	MaxBlockAge:     Duration(60 * time.Second),
	UsdPools: []string{
		"BNB.BUSD-BD1",
		"ETH.USDT-0XDAC17F958D2EE523A2206206994597C13D831EC7",
		"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48",
	},
	CaseInsensitiveChains: map[string]bool{
		"ETH": true,
	},
	EventRecorder: EventRecorder{
		OnTransferEnabled: true,
		OnMessageEnabled:  false,
	},
	TmpActionsCountTimeout: Duration(7000 * time.Millisecond),
	Endpoints: Endpoints{
		ActionParams: ActionParams{
			MaxLimit:     50,
			MaxAddresses: 50,
			MaxAssets:    4,
		},
	},
	Debug: Debug{
		EnableAggregationTimer: false,
	},
}

var logger = midlog.LoggerForModule("config")

func (d Duration) Value() time.Duration {
	return time.Duration(d)
}
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	err := unmarshal(&v)
	if err != nil {
		return err
	}
	switch value := v.(type) {
	case string:
		v, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(v)
	default:
		return errors.New("duration not a string")
	}
	return nil
}

func (d *Duration) Decode(value string) error {
	v, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func MustLoadConfigFiles(colonSeparatedFilenames string, c *Config) {
	if colonSeparatedFilenames == "" || colonSeparatedFilenames == "null" {
		return
	}

	for _, filename := range strings.Split(colonSeparatedFilenames, ":") {
		mustLoadConfigFile(filename, c)
	}
}

func mustLoadConfigFile(path string, c *Config) {
	f, err := os.Open(path)
	if err != nil {
		logger.FatalE(err, "Exit on configuration file unavailable")
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)

	dec.KnownFields(true)

	if err := dec.Decode(&c); err != nil {
		logger.FatalE(err, "Exit on malformed configuration")
	}
}

func LogAndcheckUrls(c *Config) {
	urls := []struct {
		url, name string
	}{
		{c.ThorChain.ThorNodeURL, "THORNode REST URL"},
		{c.ThorChain.TendermintURL, "Tendermint RPC URL"},
		{c.BlockStore.Remote, "BlockStore Remote URL"},
	}
	for _, v := range urls {
		logger.InfoF("%s: %q", v.name, v.url)
		if _, err := url.Parse(v.url); err != nil {
			logger.FatalEF(err, "Exit on malformed %s", v.url)
		}
	}
}

// Not thread safe, it is written once, then only read
var Global Config = defaultConfig

func readConfigFrom(filenames string) Config {
	var ret Config = defaultConfig
	MustLoadConfigFiles(filenames, &ret)

	// override config with env variables
	err := envconfig.Process("midgard", &ret)
	if err != nil {
		logger.FatalE(err, "Failed to process config environment variables")
	}

	LogAndcheckUrls(&ret)

	return ret
}

// filenames is a colon separated list of files.
// Values in later files overwrite values from earlier files.
func ReadGlobalFrom(filenames string) {
	Global = readConfigFrom(filenames)
	midlog.SetFromConfig(Global.Logs)
}

func ReadGlobal() {
	switch len(os.Args) {
	case 1:
		ReadGlobalFrom("")
	case 2:
		ReadGlobalFrom(os.Args[1])
	default:
		logger.Fatal("One optional configuration file argument only-no flags")
	}
}
