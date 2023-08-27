INSERT INTO midgard_agg.watermarks (materialized_table, watermark)
    VALUES ('rune_price', 0);

CREATE TABLE midgard_agg.rune_price (
    rune_price_usd DOUBLE PRECISION NOT NULL,
    block_timestamp bigint NOT NULL,
    PRIMARY KEY(block_timestamp)
);

-- TODO(hooriRN): fill with actual price instead of a constant
CREATE PROCEDURE midgard_agg.update_rune_price_interval(t1 bigint, t2 bigint)
LANGUAGE plpgsql AS $BODY$
BEGIN
    INSERT INTO midgard_agg.rune_price AS cb (
        SELECT 
            1 AS rune_price_usd,
            timestamp as block_timestamp
        FROM block_log
        WHERE t1 <= timestamp AND timestamp < t2
    )
    ON CONFLICT (block_timestamp) DO UPDATE SET rune_price_usd = cb.rune_price_usd;
END
$BODY$;

CREATE PROCEDURE midgard_agg.update_rune_price(w_new bigint)
LANGUAGE plpgsql AS $BODY$
DECLARE
    w_old bigint;
BEGIN
    SELECT watermark FROM midgard_agg.watermarks WHERE materialized_table = 'rune_price'
        FOR UPDATE INTO w_old;
    IF w_new <= w_old THEN
        RAISE WARNING 'Updating rune prices into past: % -> %', w_old, w_new;
        RETURN;
    END IF;
    CALL midgard_agg.update_rune_price_interval(w_old, w_new);
    UPDATE midgard_agg.watermarks SET watermark = w_new WHERE materialized_table = 'rune_price';
END
$BODY$;
