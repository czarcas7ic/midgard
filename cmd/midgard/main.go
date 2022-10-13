package main

import (
	"context"
	"fmt"
	"net/http"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/api"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/decimal"
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

func main() {
	midlog.LogCommandLine()
	config.ReadGlobal()

	// Read pools decimal overwrite from the config
	decimal.AddConfigDecimals()

	mainContext := jobs.InitSignals()

	setupDB()

	waitingJobs := []jobs.NamedFunction{}

	blocks, fetchJob := sync.InitBlockFetch(mainContext)

	// InitBlockFetch may take some time to copy remote blockstore to local.
	// If it was cancelled, we don't create anything else.
	jobs.StopIfCanceled()

	waitingJobs = append(waitingJobs, fetchJob)

	waitingJobs = append(waitingJobs, initBlockWrite(mainContext, blocks))

	waitingJobs = append(waitingJobs, db.InitAggregatesRefresh(mainContext))

	waitingJobs = append(waitingJobs, initHTTPServer(mainContext))

	waitingJobs = append(waitingJobs, initWebsockets(mainContext))

	waitingJobs = append(waitingJobs, api.GlobalCacheStore.InitBackgroundRefresh(mainContext))

	// Up to this point it was ok to fail with log.fatal.
	// From here on errors are handeled by sending a abort on the global signal channel,
	// and all jobs are gracefully shut down.
	jobs.StopIfCanceled()

	runningJobs := []*jobs.RunningJob{}
	for _, waiting := range waitingJobs {
		runningJobs = append(runningJobs, waiting.Start())
	}

	jobs.WaitUntilSignal()

	jobs.ShutdownWait(runningJobs...)

	jobs.LogSignalAndStop()
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
	midlog.InfoF("HTTP server listen port: %d", c.ListenPort)
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
		jobs.InitiateShutdown()
	}()

	return jobs.Later("HTTPserver", func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			midlog.ErrorE(err, "HTTP failed shutdown")
		}
	})
}

func initBlockWrite(ctx context.Context, blocks <-chan chain.Block) jobs.NamedFunction {
	db.EnsureDBMatchesChain()
	record.LoadCorrections(db.RootChain.Get().Name)

	err := notinchain.LoadConstants()
	if err != nil {
		midlog.FatalE(err, "Failed to read constants")
	}

	writer := blockWriter{
		ctx:    ctx,
		blocks: blocks,
	}

	return jobs.Later("BlockWrite", writer.Do)
}

func setupDB() {
	dbinit.Setup()
	err := timeseries.Setup()
	if err != nil {
		midlog.FatalE(err, "Error durring reading last block from DB")
	}
}
