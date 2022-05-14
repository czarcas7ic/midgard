package jobs

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

var signals chan os.Signal
var exitSignal os.Signal
var signalWatcher *RunningJob
var mainContext context.Context

func InitiateShutdown() {
	signals <- syscall.SIGABRT
}

func LogSignalAndStop() {
	midlog.FatalF("Exit on signal %s", exitSignal)
}

func StopIfCanceled() {
	if mainContext.Err() != nil {
		LogSignalAndStop()
	}
}

func InitSignals() context.Context {
	signals = make(chan os.Signal, 20)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	var mainCancel context.CancelFunc
	mainContext, mainCancel = context.WithCancel(context.Background())

	job := Start("SignalWatch", func() {
		exitSignal = <-signals
		midlog.Warn("Shutting down initiated")
		mainCancel()
	})
	signalWatcher = &job

	return mainContext
}

func WaitUntilSignal() {
	signalWatcher.MustWait()
}

// Assumes WaitUntilSignal finished therefore all jobs already started their shutdown.
func ShutdownWait(allJobs ...*RunningJob) {
	if mainContext.Err() == nil {
		log.Fatal("Maincontext is not cancelled, but wait for shutdown was initated")
	}
	timeout := config.Global.ShutdownTimeout.Value()
	midlog.InfoF("Shutdown timeout %s", timeout)
	finishCTX, finishCancel := context.WithTimeout(context.Background(), timeout)
	defer finishCancel()
	WaitAll(finishCTX, allJobs...)
}
