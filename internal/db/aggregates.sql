
-- version 1

DROP SCHEMA IF EXISTS midgard_agg CASCADE;
CREATE SCHEMA midgard_agg;

CREATE VIEW midgard_agg.pending_adds AS
SELECT *
FROM pending_liquidity_events AS p
WHERE pending_type = 'add'
    AND NOT EXISTS(
        -- Filter out pending liquidity which was already added
        SELECT *
        FROM stake_events AS s
        WHERE
            p.rune_addr = s.rune_addr
            AND p.pool = s.pool
            AND p.block_timestamp <= s.block_timestamp)
    AND NOT EXISTS(
        -- Filter out pending liquidity which was withdrawn without adding
        SELECT *
        FROM pending_liquidity_events AS pw
        WHERE
            pw.pending_type = 'withdraw'
            AND p.rune_addr = pw.rune_addr
            AND p.pool = pw.pool
            AND p.block_timestamp <= pw.block_timestamp);

CREATE TABLE midgard_agg.watermarks (
    materialized_table varchar PRIMARY KEY,
    watermark bigint NOT NULL
);

CREATE FUNCTION midgard_agg.watermark(t varchar) RETURNS bigint
LANGUAGE SQL STABLE AS $$
    SELECT watermark FROM midgard_agg.watermarks
    WHERE materialized_table = t;
$$;

CREATE PROCEDURE midgard_agg.refresh_watermarked_view(t varchar, w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = t
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating % into past: % -> %', t, w_old, w_new;
        RETURN;
    END IF;
    EXECUTE format($$
        INSERT INTO midgard_agg.%1$I_materialized
        SELECT * from midgard_agg.%1$I
            WHERE $1 <= block_timestamp AND block_timestamp < $2
    $$, t) USING w_old, w_new;
    UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = t;
END
$BODY$;

-------------------------------------------------------------------------------
-- Thorname
-------------------------------------------------------------------------------

-- TODO(muninn): replace with indexing time materialized table, a full select is 100ms.
CREATE VIEW midgard_agg.thorname_owner_expiration AS
    SELECT DISTINCT ON (name)
        name,
        owner,
        expire
    FROM thorname_change_events
    ORDER BY name, block_timestamp DESC;

CREATE VIEW midgard_agg.thorname_current_state AS
    SELECT DISTINCT ON (name, chain)
        change_events.name,
        change_events.chain,
        change_events.address,
        owner_expiration.owner,
        owner_expiration.expire
    FROM thorname_change_events AS change_events
    JOIN midgard_agg.thorname_owner_expiration AS owner_expiration
        ON owner_expiration.name = change_events.name
    ORDER BY name, chain, block_timestamp DESC;

-------------------------------------------------------------------------------
-- Actions
-------------------------------------------------------------------------------

--
-- Main table and its indices
--

CREATE TABLE midgard_agg.actions (
    event_id            bigint NOT NULL,
    block_timestamp     bigint NOT NULL,
    action_type         text NOT NULL,
    main_ref            text,
    addresses           text[] NOT NULL,
    transactions        text[] NOT NULL,
    assets              text[] NOT NULL,
    pools               text[],
    ins                 jsonb NOT NULL,
    outs                jsonb NOT NULL,
    fees                jsonb NOT NULL,
    meta                jsonb
);
-- TODO(huginn): should it be a hypertable? Measure both ways and decide!

CREATE INDEX ON midgard_agg.actions (event_id DESC);
CREATE INDEX ON midgard_agg.actions (action_type, event_id DESC);
CREATE INDEX ON midgard_agg.actions (main_ref, event_id DESC);
CREATE INDEX ON midgard_agg.actions (block_timestamp, event_id DESC);

CREATE INDEX ON midgard_agg.actions USING gin (addresses);
CREATE INDEX ON midgard_agg.actions USING gin (transactions);
CREATE INDEX ON midgard_agg.actions USING gin (assets);
CREATE INDEX ON midgard_agg.actions USING gin ((meta -> 'affiliateAddress'));

--
-- Functions for actions aggregates
--

CREATE FUNCTION midgard_agg.check_synth(ta text[]) RETURNS boolean
LANGUAGE plpgsql AS $BODY$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ta
    LOOP
        IF t ~ '/'  THEN
            RETURN TRUE; 
        END IF;
    END LOOP;
    RETURN FALSE;
END
$BODY$;

CREATE FUNCTION midgard_agg.check_no_rune(ta text[]) RETURNS boolean
LANGUAGE plpgsql AS $BODY$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ta
    LOOP
        IF t='THOR.RUNE' THEN
            RETURN FALSE; 
        END IF;
    END LOOP;
    RETURN TRUE;
END
$BODY$;

CREATE FUNCTION midgard_agg.check_derived(ta text[]) RETURNS boolean
LANGUAGE plpgsql AS $BODY$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ta
    LOOP
        IF t ~ 'THOR\.(?!RUNE).+'  THEN
            RETURN TRUE; 
        END IF;
    END LOOP;
    RETURN FALSE;
END
$BODY$;

CREATE FUNCTION midgard_agg.add_asset_types(ta text[]) RETURNS text[]
LANGUAGE plpgsql AS $BODY$
DECLARE
    t text[] := ta;
BEGIN
    IF midgard_agg.check_synth(ta) THEN
        t := array_append(t, 'synth');
    ELSE 
        t := array_append(t, 'nosynth');
    END IF;
    IF midgard_agg.check_no_rune(ta) THEN
        t := array_append(t, 'norune');
    END IF;
    IF midgard_agg.check_derived(ta) THEN
        t := array_append(t, 'derived');
    END IF;
    RETURN t;
END
$BODY$;

CREATE FUNCTION midgard_agg.out_tx(
    txid text,
    address text,
    height text,
    internal boolean,
    VARIADIC coins coin_rec[]
) RETURNS jsonb 
LANGUAGE plpgsql AS $BODY$
DECLARE
    ret jsonb;
BEGIN
    ret := jsonb_build_object('txID', txid, 'address', address, 'coins', coins(VARIADIC coins));
    IF height IS NOT NULL THEN
        ret := ret || jsonb_build_object('height', height);
    END IF;
    IF internal IS NOT NULL THEN
        ret := ret || jsonb_build_object('internal', internal);
    END IF;

    RETURN ret;
END
$BODY$;

--
-- Basic VIEWs that build actions
--

CREATE VIEW midgard_agg.switch_actions AS
    SELECT
        event_id,
        block_timestamp,
        'switch' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        non_null_array(tx) AS transactions,
        ARRAY[burn_asset, 'THOR.RUNE'] :: text[] AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (burn_asset, burn_e8))) AS ins,
        jsonb_build_array(mktransaction(NULL, to_addr, ('THOR.RUNE', mint_e8))) AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM switch_events;

