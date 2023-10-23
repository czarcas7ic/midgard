package db

import (
	"context"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/rs/zerolog/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

type BlockId struct {
	Height    int64
	Timestamp Nano
}

type StoredBlockId struct {
	ptr unsafe.Pointer
}

func (s *StoredBlockId) Set(height int64, timestamp Nano) {
	id := BlockId{
		Height:    height,
		Timestamp: timestamp,
	}
	atomic.StorePointer(&s.ptr, unsafe.Pointer(&id))
}

func (s *StoredBlockId) reset() {
	atomic.StorePointer(&s.ptr, nil)
}

func (s *StoredBlockId) Get() BlockId {
	ret := (*BlockId)(atomic.LoadPointer(&s.ptr))
	if ret != nil {
		return *ret
	}
	return BlockId{}
}

func (s *StoredBlockId) AsHeightTS() oapigen.HeightTS {
	return oapigen.HeightTS{
		Height:    int(s.Get().Height),
		Timestamp: int(s.Get().Timestamp.ToSecond()),
	}
}

var (
	LastThorNodeBlock  StoredBlockId
	LastFetchedBlock   StoredBlockId
	LastCommittedBlock StoredBlockId

	// Note: during batch inserting in the syncup phase the LastAggregatedBlock's Height is not
	// updated (e.g. might be 0) and the timestamp is not an exact block timsetamp.
	// When in sync both Height and Timestamp of the LastAggregatedBlock is correct.
	LastAggregatedBlock StoredBlockId

	FirstBlock StoredBlockId
)

type FullyQualifiedChainId struct {
	Name           string
	StartHash      string
	StartHeight    int64
	HardForkHeight int64
}
type StoredFullyQualifiedChainId struct {
	ptr unsafe.Pointer
}

var (
	CurrentChain StoredFullyQualifiedChainId
	RootChain    StoredFullyQualifiedChainId
)

func (c *StoredFullyQualifiedChainId) Get() FullyQualifiedChainId {
	ret := (*FullyQualifiedChainId)(atomic.LoadPointer(&c.ptr))
	if ret != nil {
		return *ret
	}
	return FullyQualifiedChainId{}
}

func (c *StoredFullyQualifiedChainId) set(cid FullyQualifiedChainId) {
	atomic.StorePointer(&c.ptr, unsafe.Pointer(&cid))
}

func (c *StoredFullyQualifiedChainId) reset() {
	atomic.StorePointer(&c.ptr, nil)
}

func ResetGlobalVarsForTests() {
	FirstBlock.reset()
	LastThorNodeBlock.reset()
	LastFetchedBlock.reset()
	LastCommittedBlock.reset()
	LastAggregatedBlock.reset()

	CurrentChain.reset()
	RootChain.reset()
}

func PrintableHash(encodedHash string) string {
	return strings.ToUpper(hex.EncodeToString([]byte(encodedHash)))
}

// Takes the parameters of the current chain and initializes both the
// `CurrentChain` and `RootChain` global variables
func InitializeChainVars(chainId string, startHeight int64, hash string) {
	current := FullyQualifiedChainId{
		Name:        chainId,
		StartHash:   hash,
		StartHeight: startHeight,
	}
	root := EnrichAndGetRoot(&current)

	// warn if current has been overwritten by config due to the target thornode being
	// recovered at a height which no longer contains the full state since genesis
	if startHeight != current.StartHeight {
		log.Warn().Msgf("Thornode pruned, overwriting start height from %d to %d", startHeight, current.StartHeight)
	}
	if hash != current.StartHash {
		log.Warn().Msgf("Thornode pruned, overwriting start hash from %s to %s", hash, current.StartHash)
	}

	CurrentChain.set(current)
	RootChain.set(root)
}

// Takes the results from ThorNode `status` query and initializes both the
// `CurrentChain` and `RootChain` global variables.
// If the current chain is the root chain, initializes the `FirstBlock` variable too.
func InitializeChainVarsFromThorNodeStatus(status *coretypes.ResultStatus) {
	syncInfo := status.SyncInfo
	hash := PrintableHash(string(syncInfo.EarliestBlockHash))
	height := syncInfo.EarliestBlockHeight
	log.Info().Msgf("ThorNode chain ID: %s", status.NodeInfo.Network)
	log.Info().Msgf("ThorNode earliest block: height=%d hash=%s", height, hash)

	InitializeChainVars(status.NodeInfo.Network, height, hash)
	// If the current chain is the root chain, set the first block too
	if height == RootChain.Get().StartHeight {
		FirstBlock.Set(height, TimeToNano(syncInfo.EarliestBlockTime))
	}
}

func getThorNodeStatus() (*coretypes.ResultStatus, error) {
	endpoint, err := url.Parse(config.Global.ThorChain.TendermintURL)
	if err != nil {
		return nil, err
	}

	ws := endpoint.Path
	endpoint.Path = ""

	rpc, err := rpchttp.NewWithClient(endpoint.String(), ws,
		&http.Client{Timeout: config.Global.ThorChain.ReadTimeout.Value()})
	if err != nil {
		return nil, err
	}
	return rpc.Status(context.Background())
}

// Queries ThorNode `status` endpoint and initializes both the
// `CurrentChain` and `RootChain` global variables based on the result.
// If the current chain is the root chain, initializes the `FirstBlock` variable too.
//
// Convenience function intended for standalone tools
func InitializeChainVarsFromThorNode() {
	status, err := getThorNodeStatus()
	if err != nil {
		log.Fatal().Err(err).Msg("ThorNode status query failed")
	}
	InitializeChainVarsFromThorNodeStatus(status)
}

func firstBlockInDB() (hash string, height int64, timestamp Nano) {
	q := `SELECT height, timestamp, hash FROM block_log ORDER BY height ASC LIMIT 1`
	err := TheDB.QueryRow(q).Scan(&height, &timestamp, &hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", 0, 0
		}
		log.Fatal().Err(err).Msg("Failed to query for the first block")
	}
	if hash == "" {
		log.Fatal().Err(err).Msg("First block hash is empty in the DB")
	}
	hash = PrintableHash(hash)
	return
}

