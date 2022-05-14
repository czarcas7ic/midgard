package main

import (
	"context"
	"errors"
	"time"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util/jobs"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

type blockWriter struct {
	ctx    context.Context
	blocks <-chan chain.Block
}

func (x *blockWriter) Do() {
	err := x.loop()
	if err != nil {
		midlog.ErrorE(err, "Unrecoverable error in BlockWriter, terminating")
		jobs.InitiateShutdown()
	}
}

func (x *blockWriter) loop() error {
	var lastHeightWritten int64
	blockBatch := int64(config.Global.TimeScale.CommitBatchSize)

	hardForkHeight := db.CurrentChain.Get().HardForkHeight
	heightBeforeStart := db.LastCommittedBlock.Get().Height
	if hardForkHeight != 0 && hardForkHeight <= heightBeforeStart {
		x.waitAtForkAndExit(heightBeforeStart)
	}

	for {
		if x.ctx.Err() != nil {
			x.logBlockWriteShutdown(lastHeightWritten)
			return nil
		}
		select {
		case <-x.ctx.Done():
			x.logBlockWriteShutdown(lastHeightWritten)
			return nil
		case block := <-x.blocks:
			if block.Height == 0 {
				// Default constructed block, height should be at least 1.
				return errors.New("Block height of 0 is invalid")
			}

			lastBlockBeforeStop := false
			if hardForkHeight != 0 {
				if block.Height == hardForkHeight {
					midlog.WarnT(
						midlog.Int64("height", block.Height),
						"Last block before fork reached, forcing a write to DB")
					lastBlockBeforeStop = true
				}
				if hardForkHeight < block.Height {
					x.waitAtForkAndExit(lastHeightWritten)
					return nil
				}
			}

			// When using the ImmediateInserter we can commit after every block, since it
			// flushes at the end of every block.
			_, immediate := db.Inserter.(*db.ImmediateInserter)

			synced := db.LastThorNodeBlock.Get().Height <= block.Height+1
			commit := immediate || synced || block.Height%blockBatch == 0 || lastBlockBeforeStop
			err := timeseries.ProcessBlock(&block, commit)
			if err != nil {
				return err
			}

			if synced {
				db.RequestAggregatesRefresh()
			}

			lastHeightWritten = block.Height

			if hardForkHeight != 0 && hardForkHeight <= lastHeightWritten {
				x.waitAtForkAndExit(lastHeightWritten)
				return nil
			}
		}
	}
}

func (x *blockWriter) waitAtForkAndExit(lastHeightWritten int64) {
	waitTime := 10 * time.Minute
	midlog.WarnTF(
		midlog.Int64("height", lastHeightWritten),
		"Last block at fork reached, quitting in %v automaticaly", waitTime)
	select {
	case <-x.ctx.Done():
		x.logBlockWriteShutdown(lastHeightWritten)
	case <-time.After(waitTime):
		midlog.WarnT(
			midlog.Int64("height", lastHeightWritten),
			"Waited at last block, restarting to see if fork happened")
		jobs.InitiateShutdown()
	}
}

func (x *blockWriter) logBlockWriteShutdown(lastHeightWritten int64) {
	midlog.InfoF("Shutdown db write process, last height processed: %d", lastHeightWritten)
}
