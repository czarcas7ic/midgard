package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pascaldekloe/metrics/gostat"
	"github.com/rs/zerolog/log"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/api"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/notinchain"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/fetch/sync"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util/jobs"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
	"gitlab.com/thorchain/midgard/internal/util/timer"
	"gitlab.com/thorchain/midgard/internal/websockets"
)

var writeTimer = timer.NewTimer("block_write_total")

var signals chan os.Signal

func main() {
	midlog.LogStartCommand()
	config.ReadGlobal()

	midlog.Init()

	signals = make(chan os.Signal, 10)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// include Go runtime metrics
	gostat.CaptureEvery(5 * time.Second)

	setupDB()

	// TODO(muninn): Don't start the jobs immediately, but wait till they are _all_ done
	// with their setups (and potentially log.Fatal()ed) and then start them together.

	mainContext, mainCancel := context.WithCancel(context.Background())

	var exitSignal os.Signal
	signalWatcher := jobs.Start("SignalWatch", func() {

		exitSignal = <-signals
		midlog.Warn("Shutting down initiated")
		mainCancel()
	})

	waitingJobs := []jobs.NamedFunction{}

	blocks, fetchJob := sync.InitBlockFetch(mainContext)

	// InitBlockFetch may take some time to copy remote blockstore to local.
	// If it was cancelled, we don't create anything else.
	if mainContext.Err() != nil {
		midlog.FatalF("Exit on signal %s", exitSignal)
	}

	waitingJobs = append(waitingJobs, fetchJob)

	waitingJobs = append(waitingJobs, initBlockWrite(mainContext, blocks))

	waitingJobs = append(waitingJobs, db.InitAggregatesRefresh(mainContext))

	waitingJobs = append(waitingJobs, initHTTPServer(mainContext))

	waitingJobs = append(waitingJobs, initWebsockets(mainContext))

	waitingJobs = append(waitingJobs, api.GlobalCacheStore.InitBackgroundRefresh(mainContext))

	if mainContext.Err() != nil {
		midlog.FatalF("Exit on signal %s", exitSignal)
	}

	// Up to this point it was ok to fail with log.fatal.
	// From here on errors are handeled by sending a abort on the global signal channel,
	// and all jobs are gracefully shut down.
	runningJobs := []*jobs.RunningJob{}
	for _, waiting := range waitingJobs {
		runningJobs = append(runningJobs, waiting.Start())
	}

	signalWatcher.MustWait()

	timeout := config.Global.ShutdownTimeout.Value()
	midlog.InfoF("Shutdown timeout %s", timeout)
	finishCTX, finishCancel := context.WithTimeout(context.Background(), timeout)
	defer finishCancel()

	jobs.WaitAll(finishCTX, runningJobs...)

	midlog.FatalF("Exit on signal %s", exitSignal)
}

func initWebsockets(ctx context.Context) jobs.NamedFunction {
	if !config.Global.Websockets.Enable {
		midlog.Info("Websockets are not enabled")
		return jobs.EmptyJob()
	}
	db.CreateWebsocketChannel()
	websocketsJob, err := websockets.Init(ctx, config.Global.Websockets.ConnectionLimit)
	if err != nil {
		midlog.FatalE(err, "Websockets failure")
	}
	return websocketsJob
}

func initHTTPServer(ctx context.Context) jobs.NamedFunction {
	c := &config.Global
	if c.ListenPort == 0 {
		c.ListenPort = 8080
		midlog.InfoF("Default HTTP server listen port to %d", c.ListenPort)
	}
	api.InitHandler(c.ThorChain.ThorNodeURL, c.ThorChain.ProxiedWhitelistedEndpoints)
	srv := &http.Server{
		Handler:      api.Handler,
		Addr:         fmt.Sprintf(":%d", c.ListenPort),
		ReadTimeout:  c.ReadTimeout.Value(),
		WriteTimeout: c.WriteTimeout.Value(),
	}

	// launch HTTP server
	go func() {
		err := srv.ListenAndServe()
		midlog.ErrorE(err, "HTTP stopped")
		signals <- syscall.SIGABRT
	}()

	return jobs.Later("HTTPserver", func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			midlog.ErrorE(err, "HTTP failed shutdown")
		}
	})
}

func logBlockWriteShutdown(lastHeightWritten int64) {
	midlog.InfoF("Shutdown db write process, last height processed: %d", lastHeightWritten)
}

func waitAtForkAndExit(ctx context.Context, lastHeightWritten int64) {
	waitTime := 10 * time.Minute
	log.Warn().Int64("height", lastHeightWritten).Msgf(
		"Last block at fork reached, quitting in %v automaticaly", waitTime)
	select {
	case <-ctx.Done():
		logBlockWriteShutdown(lastHeightWritten)
	case <-time.After(waitTime):
		log.Warn().Int64("height", lastHeightWritten).Msg(
			"Waited at last block, restarting to see if fork happened")
		signals <- syscall.SIGABRT
	}
}

func initBlockWrite(ctx context.Context, blocks <-chan chain.Block) jobs.NamedFunction {
	db.EnsureDBMatchesChain()
	record.LoadCorrections(db.RootChain.Get().Name)

	err := notinchain.LoadConstants()
	if err != nil {
		midlog.FatalE(err, "Failed to read constants")
	}
	var lastHeightWritten int64
	blockBatch := int64(config.Global.TimeScale.CommitBatchSize)

	return jobs.Later("BlockWrite", func() {
		var err error
		// TODO(huginn): replace loop label with some logic

		hardForkHeight := db.CurrentChain.Get().HardForkHeight
		heightBeforeStart := db.LastCommittedBlock.Get().Height
		if hardForkHeight != 0 && hardForkHeight <= heightBeforeStart {
			waitAtForkAndExit(ctx, heightBeforeStart)
		}

	loop:
		for {
			if ctx.Err() != nil {
				logBlockWriteShutdown(lastHeightWritten)
				return
			}
			select {
			case <-ctx.Done():
				logBlockWriteShutdown(lastHeightWritten)
				return
			case block := <-blocks:
				if block.Height == 0 {
					// Default constructed block, height should be at least 1.
					midlog.Error("Block height of 0 is invalid")
					break loop
				}

				lastBlockBeforeStop := false
				if hardForkHeight != 0 {
					if block.Height == hardForkHeight {
						log.Warn().Int64("height", block.Height).Msgf(
							"Last block before fork reached, forcing a write to DB")
						lastBlockBeforeStop = true
					}
					if hardForkHeight < block.Height {
						waitAtForkAndExit(ctx, lastHeightWritten)
						return
					}
				}

				t := writeTimer.One()

				// When using the ImmediateInserter we can commit after every block, since it
				// flushes at the end of every block.
				_, immediate := db.Inserter.(*db.ImmediateInserter)

				synced := block.Height == db.LastThorNodeBlock.Get().Height
				commit := immediate || synced || block.Height%blockBatch == 0 || lastBlockBeforeStop
				err = timeseries.ProcessBlock(&block, commit)
				if err != nil {
					break loop
				}

				if synced {
					db.RequestAggregatesRefresh()
				}

				lastHeightWritten = block.Height
				t()

				if hardForkHeight != 0 && hardForkHeight <= lastHeightWritten {
					waitAtForkAndExit(ctx, lastHeightWritten)
					return
				}
			}
		}
		midlog.ErrorE(err, "Unrecoverable error in BlockWriter, terminating")
		signals <- syscall.SIGABRT
	})
}

func setupDB() {
	db.Setup()
	err := timeseries.Setup()
	if err != nil {
		midlog.FatalE(err, "Error durring reading last block from DB")
	}
}
