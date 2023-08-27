INSERT INTO midgard_agg.watermarks (materialized_table, watermark)
    VALUES ('balances', 0);

CREATE VIEW midgard_agg.balance_deltas AS (
    SELECT to_addr AS addr, asset, amount_e8, block_timestamp FROM transfer_events
    UNION ALL
    SELECT from_addr AS addr, asset, -amount_e8 AS amount_e8, block_timestamp FROM transfer_events
);

-- TODO(freki): add indices when serving code is done.
CREATE TABLE midgard_agg.balances (
    addr text NOT NULL,
    asset text NOT NULL,
    amount_e8 bigint NOT NULL,
    block_timestamp bigint NOT NULL,
    PRIMARY KEY(addr, asset, block_timestamp)
);

-- This table is UPDATE heavy, that's why we change the `fillfactor` (from the default 100%).
-- TODO(huginn): investigate what table/index fillfactor results in the best performance.
CREATE TABLE midgard_agg.current_balances (
    addr text NOT NULL,
    asset text NOT NULL,
    amount_e8 bigint NOT NULL,
    PRIMARY KEY(addr, asset)
)
WITH (fillfactor = 90);

CREATE PROCEDURE midgard_agg.update_current_balances_interval(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    INSERT INTO midgard_agg.current_balances AS cb (
        SELECT addr, asset, SUM(amount_e8) AS amount_e8
        FROM midgard_agg.balance_deltas
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        GROUP BY addr, asset
    )
    ON CONFLICT (addr, asset) DO UPDATE SET amount_e8 = cb.amount_e8 + EXCLUDED.amount_e8;
END
$BODY$;

CREATE PROCEDURE midgard_agg.update_running_balances_interval(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    -- This `EXECUTE` trick is needed so that PostgreSQL replans the query according to specific
    -- values of `t1` and `t2`. Otherwise it uses the same plan, which is suboptimal, especially
    -- for short intervals.
    EXECUTE $$
    WITH
    -- Slice of the transfer events we are processing now.
    balance_deltas_slice AS (
        SELECT * FROM midgard_agg.balance_deltas
        WHERE $1 <= block_timestamp AND block_timestamp < $2
    ),
    -- Current balances for the slice; to start the running totals from.
    current_balances_slice AS (
        SELECT
            addr,
            asset,
            amount_e8,
            0 AS block_timestamp
        FROM midgard_agg.current_balances
        WHERE (addr, asset) IN (SELECT addr, asset FROM balance_deltas_slice GROUP BY addr, asset)
    ),
    -- Aggregate all balance changes within a block. We are only interested in the balance at
    -- the end of the block.
    block_balance_deltas AS (
        SELECT
            addr,
            asset,
            SUM(amount_e8) AS amount_e8,
            block_timestamp
        FROM balance_deltas_slice
        GROUP BY addr, asset, block_timestamp
    ),
    -- Combined table to calculate the running totals on the current slice.
    balance_deltas_with_initial AS (
        SELECT * FROM current_balances_slice
        UNION ALL
        SELECT * FROM block_balance_deltas
    )
    INSERT INTO midgard_agg.balances (
        SELECT * FROM (
            SELECT
                addr,
                asset,
                SUM(amount_e8) OVER (PARTITION BY addr, asset ORDER BY block_timestamp) AS amount_e8,
                block_timestamp
            FROM balance_deltas_with_initial
        ) AS x
        WHERE block_timestamp > 0
    )
    $$ USING t1, t2;
END
$BODY$;

CREATE PROCEDURE midgard_agg.update_balances_interval(t1 bigint, t2 bigint)
LANGUAGE SQL AS $BODY$
    CALL midgard_agg.update_running_balances_interval(t1, t2);
    CALL midgard_agg.update_current_balances_interval(t1, t2);
$BODY$;

CREATE PROCEDURE midgard_agg.update_balances(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = 'balances'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating balances into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL midgard_agg.update_balances_interval(w_old, w_new);
    UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = 'balances';
END
$BODY$;
