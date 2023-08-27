INSERT INTO midgard_agg.watermarks (materialized_table, watermark)
    VALUES ('members', 0);

CREATE TABLE midgard_agg.members_log (
    member_id text NOT NULL,
    pool text NOT NULL,
    change_type text NOT NULL, -- add, withdraw, pending_add, pending_withdraw
    basis_points bigint, -- only withraws have basis points, for other change types it's NULL
    lp_units_delta bigint NOT NULL,
    lp_units_total bigint NOT NULL,
    -- asset fields
    asset_addr text,
    asset_e8_delta bigint,
    pending_asset_e8_delta bigint,
    pending_asset_e8_total bigint NOT NULL,
    asset_tx text,
    -- rune fields
    rune_addr text,
    rune_e8_delta bigint,
    pending_rune_e8_delta bigint,
    pending_rune_e8_total bigint NOT NULL,
    rune_tx text,
    --
    event_id bigint NOT NULL,
    block_timestamp bigint NOT NULL
);

-- Intended to be inserted into `members_log` with the totals and other missing info filled out
-- by the trigger.
CREATE VIEW midgard_agg.members_log_partial AS (
    SELECT * FROM (
        SELECT
            COALESCE(rune_addr, asset_addr) AS member_id,
            pool,
            'add' AS change_type,
            NULL::bigint AS basis_points,
            stake_units AS lp_units_delta,
            NULL::bigint AS lp_units_total,
            asset_addr,
            asset_e8 AS asset_e8_delta,
            NULL::bigint AS pending_asset_e8_delta,
            NULL::bigint AS pending_asset_e8_total,
            asset_tx,
            rune_addr,
            rune_e8 AS rune_e8_delta,
            NULL::bigint AS pending_rune_e8_delta,
            NULL::bigint AS pending_rune_e8_total,
            rune_tx,
            event_id,
            block_timestamp
        FROM stake_events
        UNION ALL
        SELECT
            from_addr AS member_id,
            pool,
            'withdraw' AS change_type,
            basis_points AS basis_points,
            -stake_units AS lp_units_delta,
            NULL::bigint AS lp_units_total,
            NULL AS asset_addr,
            -emit_asset_e8 AS asset_e8_delta,
            NULL::bigint AS pending_asset_e8_delta,
            NULL::bigint AS pending_asset_e8_total,
            NULL AS asset_tx,
            NULL AS rune_addr,
            -emit_rune_e8 AS rune_e8_delta,
            NULL::bigint AS pending_rune_e8_delta,
            NULL::bigint AS pending_rune_e8_total,
            NULL AS rune_tx,
            event_id,
            block_timestamp
        FROM withdraw_events
        UNION ALL
        SELECT
            COALESCE(rune_addr, asset_addr) AS member_id,
            pool,
            'pending_' || pending_type AS change_type,
            NULL::bigint AS basis_points,
            0 AS lp_units_delta,
            NULL::bigint AS lp_units_total,
            asset_addr,
            NULL::bigint AS asset_e8_delta,
            CASE WHEN pending_type = 'add' THEN asset_e8 ELSE -asset_e8 END AS pending_asset_e8_delta,
            NULL::bigint AS pending_asset_e8_total,
            asset_tx,
            rune_addr,
            NULL::bigint AS rune_e8_delta,
            CASE WHEN pending_type = 'add' THEN rune_e8 ELSE -rune_e8 END AS pending_rune_e8_delta,
            NULL::bigint AS pending_rune_e8_total,
            rune_tx,
            event_id,
            block_timestamp
        FROM pending_liquidity_events
    ) AS x
    ORDER BY block_timestamp, change_type
);

CREATE TABLE midgard_agg.members (
    member_id text NOT NULL,
    pool text NOT NULL,
    lp_units_total bigint NOT NULL,
    -- asset fields
    asset_addr text,
    asset_e8_deposit bigint NOT NULL,
    added_asset_e8_total bigint NOT NULL,
    withdrawn_asset_e8_total bigint NOT NULL,
    pending_asset_e8_total bigint NOT NULL,
    -- rune fields
    rune_addr text,
    rune_e8_deposit bigint NOT NULL,
    added_rune_e8_total bigint NOT NULL,
    withdrawn_rune_e8_total bigint NOT NULL,
    pending_rune_e8_total bigint NOT NULL,
    --
    first_added_timestamp bigint,
    last_added_timestamp bigint,
    PRIMARY KEY (member_id, pool)
)
WITH (fillfactor = 90);

