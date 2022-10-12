SET "v.p" = 'BCH.BCH';
SET "v.t" = 1646694000000000000;

DROP VIEW IF EXISTS midgard_agg.lpdiff;

CREATE VIEW midgard_agg.lpdiff AS
SELECT
  pool, block_timestamp, sum(lpdiff) as lpdiff
FROM
  (select pool, stake_units as lpdiff, block_timestamp from deposit_events
  UNION
  select pool, -stake_units as lpdiff, block_timestamp from withdraw_events) AS x
GROUP BY pool, block_timestamp
;

\COPY (select block_timestamp, 'swap' as kind, _direction as direction, from_e8, to_e8, liq_fee_e8, liq_fee_in_rune_e8  from swap_events where _direction > 1 and block_timestamp > current_setting('v.t')::BIGINT and pool =  current_setting('v.p') ORDER BY block_timestamp ASC) TO 'tmp/export/swaps.csv' WITH CSV;

\COPY (SELECT block_timestamp, 'units' as kind, SUM(lpdiff) over (PARTITION BY pool ORDER BY block_timestamp) FROM midgard_agg.lpdiff WHERE pool = current_setting('v.p') ORDER BY block_timestamp) TO 'tmp/export/units.csv' WITH CSV;

\copy (SELECT block_timestamp, 'depth' as kind, asset_e8, rune_e8, synth_e8 FROM block_pool_depths WHERE pool = current_setting('v.p') AND block_timestamp > current_setting('v.t')::bigint ORDER BY block_timestamp) TO 'tmp/export/depths.csv' WITH CSV;
