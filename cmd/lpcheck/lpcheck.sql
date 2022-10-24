SELECT * FROM (
    SELECT
        COALESCE(rune_addr, asset_addr) AS member_id,
        pool,
        'add' AS change_type,
        rune_addr,
        rune_e8,
        asset_addr,
        asset_e8,
        stake_units AS units,
        event_id,
        block_timestamp
    FROM stake_events
    UNION ALL
    SELECT
        from_addr AS member_id,
        pool,
        'withdraw' AS change_type,
        NULL AS rune_addr,
        emit_rune_e8 AS rune_e8,
        NULL AS asset_addr,
        emit_asset_e8 AS asset_e8,
        -stake_units AS units,
        event_id,
        block_timestamp
    FROM unstake_events
    UNION ALL
    SELECT
        COALESCE(rune_addr, asset_addr) AS member_id,
        pool,
        '_pending_' || pending_type AS change_type,
        rune_addr,
        rune_e8,
        asset_addr,
        asset_e8,
        0 AS units,
        event_id,
        block_timestamp
    FROM pending_liquidity_events
) AS x
ORDER BY member_id, pool, event_id ASC
;