CREATE VIEW midgard_agg.refund_actions AS
    SELECT
        event_id,
        block_timestamp,
        'refund' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        non_null_array(asset, asset_2nd) AS assets,
        NULL :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (asset, asset_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'reason', reason,
            'memo', memo,
            'affiliateFee', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){4}(\d{1,5}?)(?::|$)')::int 
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){3}(\d{1,5}?)(?::|$)')::int
                ELSE NULL
            END,
            'affiliateAddress', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN 
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){3}([^:]+)') 
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){2}([^:]+)')
                ELSE NULL
            END
            ) AS meta
    FROM refund_events;

CREATE VIEW midgard_agg.donate_actions AS
    SELECT
        event_id,
        block_timestamp,
        'donate' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        CASE WHEN rune_e8 > 0 THEN ARRAY[asset, 'THOR.RUNE']
            ELSE ARRAY[asset] END :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (asset, asset_e8),
            ('THOR.RUNE', rune_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        NULL :: jsonb AS meta
    FROM add_events;

CREATE VIEW midgard_agg.withdraw_actions AS
    SELECT
        event_id,
        block_timestamp,
        'withdraw' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        CASE WHEN
            midgard_agg.check_synth(ARRAY[pool]) 
            THEN ARRAY[pool, 'synth'] 
            ELSE ARRAY[pool, 'nosynth'] END :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (asset, asset_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'asymmetry', asymmetry,
            'basisPoints', basis_points,
            'impermanentLossProtection', imp_loss_protection_e8,
            'liquidityUnits', -stake_units,
            'emitAssetE8', emit_asset_e8,
            'emitRuneE8', emit_rune_e8,
            'memo', memo
            ) AS meta
    FROM withdraw_events;

-- TODO(huginn): use _direction for join
CREATE VIEW midgard_agg.swap_actions AS
    -- Single swap (unique txid)
    SELECT
        event_id,
        block_timestamp,
        'swap' AS action_type,
        tx :: text AS main_ref,
        ARRAY[from_addr, to_addr] :: text[] AS addresses,
        ARRAY[tx] :: text[] AS transactions,
        midgard_agg.add_asset_types(ARRAY[from_asset, to_asset]) :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        jsonb_build_array(mktransaction(tx, from_addr, (from_asset, from_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'swapSingle', TRUE,
            'liquidityFee', liq_fee_in_rune_e8,
            'swapTarget', to_e8_min,
            'swapSlip', swap_slip_bp,
            'memo', memo,
            'affiliateFee', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){4}(\d{1,5}?)(?::|$)')::int 
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){3}(\d{1,5}?)(?::|$)')::int
                ELSE NULL
            END,
            'affiliateAddress', CASE
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN 
                    SUBSTRING(memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){3}([^:]+)') 
                WHEN SUBSTRING(memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){2}([^:]+)')
                ELSE NULL
            END
            ) AS meta
    FROM swap_events AS single_swaps
    WHERE NOT EXISTS (
        SELECT tx FROM swap_events
        WHERE block_timestamp = single_swaps.block_timestamp AND tx = single_swaps.tx
            AND from_asset <> single_swaps.from_asset
    )
    UNION ALL
    -- Double swap (same txid in different pools)
    SELECT
        swap_in.event_id,
        swap_in.block_timestamp,
        'swap' AS action_type,
        swap_in.tx :: text AS main_ref,
        ARRAY[swap_in.from_addr, swap_in.to_addr] :: text[] AS addresses,
        ARRAY[swap_in.tx] :: text[] AS transactions,
        midgard_agg.add_asset_types(ARRAY[swap_in.from_asset, swap_out.to_asset]) :: text[] AS assets,
        CASE WHEN swap_in.pool <> swap_out.pool THEN ARRAY[swap_in.pool, swap_out.pool]
            ELSE ARRAY[swap_in.pool] END :: text[] AS pools,
        jsonb_build_array(mktransaction(swap_in.tx, swap_in.from_addr,
            (swap_in.from_asset, swap_in.from_e8))) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'swapSingle', FALSE,
            'liquidityFee', swap_in.liq_fee_in_rune_e8 + swap_out.liq_fee_in_rune_e8,
            'swapTarget', swap_out.to_e8_min,
            'swapSlip', swap_in.swap_slip_BP + swap_out.swap_slip_BP
                - swap_in.swap_slip_BP*swap_out.swap_slip_BP/10000,
            'memo', swap_in.memo,
            'affiliateFee', CASE
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN
                    SUBSTRING(swap_in.memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){4}(\d{1,5}?)(?::|$)')::int 
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(swap_in.memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){3}(\d{1,5}?)(?::|$)')::int
                ELSE NULL
            END,
            'affiliateAddress', CASE
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{SWAP,s,=}') THEN 
                    SUBSTRING(swap_in.memo FROM '^(?:=|SWAP|[s]):(?:[^:]*:){3}([^:]+)') 
                WHEN SUBSTRING(swap_in.memo FROM '^(.*?):')::text = ANY('{ADD,a,+}') THEN
                    SUBSTRING(swap_in.memo FROM '^(?:ADD|[+]|a):(?:[^:]*:){2}([^:]+)')
                ELSE NULL
            END,
            'outRuneE8',
                swap_in.to_e8
            ) AS meta
    FROM swap_events AS swap_in
    INNER JOIN swap_events AS swap_out
    ON swap_in.tx = swap_out.tx AND swap_in.block_timestamp = swap_out.block_timestamp
    WHERE swap_in.from_asset <> swap_out.to_asset AND swap_in.to_e8 = swap_out.from_e8
        AND swap_in.to_asset = 'THOR.RUNE' AND swap_out.from_asset = 'THOR.RUNE' 
    ;

