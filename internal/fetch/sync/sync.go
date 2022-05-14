package sync

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/notinchain"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/blockstore"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/util/jobs"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/internal/util/midlog"

	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	jsonrpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
)

var logger = midlog.LoggerForModule("sync")

type Sync struct {
	chainClient *chain.Client
	blockStore  *blockstore.BlockStore

	ctx    context.Context
	status *coretypes.ResultStatus
}

var CheckBlockStoreBlocks = false

func (s *Sync) FetchSingle(height int64) (*coretypes.ResultBlockResults, error) {
	if s.blockStore != nil && s.blockStore.HasHeight(height) {
		block, err := s.blockStore.SingleBlock(height)
		if err != nil {
			return nil, err
		}
		ret := block.Results
		if CheckBlockStoreBlocks {
			fromChain, err := s.chainClient.FetchSingle(height)
			if err != nil {
				return nil, err
			}
			if !reflect.DeepEqual(ret, fromChain) {
				return nil, miderr.InternalErr("Blockstore blocks blocks don't match chain blocks")
			}
		}
		return ret, nil
	}
	return s.chainClient.FetchSingle(height)
}

func reportProgress(nextHeightToFetch, thornodeHeight int64, fetchingFrom string) {
	midgardHeight := nextHeightToFetch - 1
	if midgardHeight < 0 {
		midgardHeight = 0
	}
	if thornodeHeight <= midgardHeight+1 {
		logger.InfoT(
			midlog.Int64("height", midgardHeight),
			"Fully synced")
	} else {
		progress := 100 * float64(midgardHeight) / float64(thornodeHeight)
		logger.InfoT(
			midlog.Tags(
				midlog.Str("progress", fmt.Sprintf("%.2f%%", progress)),
				midlog.Int64("height", midgardHeight),
				midlog.Str("from", fetchingFrom)),
			"Syncing")
	}
}

var lastReportDetailedTime db.Second

// Reports every 5 min when in sync.
func (s *Sync) reportDetailed(offset int64, force bool, fetchingFrom string) {
	currentTime := db.TimeToSecond(time.Now())
	if force || db.Second(60*5) <= currentTime-lastReportDetailedTime {
		lastReportDetailedTime = currentTime
		logger.InfoF("Connected to Tendermint node %q [%q] on chain %q",
			s.status.NodeInfo.DefaultNodeID, s.status.NodeInfo.ListenAddr, s.status.NodeInfo.Network)
		logger.InfoF("Thornode blocks %d - %d from %s to %s",
			s.status.SyncInfo.EarliestBlockHeight,
			s.status.SyncInfo.LatestBlockHeight,
			s.status.SyncInfo.EarliestBlockTime.Format("2006-01-02"),
			s.status.SyncInfo.LatestBlockTime.Format("2006-01-02"))
		reportProgress(offset, s.status.SyncInfo.LatestBlockHeight, fetchingFrom)
	}
}

func (s *Sync) refreshStatus() (finalBlockHeight int64, err error) {
	status, err := s.chainClient.RefreshStatus()
	if err != nil {
		return 0, fmt.Errorf("Status() RPC failed: %w", err)
	}
	s.status = status

	finalBlockHeight = s.status.SyncInfo.LatestBlockHeight
	db.LastThorNodeBlock.Set(s.status.SyncInfo.LatestBlockHeight,
		db.TimeToNano(s.status.SyncInfo.LatestBlockTime))

	return finalBlockHeight, nil
}

