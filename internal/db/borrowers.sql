INSERT INTO midgard_agg.watermarks (materialized_table, watermark)
    VALUES ('borrowers', 0);

CREATE TABLE midgard_agg.borrowers_log (
    borrower_id text NOT NULL,
    change_type text NOT NULL,
    --
    collateral_asset text NOT NULL,
    target_asset text,
    debt_up BIGINT,
    debt_down BIGINT,
    collateral_up BIGINT,
    collateral_down BIGINT,
    collateralization_ratio BIGINT,
    --
    event_id bigint NOT NULL,
    block_timestamp bigint NOT NULL
);

CREATE VIEW midgard_agg.borrowers_log_partial AS (
    SELECT * FROM (
        SELECT
            owner AS borrower_id,
            'open' AS change_type,
            collateral_asset,
            target_asset,
            debt_up,
            NULL::bigint AS debt_down,
            collateral_up,
            NULL::bigint AS collateral_down,
            collateralization_ratio,
            event_id,
            block_timestamp
        FROM loan_open_events
        UNION ALL
        SELECT
            owner AS borrower_id,
            'repayment' AS change_type,
            collateral_asset,
            NULL AS target_asset,
            NULL::bigint AS debt_up,
            debt_down,
            NULL::bigint AS collateral_up,
            collateral_down,
            NULL::bigint AS collateralization_ratio,
            event_id,
            block_timestamp
        FROM loan_repayment_events
    ) AS x
    ORDER BY block_timestamp, change_type
);

CREATE TABLE midgard_agg.borrowers (
    borrower_id text NOT NULL,
    collateral_asset text,
    target_assets text[],
    -- CR fields
    debt_up bigint NOT NULL,
    debt_down bigint NOT NULL,
    collateral_up bigint NOT NULL,
    collateral_down bigint NOT NULL,
    total_collateral_tor bigint NOT NULL,
    --
    last_open_loan_timestamp bigint,
    last_repay_loan_timestamp bigint,
    PRIMARY KEY (borrower_id, collateral_asset)
)
WITH (fillfactor = 90);

CREATE INDEX ON midgard_agg.borrowers (borrower_id);
CREATE INDEX ON midgard_agg.borrowers (collateral_asset);

CREATE TABLE midgard_agg.borrowers_count (
    collateral_asset text NOT NULL,
    count bigint NOT NULL,
    block_timestamp bigint NOT NULL,
    PRIMARY KEY (collateral_asset, block_timestamp)
);

CREATE INDEX ON midgard_agg.borrowers_count (collateral_asset, block_timestamp DESC);

CREATE FUNCTION midgard_agg.add_borrowers_log() RETURNS trigger
LANGUAGE plpgsql AS $BODY$
DECLARE
    borrower midgard_agg.borrowers%ROWTYPE;
