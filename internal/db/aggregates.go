package db

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/internal/util/jobs"
	"gitlab.com/thorchain/midgard/internal/util/timer"
)

// TODO(huginn): if sync is fast and can do a lot of work in 5 minutes:
// - refresh once immediately after sync is finished
// - report inSync on `v2/health` only after aggregates are refreshed
const (
	aggregatesInitialDelay    = 10 * time.Second
	aggregatesRefreshInterval = 5 * time.Minute
)

type aggregateParams struct {
	lowerQuery  string
	higherQuery string
}

var aggregates = map[string]aggregateParams{}

// TimescaleDB does not support continuous aggregates where the time buckets
// are not constant size (ie., month, year), so for those we need to aggregate
// the daily aggregate into higher aggregates.
//
// The `lowerQuery` is the query template for creating the materialized view
// from a hypertable. Should contain a single %s hole which will be filled by
// a `time_bucket(..., block_timestamp)`, and this column should be named
// `bucket_start`.
//
// The `higherQuery` is the query template for creating views from the daily aggregate.
// Should contain a single %s hole which will be filled by a `nano_trunc(..., d.bucket_start)`,
// so the daily aggregate should be aliased as `d`.
func RegisterAggregate(name string, lowerQuery string, upperQuery string) {
	aggregates[name] = aggregateParams{lowerQuery, upperQuery}
}

func AggregatesDdl() string {
	var b strings.Builder
	fmt.Fprint(&b, `
		-- version 1

		DROP SCHEMA IF EXISTS midgard_agg CASCADE;
		CREATE SCHEMA midgard_agg;

	`)

	// Sort to iterate in deterministic order.
	// We need this to avoid unnecessarily recreating the 'aggregate' schema.
	aggregateNames := make([]string, 0, len(aggregates))
	for name := range aggregates {
		aggregateNames = append(aggregateNames, name)
	}
	sort.Strings(aggregateNames)

	for _, name := range aggregateNames {
		aggregate := aggregates[name]
		for _, bucket := range intervals {
			if bucket.exact {
				bucketField := fmt.Sprintf("time_bucket('%d', block_timestamp)",
					bucket.minDuration*1e9)
				q := strings.TrimSpace(fmt.Sprintf(aggregate.lowerQuery, bucketField))
				fmt.Fprint(&b, `
					CREATE MATERIALIZED VIEW midgard_agg.`+name+`_`+bucket.name+`
					WITH (timescaledb.continuous) AS
					`+q+`
					WITH NO DATA;
				`)
			} else {
				bucketField := fmt.Sprintf("nano_trunc('%s', d.bucket_start)",
					bucket.name)
				q := strings.TrimSpace(fmt.Sprintf(aggregate.higherQuery, bucketField))
				fmt.Fprint(&b, `
					CREATE VIEW midgard_agg.`+name+`_`+bucket.name+` AS
					`+q+`;
				`)
			}
		}
	}
	return b.String()
}

func DropAggregates() (err error) {
	_, err = theDB.Exec(`
		DROP SCHEMA IF EXISTS midgard_agg CASCADE;
		DELETE FROM midgard.constants WHERE key = '` + aggregatesDdlHashKey + `';
	`)
	return
}

var aggregatesRefreshTimer = timer.NewTimer("aggregates_refresh")

func refreshAggregates(ctx context.Context) {
	defer aggregatesRefreshTimer.One()()
	log.Debug().Msg("Refreshing aggregates")

	refreshEnd := LastBlockTimestamp() - 5*60*1e9
	for name := range aggregates {
		for _, bucket := range intervals {
			if !bucket.exact {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			q := fmt.Sprintf("CALL refresh_continuous_aggregate('midgard_agg.%s_%s', NULL, '%d')",
				name, bucket.name, refreshEnd)
			_, err := theDB.Exec(q)
			if err != nil {
				log.Error().Err(err).Msgf("Refreshing %s_%s", name, bucket.name)
			}
		}
	}

	log.Debug().Msg("Refreshing done")
}

func StartAggregatesRefresh(ctx context.Context) *jobs.Job {
	log.Info().Msg("Starting aggregates refresh job")
	job := jobs.Start("AggregatesRefresh", func() {
		jobs.Sleep(ctx, aggregatesInitialDelay)
		for {
			if ctx.Err() != nil {
				log.Info().Msg("Shutdown aggregates refresh job")
				return
			}
			refreshAggregates(ctx)
			jobs.Sleep(ctx, aggregatesRefreshInterval)
		}
	})
	return &job
}