// CatchUp reads the latest block height from Status then it fetches all blocks from offset to
// that height.
// The error return is never nil. See ErrQuit and ErrNoData for normal exit.
func (s *Sync) CatchUp(out chan<- chain.Block, startHeight int64) (
	height int64, inSync bool, err error) {
	originalStartHeight := startHeight

	finalBlockHeight, err := s.refreshStatus()
	if err != nil {
		return startHeight, false, fmt.Errorf("Status() RPC failed: %w", err)
	}

	// https://discord.com/channels/838986635756044328/973251236025466961
	// This is a work-around to prevent Midgard from getting stuck at one height for a long time
	// due to that issue. If we have more than one block to fetch do not try to fetch the last one.
	// Fetching blocks before the last seem to be working reliably.
	if finalBlockHeight > startHeight {
		finalBlockHeight -= 1
	}

	i := NewIterator(s, startHeight, finalBlockHeight)

	s.reportDetailed(startHeight, false, i.FetchingFrom())

	// If there are not many blocks to fetch we are probably in sync with ThorNode
	const heightEpsilon = 10
	inSync = finalBlockHeight < originalStartHeight+heightEpsilon

	for {
		if s.ctx.Err() != nil {
			// Job was cancelled.
			return startHeight, false, nil
		}

		block, err := i.Next()
		if err != nil {
			return startHeight, false, err
		}

		if block == nil {
			if !inSync {
				// Force report when there was a long CatchUp
				s.reportDetailed(startHeight, true, i.FetchingFrom())
			}
			s.reportDetailed(startHeight, false, i.FetchingFrom())
			return startHeight, inSync, nil
		}

		select {
		case <-s.ctx.Done():
			return startHeight, false, nil
		case out <- *block:
			startHeight = block.Height + 1
			db.LastFetchedBlock.Set(block.Height, db.TimeToNano(block.Time))

			// report every so often in batch mode too.
			var reportFreq int64 = 1000
			if i.FetchingFrom() == "blockstore" {
				reportFreq = 10000
			}
			if !inSync && startHeight%reportFreq == 1 {
				reportProgress(startHeight, finalBlockHeight, i.FetchingFrom())
			}
		}
	}
}

func (s *Sync) KeepInSync(ctx context.Context, out chan chain.Block) {
	heightOnStart := db.LastCommittedBlock.Get().Height
	midlog.InfoF("Starting chain read from previous height in DB %d", heightOnStart)

	var nextHeightToFetch int64 = heightOnStart + 1

	var previousHeight int64 = 0
	errorCountAtCurrentHeight := 0

	for {
		if ctx.Err() != nil {
			// Requested to stop
			return
		}
		var err error
		var inSync bool

		nextHeightToFetch, inSync, err = s.CatchUp(out, nextHeightToFetch)
		if err != nil {
			var rpcerror *jsonrpctypes.RPCError
			// Don't log this particular error, as we expect to get it quite often.
			// One can only get this error when fetching results for a single (the latest)
			// block.
			// For details, see: https://discord.com/channels/838986635756044328/973251236025466961
			if !(errors.As(err, &rpcerror) &&
				strings.HasPrefix(rpcerror.Data, "could not find results for height")) {
				midlog.DebugF("Block fetch error at height %d, retrying: %v",
					nextHeightToFetch, err)
			}
			if nextHeightToFetch == previousHeight {
				errorCountAtCurrentHeight++
				const maxErrorCount = 20
				if maxErrorCount < errorCountAtCurrentHeight {
					midlog.ErrorF(
						"Already failed %d times fetching height %d, quitting",
						maxErrorCount, nextHeightToFetch)
					jobs.InitiateShutdown()
					return
				}
			}
			db.SleepWithContext(ctx, config.Global.ThorChain.LastChainBackoff.Value())
		}

		if inSync {
			db.SetFetchCaughtUp()
			db.SleepWithContext(ctx, 2*time.Second)
		}

		if previousHeight != nextHeightToFetch {
			previousHeight = nextHeightToFetch
			errorCountAtCurrentHeight = 0
		}
	}
}

func (s *Sync) BlockStoreHeight() int64 {
	return s.blockStore.LastFetchedHeight()
}

var GlobalSync *Sync

func InitGlobalSync(ctx context.Context) {
	var err error
	notinchain.BaseURL = config.Global.ThorChain.ThorNodeURL
	GlobalSync = &Sync{ctx: ctx}
	GlobalSync.chainClient, err = chain.NewClient(ctx)
	if err != nil {
		// error check does not include network connectivity
		logger.FatalE(err, "Exit on Tendermint RPC client instantiation")
	}

	_, err = GlobalSync.refreshStatus()
	if err != nil {
		logger.FatalE(err, "Error fetching ThorNode status")
	}
	db.InitializeChainVarsFromThorNodeStatus(GlobalSync.status)

	GlobalSync.blockStore = blockstore.NewBlockStore(
		ctx, config.Global.BlockStore, db.RootChain.Get().Name)
}

func InitBlockFetch(ctx context.Context) (<-chan chain.Block, jobs.NamedFunction) {
	InitGlobalSync(ctx)

	ch := make(chan chain.Block, GlobalSync.chainClient.BatchSize())
	return ch, jobs.Later("BlockFetch", func() {
		GlobalSync.KeepInSync(ctx, ch)
	})
}
