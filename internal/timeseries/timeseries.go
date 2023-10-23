package timeseries

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/util/timer"
)

// OutboundTimeout is an upperboundary for the amount of time for a followup on outbound events.
const OutboundTimeout = time.Hour * 48

// LastBlockTrack is an in-memory copy of the write state.
// TODO(acsaba): migrate users to using BlockState wherever it's possible.
var lastBlockTrack atomic.Value

var blockFlushTimer = timer.NewTimer("block_write_flush")

// BlockTrack is a write state.
type blockTrack struct {
	Height int64
	// TODO(acsaba): rewrite to db.Nano
	Timestamp time.Time
	Hash      []byte
	aggTrack
}

// AggTrack has a snapshot of runningTotals.
type aggTrack struct {
	AssetE8DepthPerPool map[string]int64
	RuneE8DepthPerPool  map[string]int64
	SynthE8DepthPerPool map[string]int64
}

// Setup initializes the package. The previous state is restored (if there was any).
func Setup() error {
	const q = "SELECT height, timestamp, hash, agg_state FROM block_log ORDER BY height DESC LIMIT 1"
	rows, err := db.Query(context.Background(), q)
	if err != nil {
		return fmt.Errorf("last block lookup: %w", err)
	}
	defer rows.Close()

	var track blockTrack
	if rows.Next() {
		var ns int64
		var aggSerial []byte
		err := rows.Scan(&track.Height, &ns, &track.Hash, &aggSerial)
		if err != nil {
			return err
		}
		track.Timestamp = time.Unix(0, ns)
		if err := gob.NewDecoder(bytes.NewReader(aggSerial)).Decode(&track.aggTrack); err != nil {
			return fmt.Errorf("restore with malformed aggregation state denied on %w", err)
		}
	}

	// sync in-memory tracker
	setLastBlock(&track)

	// apply aggregation state to recorder
	for pool, E8 := range track.AssetE8DepthPerPool {
		record.Recorder.SetAssetDepth(pool, E8)
	}
	for pool, E8 := range track.RuneE8DepthPerPool {
		record.Recorder.SetRuneDepth(pool, E8)
	}
	for pool, E8 := range track.SynthE8DepthPerPool {
		record.Recorder.SetSynthDepth(pool, E8)
	}

	return rows.Err()
}

// QueryOneValue is a helper to make store single value queries
// result into dest
func QueryOneValue(dest interface{}, ctx context.Context, query string, args ...interface{}) error {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	ok := rows.Next()

	if !ok {
		return errors.New("Expected one result from query but got none.")
	}

	err = rows.Scan(dest)
	if err != nil {
		return err
	}

	return nil
}

func ProcessBlock(block *chain.Block, commit bool) (err error) {
	err = db.Inserter.StartBlock()
	if err != nil {
		return
	}

	// Record all the events
	record.ProcessBlock(block)

	// in-memory snapshot
	track := blockTrack{
		Height:    block.Height,
		Timestamp: block.Time,
		Hash:      block.Hash,
		aggTrack: aggTrack{
			AssetE8DepthPerPool: record.Recorder.AssetE8DepthPerPool(),
			RuneE8DepthPerPool:  record.Recorder.RuneE8DepthPerPool(),
			SynthE8DepthPerPool: record.Recorder.SynthE8DepthPerPool(),
		},
	}

	firstBlockHeight := db.FirstBlock.Get().Height
	// We know that this is the first block if:
	// - db.FirstBlock was not set yet
	// - it was set and this block is it
	thisIsTheFirstBlock := firstBlockHeight == 0 || block.Height <= firstBlockHeight
	firstBlockGenesis := db.ConfigHasGenesis() && db.GenesisInfo.Get().Height == block.Height

	var aggSerial bytes.Buffer
	if commit || thisIsTheFirstBlock || firstBlockGenesis {
		// Persist the current state to the DB on "commit" blocks.
		// This way we can continue after being interrupted, but not waste space on intermediary
		// blocks in the batch.
		if err := gob.NewEncoder(&aggSerial).Encode(&track.aggTrack); err != nil {
			// won't bring the service down, but prevents state recovery
			log.Error().Err(err).Msg("Failed to persist tracking state")
		}
	}
	cols := []string{"height", "timestamp", "hash", "agg_state"}
	err = db.Inserter.Insert("block_log", cols, block.Height, block.Time.UnixNano(), block.Hash, aggSerial.Bytes())
	if err != nil {
		return fmt.Errorf("persist block height %d: %w", block.Height, err)
	}

	err = depthRecorder.update(block.Time,
		track.aggTrack.AssetE8DepthPerPool,
		track.aggTrack.RuneE8DepthPerPool,
		track.aggTrack.SynthE8DepthPerPool)
	if err != nil {
		return
	}

	err = db.Inserter.EndBlock()
	if err != nil {
		return
	}

	if commit || thisIsTheFirstBlock || firstBlockGenesis {
		defer blockFlushTimer.One()()

		err = db.Inserter.Flush()
		if err != nil {
			db.MarkBatchInserterFail()
			log.Fatal().Err(err).Msg("Inserter.Flush() failed. Marking BatchInserter as failed and exiting to switch to TxInserter.")
			return
		}
		// update global in-memory state
		setLastBlock(&track)

		// For the first block:
		if thisIsTheFirstBlock {
			hash := db.PrintableHash(string(block.Hash))
			log.Info().Int64("height", block.Height).Str("hash", hash).Msg("Processed first block")
			db.SetAndCheckFirstBlock(hash, block.Height, db.TimeToNano(block.Time))
		}
	}
	return nil
}

func setLastBlock(track *blockTrack) {
	lastBlockTrack.Store(track)
	db.LastCommittedBlock.Set(track.Height, db.TimeToNano(track.Timestamp))
	Latest.setLatestStates(track)
}

func getLastBlock() *blockTrack {
	interfacePtr := lastBlockTrack.Load()
	if interfacePtr == nil {
		log.Panic().Msg("LastBlock not loaded yet")
	}
	return interfacePtr.(*blockTrack)
}

// Deprecated, use timeseries.Latest.GetState().{Heiht,Timestamp} instead
func LastBlock() (height int64, timestamp time.Time, hash []byte) {
	track := getLastBlock()
	return track.Height, track.Timestamp, track.Hash
}

// Deprecated, use timeseries.Latest.GetState().PoolInfo(poolname) instead
func AssetAndRuneDepths() (assetE8PerPool, runeE8PerPool map[string]int64, timestamp time.Time) {
	track := getLastBlock()
	return track.aggTrack.AssetE8DepthPerPool, track.aggTrack.RuneE8DepthPerPool, track.Timestamp
}
