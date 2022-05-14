package main

// Output:
//
// random_access
//     Count: 30
//     Average: 46.9ms
//     Histogram: 3ms: 3.333%, 10ms: 6.667%, 100ms: 93.333%, 300ms: 100.000%,

// sequencial_access
//     Count: 1000
//     Average: 1.31ms
//     Histogram: 3ms: 99.900%, 10ms: 100.000%,

import (
	"context"
	"math/rand"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/blockstore"
)

const SequentialStartBlock = 990000
const SequentialCount = 15000
const RandomCount = 300

var blockStore *blockstore.BlockStore

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	config.ReadGlobal()
	blockStore = blockstore.NewBlockStore(context.Background(), config.Global.BlockStore, "7D37DEF6E1BE23C912092069325C4A51E66B9EF7DDBDE004FF730CFABC0307B1")
	measureRandomAccess()
	measureSequentialAccess()
}

func measureSequentialAccess() {
	it := blockStore.Iterator(SequentialStartBlock)
	for i := 0; i < SequentialCount; i++ {
		_, err := it.Next()
		if err != nil {
			log.Fatal().Err(err).Msgf("Cannot read block")
		}
	}
}

func measureRandomAccess() {
	for i := 0; i < RandomCount; i++ {
		height := rand.Int63n(blockStore.LastFetchedHeight())
		_, err := blockStore.SingleBlock(height)
		if err != nil {
			log.Fatal().Err(err).Msgf("Cannot read block")
		}
	}
}