CREATE VIEW midgard_agg.addliquidity_actions AS
    SELECT
        event_id,
        block_timestamp,
        'addLiquidity' AS action_type,
        NULL :: text AS main_ref,
        non_null_array(rune_addr, asset_addr) AS addresses,
        non_null_array(rune_tx, asset_tx) AS transactions,
        CASE WHEN
            midgard_agg.check_synth(ARRAY[pool]) 
            THEN ARRAY[pool, 'synth'] 
            ELSE ARRAY[pool, 'THOR.RUNE', 'nosynth'] END :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        transaction_list(
            mktransaction(rune_tx, rune_addr, ('THOR.RUNE', rune_e8)),
            mktransaction(asset_tx, asset_addr, (pool, asset_e8))
            ) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object(
            'status', 'success',
            'liquidityUnits', stake_units
            ) AS meta
    FROM stake_events
    UNION ALL
    -- Pending `add`s will be removed when not pending anymore
    SELECT
        event_id,
        block_timestamp,
        'addLiquidity' AS action_type,
        'PL:' || rune_addr || ':' || pool :: text AS main_ref,
        non_null_array(rune_addr, asset_addr) AS addresses,
        non_null_array(rune_tx, asset_tx) AS transactions,
        ARRAY[pool, 'THOR.RUNE'] :: text[] AS assets,
        ARRAY[pool] :: text[] AS pools,
        transaction_list(
            mktransaction(rune_tx, rune_addr, ('THOR.RUNE', rune_e8)),
            mktransaction(asset_tx, asset_addr, (pool, asset_e8))
            ) AS ins,
        jsonb_build_array() AS outs,
        jsonb_build_array() AS fees,
        jsonb_build_object('status', 'pending') AS meta
    FROM pending_liquidity_events
    WHERE pending_type = 'add'
    ;

--
-- Procedures for updating actions
--

