package db

import (
	"context"
	"fmt"
	"io"
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

type aggregateColumnType int

const (
	groupAggregateColumn aggregateColumnType = iota
	sumAggregateColumn
	lastAggregateColumn
)

type aggregateColumn struct {
	name       string
	expression string
	columnType aggregateColumnType
}

type aggregateDescription struct {
	name    string
	table   string
	columns []aggregateColumn
}

func NewAggregate(name string, table string) *aggregateDescription {
	return &aggregateDescription{name: name, table: table}
}

func (a *aggregateDescription) addExpression(name string, expression string, cType aggregateColumnType) *aggregateDescription {
	a.columns = append(a.columns, aggregateColumn{
		name:       name,
		expression: expression,
		columnType: cType,
	})
	return a
}

func (a *aggregateDescription) AddGroupExpression(name string, expression string) *aggregateDescription {
	return a.addExpression(name, expression, groupAggregateColumn)
}

func (a *aggregateDescription) AddGroupColumn(column string) *aggregateDescription {
	return a.AddGroupExpression(column, column)
}

func (a *aggregateDescription) AddSumlikeExpression(name string, expression string) *aggregateDescription {
	return a.addExpression(name, expression, sumAggregateColumn)
}

func (a *aggregateDescription) AddSumColumn(column string) *aggregateDescription {
	return a.AddSumlikeExpression(column, "SUM("+column+")")
}

// If the column is know to be BIGINT this is preferred to the plain `AddSumColumn`
func (a *aggregateDescription) AddBigintSumColumn(column string) *aggregateDescription {
	return a.AddSumlikeExpression(column, "SUM("+column+")::BIGINT")
}

// Note, unlike the `AddGroupExpression` and `AddSumlikeExpression`, here the `expression` is not
// the whole definition for the aggregate column, just the first argument for `last()`.
// The second argument is assumed to be the timestamp and will be added automatically.
func (a *aggregateDescription) AddLastExpression(name string, expression string) *aggregateDescription {
	return a.addExpression(name, expression, lastAggregateColumn)
}

func (a *aggregateDescription) AddLastColumn(column string) *aggregateDescription {
	return a.addExpression(column, column, lastAggregateColumn)
}

func (agg *aggregateDescription) groupColumns(includeTimestamp bool) []string {
	var columns []string
	if includeTimestamp {
		columns = append(columns, "aggregate_timestamp")
	}
	for _, c := range agg.columns {
		if c.columnType == groupAggregateColumn {
			columns = append(columns, c.name)
		}
	}
	return columns
}

func (agg *aggregateDescription) baseQueryBuilder(b io.Writer, aggregateTimestamp string, whereConditions []string, groupColumns []string) {
	fmt.Fprint(b, "SELECT\n")
	for _, c := range agg.columns {
		expression := c.expression
		if c.columnType == lastAggregateColumn {
			expression = "last(" + expression + ", block_timestamp)"
		}
		fmt.Fprintf(b, "\t\t\t%s AS %s,\n", expression, c.name)
	}
	fmt.Fprintf(b, "\t\t\t%s AS aggregate_timestamp\n", aggregateTimestamp)

	fmt.Fprintf(b, "\t\tFROM %s\n", agg.table)
	if len(whereConditions) > 0 {
		fmt.Fprintf(b, "\t\t%s\n", Where(whereConditions...))
	}
	if len(groupColumns) > 0 {
		fmt.Fprintf(b, "\t\tGROUP BY %s", strings.Join(groupColumns, ", "))
	}
}

func (agg *aggregateDescription) baseQuery(aggregateTimestamp string) string {
	var b strings.Builder
	agg.baseQueryBuilder(&b, aggregateTimestamp, nil, agg.groupColumns(true))
	return b.String()
}

func (agg *aggregateDescription) aggregateQueryBuilder(
	b io.Writer,
	subquery string,
	subqueryName string,
	aggregateTimestamp string,
	whereConditions []string,
	groupColumns []string,
) {
	fmt.Fprint(b, "SELECT\n")
	for _, c := range agg.columns {
		expression := subqueryName + "." + c.name
		switch c.columnType {
		case sumAggregateColumn:
			expression = "SUM(" + expression + ")"
		case lastAggregateColumn:
			expression = "last(" + expression + ", " + subqueryName + ".aggregate_timestamp)"
		}
		fmt.Fprintf(b, "\t\t\t%s AS %s,\n", expression, c.name)
	}
	fmt.Fprintf(b, "\t\t\t%s AS aggregate_timestamp\n", aggregateTimestamp)

	fmt.Fprint(b, "\t\tFROM "+subquery+" AS "+subqueryName+"\n")
	if len(whereConditions) > 0 {
		fmt.Fprintf(b, "\t\t%s\n", Where(whereConditions...))
	}
	if len(groupColumns) > 0 {
		fmt.Fprintf(b, "\t\tGROUP BY %s", strings.Join(groupColumns, ", "))
	}
}

func (agg *aggregateDescription) aggregateQuery(
	subquery string,
	subqueryName string,
	aggregateTimestamp string,
) string {
	var b strings.Builder
	agg.aggregateQueryBuilder(&b, subquery, subqueryName, aggregateTimestamp, nil, agg.groupColumns(true))
	return b.String()
}

func (agg *aggregateDescription) createContinuousView(b io.Writer, period IntervalDescription) {
	fmt.Fprint(b, `
		CREATE MATERIALIZED VIEW midgard_agg.`+agg.name+`_`+period.name+`
		WITH (timescaledb.continuous) AS
		`)
	bucketField := fmt.Sprintf("time_bucket('%d', block_timestamp)", period.minDuration*1e9)
	fmt.Fprint(b, agg.baseQuery(bucketField))
	fmt.Fprint(b, `
		WITH NO DATA;
	`)
}

func (agg *aggregateDescription) createHigherView(b io.Writer, period string) {
	fmt.Fprint(b, `
		CREATE VIEW midgard_agg.`+agg.name+`_`+period+` AS
		`)
	fmt.Fprint(b, agg.aggregateQuery("midgard_agg."+agg.name+"_day", "d",
		"nano_trunc('"+period+"', d.aggregate_timestamp)"))
	fmt.Fprint(b, ";\n")
}

func (agg *aggregateDescription) createViews(b io.Writer) {
	for _, bucket := range intervals {
		if bucket.exact {
			agg.createContinuousView(b, bucket)
		} else {
			agg.createHigherView(b, bucket.name)
		}
	}
}

// TODO(huginn): move this to buckets
func TimeBucketCeil(time Nano, period Nano) Nano {
	return (time + period - 1) / period * period
}

// TODO(huginn): move this to buckets
func TimeBucketFloor(time Nano, period Nano) Nano {
	return time / period * period
}

// Returns a UNION query for the aggregate that is suitable for aggregating over large and/or
// non-bucket aligned time intervals.
//
// The query is intended to for creating aggregates over a single interval, so the `aggregate_time`
// column should be dropped in the final query, but it is provided so that `last()` aggregates
// can be computed. For this, the timestamp needs to be aggregated in an arbitrary order-preserving
// way, so we just use `MIN`.
//
// If `timeLow <= 0` the lower bound is omitted
func (agg *aggregateDescription) UnionQuery(timeLow Nano, timeHigh Nano, whereConditions []string, params []interface{}) (string, []interface{}) {
	var b strings.Builder
	fmt.Fprint(&b, "(\n")

	var timeLowCeil Nano
	var timeLowCeilParam int
	if timeLow > 0 {
		timeLowCeil = TimeBucketCeil(timeLow, 3600e9)
		params = append(params, timeLowCeil)
		timeLowCeilParam = len(params)
		if timeLowCeil != timeLow {
			params = append(params, timeLow)
			timeLowParam := len(params)
			fmt.Fprint(&b, "\t\t(")
			agg.baseQueryBuilder(
				&b,
				"MIN(block_timestamp)",
				append(
					whereConditions,
					fmt.Sprintf("$%d <= block_timestamp", timeLowParam),
					fmt.Sprintf("block_timestamp < $%d", timeLowCeilParam)),
				agg.groupColumns(false),
			)
			fmt.Fprint(&b, ")\n\tUNION ALL\n")
		}
	}

	timeHighFloor := TimeBucketFloor(timeHigh, 3600e9)
	params = append(params, timeHighFloor)
	timeHighFloorParam := len(params)
	if timeHigh != timeHighFloor {
		params = append(params, timeHigh)
		timeHighParam := len(params)
		fmt.Fprint(&b, "\t\t(")
		agg.baseQueryBuilder(
			&b,
			"MIN(block_timestamp)",
			append(
				whereConditions,
				fmt.Sprintf("$%d <= block_timestamp", timeHighFloorParam),
				fmt.Sprintf("block_timestamp < $%d", timeHighParam)),
			agg.groupColumns(false),
		)
		fmt.Fprint(&b, ")\n\tUNION ALL\n")
	}

	fmt.Fprint(&b, "\t\t(")
	conds := append(whereConditions, fmt.Sprintf("h.aggregate_timestamp < $%d", timeHighFloorParam))
	if timeLow > 0 {
		conds = append(conds, fmt.Sprintf("$%d <= h.aggregate_timestamp", timeLowCeilParam))
	}
	agg.aggregateQueryBuilder(
		&b,
		"midgard_agg."+agg.name+"_hour",
		"h",
		"MIN(h.aggregate_timestamp)",
		conds,
		agg.groupColumns(false),
	)
	fmt.Fprint(&b, ")\n")

	fmt.Fprint(&b, ")")
	return b.String(), params
}

// Returns a query that aggregates over the provided `buckets`.
//
// The `template` should be a query template with a single %s after FROM.
//
// This is either a simple SELECT from the appropriate (materialized) view if `buckets` are regular
// periodic buckets, or a `UnionQuery` if it's just an arbitrary interval.
func (agg *aggregateDescription) BucketedQuery(template string,
	buckets Buckets,
	whereConditions []string,
	params []interface{},
) (string, []interface{}) {
	var b strings.Builder

	fmt.Fprint(&b, "(")

	if buckets.OneInterval() {
		var unionQ string
		unionQ, params = agg.UnionQuery(buckets.Start().ToNano(), buckets.End().ToNano(), whereConditions, params)
		params = append(params, buckets.Start().ToNano())
		startTimestamp := fmt.Sprintf("$%d::BIGINT", len(params))
		agg.aggregateQueryBuilder(&b, unionQ, "uni", startTimestamp, nil, agg.groupColumns(false))
	} else {
		fmt.Fprintf(&b, "SELECT * FROM midgard_agg.%s_%s ", agg.name, buckets.AggregateName())
		params = append(params, buckets.Start().ToNano())
		where := append(whereConditions, fmt.Sprintf("$%d <= aggregate_timestamp", len(params)))
		params = append(params, buckets.End().ToNano())
		where = append(where, fmt.Sprintf("aggregate_timestamp < $%d", len(params)))
		fmt.Fprint(&b, Where(where...))
	}

	fmt.Fprint(&b, ") AS bucketed")

	return fmt.Sprintf(template, b.String()), params
}

////////////////////////////////////////////////////////////////////////////////////////////////////

var aggregates = map[string]*aggregateDescription{}

func RegisterAggregate(agg *aggregateDescription) *aggregateDescription {
	aggregates[agg.name] = agg
	return agg
}

var watermarkedMaterializedViews = map[string]string{}

func RegisterWatermarkedMaterializedView(name string, query string) {
	watermarkedMaterializedViews[name] = query
}

func AggregatesDdl() string {
	var b strings.Builder
	fmt.Fprint(&b, `
		-- version 1

		DROP SCHEMA IF EXISTS midgard_agg CASCADE;
		CREATE SCHEMA midgard_agg;

		CREATE TABLE midgard_agg.watermarks (
			materialized_table VARCHAR(60) PRIMARY KEY,
			watermark BIGINT NOT NULL
		);

		CREATE FUNCTION midgard_agg.watermark(t VARCHAR) RETURNS BIGINT
		LANGUAGE SQL STABLE AS $$
			SELECT watermark FROM midgard_agg.watermarks
			WHERE materialized_table = t;
		$$;

		CREATE PROCEDURE midgard_agg.refresh_watermarked_view(t VARCHAR, w_new BIGINT)
		LANGUAGE plpgsql AS $BODY$
		DECLARE
			w_old BIGINT;
		BEGIN
			SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = t
				FOR UPDATE INTO w_old;
			EXECUTE format($$
				INSERT INTO midgard_agg.%1$I_materialized
				SELECT * from midgard_agg.%1$I
					WHERE $1 <= block_timestamp AND block_timestamp < $2
			$$, t) USING w_old, w_new;
			UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = t;
		END
		$BODY$;
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
		aggregate.createViews(&b)
	}

	// Sort to iterate in deterministic order.
	// We need this to avoid unnecessarily recreating the 'aggregate' schema.
	watermarkedNames := make([]string, 0, len(watermarkedMaterializedViews))
	for name := range watermarkedMaterializedViews {
		watermarkedNames = append(watermarkedNames, name)
	}
	sort.Strings(watermarkedNames)

	for _, name := range watermarkedNames {
		query := watermarkedMaterializedViews[name]
		fmt.Fprintf(&b, `
			CREATE VIEW midgard_agg.`+name+` AS
			`+query+`;
			-- TODO(huginn): should this be a hypertable?
			CREATE TABLE midgard_agg.`+name+`_materialized (LIKE midgard_agg.`+name+`);
			CREATE INDEX ON midgard_agg.`+name+`_materialized (block_timestamp);
			INSERT INTO midgard_agg.watermarks (materialized_table, watermark)
			VALUES ('`+name+`', 0);

			CREATE VIEW midgard_agg.`+name+`_combined AS
				SELECT * from midgard_agg.`+name+`_materialized
				WHERE block_timestamp < midgard_agg.watermark('`+name+`')
			UNION ALL
				SELECT * from midgard_agg.`+name+`
				WHERE midgard_agg.watermark('`+name+`') <= block_timestamp;
		`)
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

	lastBlockTimestamp := LastBlockTimestamp()

	refreshEnd := lastBlockTimestamp - 5*60*1e9
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
			_, err := theDB.ExecContext(ctx, q)
			if err != nil {
				log.Error().Err(err).Msgf("Refreshing %s_%s", name, bucket.name)
			}
		}
	}

	for name := range watermarkedMaterializedViews {
		q := fmt.Sprintf("CALL midgard_agg.refresh_watermarked_view('%s', '%d')",
			name, lastBlockTimestamp)
		_, err := theDB.Exec(q)
		if err != nil {
			log.Error().Err(err).Msgf("Refreshing %s", name)
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