const chainIdKey = "chain_id"

// Checks that values provided are consistent with global variables already initialized.
func SetAndCheckFirstBlock(hash string, height int64, timestamp Nano) {
	startHeight := RootChain.Get().StartHeight
	if genesisHeight := config.Global.Genesis.InitialBlockHeight; genesisHeight > 0 {
		if dbGenesisHeight := ReadDBGenesisHeight(); dbGenesisHeight != genesisHeight {
			log.Fatal().Msgf("Genesis DB height mismatch, config: %d, DB: %d",
				genesisHeight, dbGenesisHeight)
		}

		startHeight = genesisHeight
	}

	if startHeight != height {
		log.Fatal().Msgf("Start height mismatch, ThorNode/config: %d, DB: %d",
			startHeight, height)
	}

	firstBlock := FirstBlock.Get()
	if firstBlock.Height != 0 {
		if firstBlock.Height != height {
			log.Fatal().Msgf("First block height mismatch, ThorNode: %d, DB: %d",
				firstBlock.Height, height)
		}
		if firstBlock.Timestamp != timestamp {
			log.Fatal().Msgf("First block timestamp mismatch, ThorNode: %v, DB: %v",
				firstBlock.Timestamp, timestamp)
		}
	} else {
		FirstBlock.Set(height, timestamp)
	}
}

func EnsureDBMatchesChain() {
	var chainId string
	err := TheDB.QueryRow("SELECT value FROM constants WHERE key = $1", chainIdKey).Scan(&chainId)
	if err != nil && err != sql.ErrNoRows {
		log.Fatal().Err(err).Msg("Failed to read 'chain_id' from constants")
	}

	rootChain := RootChain.Get()
	if chainId != "" && chainId != rootChain.Name {
		log.Fatal().Msgf(
			"Chain id mismatch, ThorNode (root chain id): %s, DB: %s , "+
				"maybe you've re-started Midgard on the wrong chain. "+
				"If it was intentional then recreate the docker DB volume manually or "+
				" instruct Midgard to refresh the db next time by using cmd/nukedb",
			rootChain.Name, chainId)
	}

	hash, height, timestamp := firstBlockInDB()

	genesisHash := config.Global.Genesis.InitialBlockHash
	startHash := rootChain.StartHash
	if genesisHash != "" {
		log.Debug().Msg("The config is based on genesis load.")
		startHash = genesisHash
	}

	if hash != "" {
		if startHash != "" && startHash != hash {
			log.Fatal().Msgf("First hash mismatch, ThorNode: %s, DB: %s", startHash, hash)
		}
		if startHash == "" {
			rootChain.StartHash = hash
			RootChain.set(rootChain)
		}

		SetAndCheckFirstBlock(hash, height, timestamp)

		PreventOverrunAfterFork()
	}

	// Everything OK, if chainId hasn't been recorded yet, record it
	_, err = TheDB.Exec(`INSERT INTO constants (key, value) VALUES ($1, $2)
	ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		chainIdKey, rootChain.Name)
	if err != nil {
		log.Error().Err(err).Msg("Recording chain_id in the DB failed")
	}
}

// If a Midgard is not updated with the correct value of HardForkHeight in
// time before the fork, it might add bogus blocks to the DB. In such a
// case we force it to resync.
func PreventOverrunAfterFork() {
	if CurrentChain.Get().StartHeight == RootChain.Get().StartHeight {
		// There was no fork on this chain
		return
	}

	height := CurrentChain.Get().StartHeight
	var hash string
	err := TheDB.QueryRow(`SELECT hash FROM block_log WHERE height = $1`, height).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			// First block after fork hasn't been recorded yet, all good
			return
		}
		log.Fatal().Err(err).Msgf("Failed to query for block at height %d", height)
	}
	if hash == "" {
		log.Fatal().Err(err).Msg("Hash of the first block after fork is empty in the DB")
	}
	hash = PrintableHash(hash)
	if hash != CurrentChain.Get().StartHash {
		log.Error().Msgf("First block after fork mismatch, ThorNode: %s, DB: %s",
			CurrentChain.Get().StartHash, hash)
		_, err = TheDB.Exec(`INSERT INTO constants (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = $2`,
			ddlHashKey, "bad")
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to clear `ddl_hash` in the constants table")
		}
		log.Fatal().Msg("Marked the DB for reset by overriding the ddl constant")
	}
}

// TODO(huginn): define a better signaling, make it DB aggregate dependent
// 0 == false ; 1 == true
var fetchCaughtUp int32 = 0

func SetFetchCaughtUp() {
	atomic.StoreInt32(&fetchCaughtUp, 1)
}

// FullyCaughtUp returns true if the last stage of block processing (aggregation)
// is less than the configured amount of time in the past.
// At this point Midgard is fully functional and is ready to serve up-to-date data.
func FullyCaughtUp() bool {
	duration := time.Since(LastAggregatedBlock.Get().Timestamp.ToTime())
	return duration < time.Duration(config.Global.MaxBlockAge)
}