BEGIN
    -- Fix Ethereum addresses to be uniformly lowercase
    -- TODO(huginn): fix this on the event parsing/recording level
    IF NEW.collateral_asset LIKE 'ETH.%' THEN
        NEW.borrower_id = lower(NEW.borrower_id);
    END IF;

    -- Look up the current state of the borrower
    SELECT * FROM midgard_agg.borrowers
        WHERE borrower_id = NEW.borrower_id AND collateral_asset = NEW.collateral_asset
        FOR UPDATE INTO borrower;

    -- If this is a new borrower, fill out its fields
    IF borrower.borrower_id IS NULL THEN
        borrower.borrower_id = NEW.borrower_id;
        borrower.collateral_asset = NEW.collateral_asset;
        borrower.target_assets = ARRAY[]::text[];
        borrower.debt_up = 0;
        borrower.debt_down = 0;
        borrower.collateral_up = 0;
        borrower.collateral_down = 0;
        borrower.last_open_loan_timestamp = 0;
        borrower.last_repay_loan_timestamp = 0;
        borrower.total_collateral_tor = 0;

        -- Add to borrowers count table
        INSERT INTO midgard_agg.borrowers_count VALUES
        (
            borrower.collateral_asset,
            COALESCE(
                (
                    SELECT count + 1 FROM midgard_agg.borrowers_count
                    WHERE
                        collateral_asset = borrower.collateral_asset 
                    ORDER BY block_timestamp DESC LIMIT 1
                ),
                1
            ),
            NEW.block_timestamp 
        ) ON CONFLICT (collateral_asset, block_timestamp) DO UPDATE SET count = EXCLUDED.count;
    END IF;

    --  can change the debt in log to debt_delta as members
    IF NEW.change_type = 'open' THEN
        borrower.debt_up := borrower.debt_up + NEW.debt_up;
        borrower.collateral_up := borrower.collateral_up + NEW.collateral_up;
        borrower.total_collateral_tor := borrower.total_collateral_tor + (NEW.debt_up * NEW.collateralization_ratio / 10000);

        IF NOT NEW.target_asset = ANY(borrower.target_assets) THEN
            borrower.target_assets := borrower.target_assets || NEW.target_asset;
        END IF;

        borrower.last_open_loan_timestamp := NEW.block_timestamp;
    END IF;

    IF NEW.change_type = 'repayment' THEN
        IF borrower.debt_up > borrower.debt_down THEN
            borrower.total_collateral_tor := borrower.total_collateral_tor - (borrower.total_collateral_tor * NEW.debt_down / (borrower.debt_up - borrower.debt_down));
        END IF;

        borrower.debt_down := borrower.debt_down + NEW.debt_down;
        borrower.collateral_down := borrower.collateral_down + NEW.collateral_down;

        borrower.last_repay_loan_timestamp := NEW.block_timestamp;
    END IF;

    -- Update the `borrowers` table:
    IF borrower.debt_up - borrower.debt_down <= 0 THEN
        -- Remove borrower from borrowers count table
        INSERT INTO midgard_agg.borrowers_count VALUES
        (
            borrower.collateral_asset,
            (
                SELECT count - 1 FROM midgard_agg.borrowers_count
                WHERE collateral_asset = borrower.collateral_asset
                ORDER BY block_timestamp DESC LIMIT 1
            ),
            NEW.block_timestamp
        )
        ON CONFLICT (collateral_asset, block_timestamp) DO UPDATE SET count = EXCLUDED.count;
    ELSE
        INSERT INTO midgard_agg.borrowers VALUES (borrower.*)
        ON CONFLICT (borrower_id, collateral_asset) DO UPDATE SET
            -- Note, `EXCLUDED` is exactly the `borrower` variable here
            target_assets = EXCLUDED.target_assets,
            debt_up = EXCLUDED.debt_up,
            debt_down = EXCLUDED.debt_down,
            collateral_up = EXCLUDED.collateral_up,
            collateral_down = EXCLUDED.collateral_down,
            total_collateral_tor = EXCLUDED.total_collateral_tor,
            last_open_loan_timestamp = EXCLUDED.last_open_loan_timestamp,
            last_repay_loan_timestamp = EXCLUDED.last_repay_loan_timestamp;
    END IF;

    -- Never fails, just enriches the row to be inserted and updates the `borrowers` table.
    RETURN NEW;
END;
$BODY$;

CREATE TRIGGER add_log_trigger
    BEFORE INSERT ON midgard_agg.borrowers_log
    FOR EACH ROW
    EXECUTE FUNCTION midgard_agg.add_borrowers_log();


CREATE PROCEDURE midgard_agg.update_borrowers_interval(t1 bigint, t2 bigint)
LANGUAGE SQL AS $BODY$
    INSERT INTO midgard_agg.borrowers_log (
        SELECT * FROM midgard_agg.borrowers_log_partial
        WHERE t1 <= block_timestamp AND block_timestamp < t2
        ORDER BY event_id
    );
$BODY$;

CREATE PROCEDURE midgard_agg.update_borrowers(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = 'borrowers'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating borrowers into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL midgard_agg.update_borrowers_interval(w_old, w_new);
    UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = 'borrowers';
END
$BODY$;