CREATE INDEX ON midgard_agg.members (asset_addr);

CREATE TABLE midgard_agg.members_count (
    pool text NOT NULL,
    count bigint NOT NULL,
    block_timestamp bigint NOT NULL,
    PRIMARY KEY (pool, block_timestamp)
);

CREATE INDEX ON midgard_agg.members_count (pool, block_timestamp DESC);

CREATE FUNCTION midgard_agg.add_members_log() RETURNS trigger
LANGUAGE plpgsql AS $BODY$
DECLARE
    member midgard_agg.members%ROWTYPE;
BEGIN
    -- Fix Ethereum addresses to be uniformly lowercase
    -- TODO(huginn): fix this on the event parsing/recording level
    IF NEW.pool LIKE 'ETH.%' THEN
        NEW.asset_addr = lower(NEW.asset_addr);
        IF lower(NEW.member_id) = NEW.asset_addr THEN
            NEW.member_id = lower(NEW.member_id);
        END IF;
    END IF;

    -- Look up the current state of the member
    SELECT * FROM midgard_agg.members
        WHERE member_id = NEW.member_id AND pool = NEW.pool
        FOR UPDATE INTO member;

    -- If this is a new member, fill out its fields
    IF member.member_id IS NULL THEN
        member.member_id = NEW.member_id;
        member.pool = NEW.pool;
        member.asset_addr = NEW.asset_addr;
        member.rune_addr = NEW.rune_addr;
        member.lp_units_total = 0;
        member.added_asset_e8_total = 0;
        member.withdrawn_asset_e8_total = 0;
        member.pending_asset_e8_total = 0;
        member.added_rune_e8_total = 0;
        member.withdrawn_rune_e8_total = 0;
        member.pending_rune_e8_total = 0;
        member.asset_e8_deposit = 0;
        member.rune_e8_deposit = 0;

        -- Add to members count table
        INSERT INTO midgard_agg.members_count VALUES
        (
            member.pool,
            COALESCE(
                (
                    SELECT count + 1 FROM midgard_agg.members_count
                    WHERE pool = member.pool ORDER BY block_timestamp DESC LIMIT 1
                ),
                1
            ),
            NEW.block_timestamp 
        ) ON CONFLICT (pool, block_timestamp) DO UPDATE SET count = EXCLUDED.count;
    END IF;

    -- Currently (2022-05-18) there is no way for a member to change/add/remove their rune or asset
    -- addresses. But, this was not always the case. So, to handle these past instances, we allow
    -- a missing asset address to be changed into a specific address. But after that it
    -- cannot change again.
    member.asset_addr := COALESCE(member.asset_addr, NEW.asset_addr);
    member.rune_addr := COALESCE(member.rune_addr, NEW.rune_addr);

    member.lp_units_total := member.lp_units_total + COALESCE(NEW.lp_units_delta, 0);
    NEW.lp_units_total := member.lp_units_total;


    IF NEW.change_type = 'add' THEN
        member.added_asset_e8_total := member.added_asset_e8_total + NEW.asset_e8_delta;
        member.added_rune_e8_total := member.added_rune_e8_total + NEW.rune_e8_delta;

        -- Calculate deposited Value here
        member.asset_e8_deposit := member.asset_e8_deposit + NEW.asset_e8_delta;
        member.rune_e8_deposit := member.rune_e8_deposit + NEW.rune_e8_delta;

        -- Reset pending asset and rune
        NEW.pending_asset_e8_delta := -member.pending_asset_e8_total;
        NEW.pending_rune_e8_delta := -member.pending_rune_e8_total;
        member.pending_asset_e8_total := 0;
        member.pending_rune_e8_total := 0;

        member.first_added_timestamp := COALESCE(member.first_added_timestamp, NEW.block_timestamp);
        member.last_added_timestamp := NEW.block_timestamp;
    END IF;

    IF NEW.change_type = 'withdraw' THEN
        -- Deltas are negative here
        member.withdrawn_asset_e8_total := member.withdrawn_asset_e8_total - NEW.asset_e8_delta;
        member.withdrawn_rune_e8_total := member.withdrawn_rune_e8_total - NEW.rune_e8_delta;
        -- Calculate deposited Value here
        member.asset_e8_deposit := ((10000 - NEW.basis_points)/10000) * member.asset_e8_deposit;
        member.rune_e8_deposit := ((10000 - NEW.basis_points)/10000) * member.rune_e8_deposit;
    END IF;

    IF NEW.change_type = 'pending_add' THEN
        member.pending_asset_e8_total := member.pending_asset_e8_total + NEW.pending_asset_e8_delta;
        member.pending_rune_e8_total := member.pending_rune_e8_total + NEW.pending_rune_e8_delta;
    END IF;

    IF NEW.change_type = 'pending_withdraw' THEN
        -- Reset pending asset and rune
        -- TODO(huginn): When we have reliable order information check that this is correct:
        member.pending_asset_e8_total := 0;
        member.pending_rune_e8_total := 0;
    END IF;

    -- Record into the log the new pending totals.
    NEW.pending_asset_e8_total := member.pending_asset_e8_total;
    NEW.pending_rune_e8_total := member.pending_rune_e8_total;

    -- Update the `members` table:
    IF member.lp_units_total = 0 AND member.pending_asset_e8_total = 0
            AND member.pending_rune_e8_total = 0 THEN
        DELETE FROM midgard_agg.members
        WHERE member_id = member.member_id AND pool = member.pool;

        -- Remove member from members count table
        INSERT INTO midgard_agg.members_count VALUES
        (
            member.pool,
            (
                SELECT count - 1 FROM midgard_agg.members_count
                WHERE pool = member.pool ORDER BY block_timestamp DESC LIMIT 1
            ),
            NEW.block_timestamp
        )
        ON CONFLICT (pool, block_timestamp) DO UPDATE SET count = EXCLUDED.count;
    ELSE
        INSERT INTO midgard_agg.members VALUES (member.*)
        ON CONFLICT (member_id, pool) DO UPDATE SET
            -- Note, `EXCLUDED` is exactly the `member` variable here
            lp_units_total = EXCLUDED.lp_units_total,
            asset_addr = EXCLUDED.asset_addr,
            asset_e8_deposit = EXCLUDED.asset_e8_deposit,
            added_asset_e8_total = EXCLUDED.added_asset_e8_total,
            withdrawn_asset_e8_total = EXCLUDED.withdrawn_asset_e8_total,
            pending_asset_e8_total = EXCLUDED.pending_asset_e8_total,
            rune_addr = EXCLUDED.rune_addr,
            rune_e8_deposit = EXCLUDED.rune_e8_deposit,
            added_rune_e8_total = EXCLUDED.added_rune_e8_total,
            withdrawn_rune_e8_total = EXCLUDED.withdrawn_rune_e8_total,
            pending_rune_e8_total = EXCLUDED.pending_rune_e8_total,
            first_added_timestamp = EXCLUDED.first_added_timestamp,
            last_added_timestamp = EXCLUDED.last_added_timestamp;
    END IF;

    -- Never fails, just enriches the row to be inserted and updates the `members` table.
    RETURN NEW;
END;
$BODY$;

CREATE TRIGGER add_log_trigger
    BEFORE INSERT ON midgard_agg.members_log
    FOR EACH ROW
    EXECUTE FUNCTION midgard_agg.add_members_log();


CREATE PROCEDURE midgard_agg.update_members_interval(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    INSERT INTO midgard_agg.members_log (
        SELECT * FROM midgard_agg.members_log_partial
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        ORDER BY event_id
    );
END
$BODY$;

CREATE PROCEDURE midgard_agg.update_members(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = 'members'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating members into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL midgard_agg.update_members_interval(w_old, w_new);
    UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = 'members';
END
$BODY$;