CREATE PROCEDURE midgard_agg.insert_actions(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    EXECUTE $$ INSERT INTO midgard_agg.actions
    SELECT * FROM midgard_agg.switch_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 $$ USING t1, t2;

    EXECUTE $$ INSERT INTO midgard_agg.actions
    SELECT * FROM midgard_agg.refund_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 $$ USING t1, t2;

    EXECUTE $$ INSERT INTO midgard_agg.actions
    SELECT * FROM midgard_agg.donate_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 $$ USING t1, t2;

    EXECUTE $$ INSERT INTO midgard_agg.actions
    SELECT * FROM midgard_agg.withdraw_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 $$ USING t1, t2;

    EXECUTE $$ INSERT INTO midgard_agg.actions
    SELECT * FROM midgard_agg.swap_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 $$ USING t1, t2;

    EXECUTE $$ INSERT INTO midgard_agg.actions
    SELECT * FROM midgard_agg.addliquidity_actions
        WHERE $1 <= block_timestamp AND block_timestamp < $2 $$ USING t1, t2;
END
$BODY$;

-- TODO(muninn): Check the pending logic regarding nil rune address
CREATE PROCEDURE midgard_agg.trim_pending_actions(t1 bigint, t2 bigint)
LANGUAGE SQL AS $BODY$
    DELETE FROM midgard_agg.actions AS a
    USING stake_events AS s
    WHERE
        t1 <= s.block_timestamp AND s.block_timestamp < t2
        AND a.event_id <= s.event_id
        AND a.main_ref = 'PL:' || s.rune_addr || ':' || s.pool;

    DELETE FROM midgard_agg.actions AS a
    USING pending_liquidity_events AS pw
    WHERE
        t1 <= pw.block_timestamp AND pw.block_timestamp < t2
        AND a.event_id <= pw.event_id
        AND pw.pending_type = 'withdraw'
        AND a.main_ref = 'PL:' || pw.rune_addr || ':' || pw.pool;
$BODY$;

-- TODO(huginn): Remove duplicates from these lists?
CREATE PROCEDURE midgard_agg.actions_add_outbounds(t1 bigint, t2 bigint)
LANGUAGE SQL AS $BODY$
    UPDATE outbound_events as o
    SET
        internal = TRUE
    FROM (
        SELECT
            main_ref,
            action_type,
            (meta ->> 'outRuneE8')::bigint as out_e8
        FROM midgard_agg.actions
        WHERE t1 <= block_timestamp AND block_timestamp < t2 
        ) as a
    WHERE 
        o.in_tx = a.main_ref AND 
        a.out_e8 = o.asset_e8 AND 
        o.asset = 'THOR.RUNE' AND 
        a.action_type = 'swap'
    ;

    UPDATE midgard_agg.actions AS a
    SET
        addresses = a.addresses || o.froms || o.tos,
        transactions = a.transactions || array_remove(o.transactions, NULL),
        assets = a.assets || o.assets,
        outs = a.outs || o.outs
    FROM (
        SELECT
            in_tx,
            array_agg(from_addr :: text) AS froms,
            array_agg(to_addr :: text) AS tos,
            array_agg(tx :: text) AS transactions,
            array_agg(asset :: text) AS assets,
            jsonb_agg(midgard_agg.out_tx(tx, to_addr, TRUNC(event_id / 1e10)::text, internal, (asset, asset_e8))) AS outs
        FROM outbound_events
        WHERE t1 <= block_timestamp AND block_timestamp < t2 AND internal IS NOT TRUE
        GROUP BY in_tx
        ) AS o
    WHERE
        o.in_tx = a.main_ref;
$BODY$;

CREATE PROCEDURE midgard_agg.actions_add_fees(t1 bigint, t2 bigint)
LANGUAGE SQL AS $BODY$
    UPDATE midgard_agg.actions AS a
    SET
        fees = a.fees || f.fees
    FROM (
        SELECT
            tx,
            jsonb_agg(jsonb_build_object('asset', asset, 'amount', asset_e8)) AS fees
        FROM fee_events
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        GROUP BY tx
        ) AS f
    WHERE
        f.tx = a.main_ref;
$BODY$;

CREATE PROCEDURE midgard_agg.update_actions_interval(t1 bigint, t2 bigint)
LANGUAGE SQL AS $BODY$
    CALL midgard_agg.insert_actions(t1, t2);
    CALL midgard_agg.trim_pending_actions(t1, t2);
    CALL midgard_agg.actions_add_outbounds(t1, t2);
    CALL midgard_agg.actions_add_fees(t1, t2);
$BODY$;

INSERT INTO midgard_agg.watermarks (materialized_table, watermark)
    VALUES ('actions', 0);

CREATE PROCEDURE midgard_agg.update_actions(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = 'actions'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating actions into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL midgard_agg.update_actions_interval(w_old, w_new);
    UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = 'actions';
END
$BODY$;
