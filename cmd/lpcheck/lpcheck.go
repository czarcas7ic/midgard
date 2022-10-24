package main

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

//go:embed lpcheck.sql
var query string

var rowNum int
var errors int
var errorMap = make(map[string]int)

func unexpected(format string, args ...interface{}) {
	errors++
	errorMap[format]++
	if errorMap[format] < 5 {
		msg := fmt.Sprintf(format, args...)
		midlog.ErrorF("Unexpected in row %d: %s", rowNum, msg)
	}
}

func main() {
	midlog.LogCommandLine()

	config.ReadGlobal()
	dbinit.Setup()

	rows, err := db.Query(context.Background(), query)
	if err != nil {
		midlog.FatalE(err, "Query failed")
	}
	defer rows.Close()

	var first_timestamp int64
	var cur_member_id, cur_pool string
	var cur_rune_addr, cur_asset_addr sql.NullString
	var cur_units, pending_rune, pending_asset int64
	var has_pending bool

	for rows.Next() {
		var member_id, pool, change_type, rune_addr, asset_addr sql.NullString
		var rune_e8, asset_e8, units, event_id, block_timestamp sql.NullInt64
		err := rows.Scan(&member_id, &pool, &change_type, &rune_addr, &rune_e8,
			&asset_addr, &asset_e8, &units, &event_id, &block_timestamp)
		if err != nil {
			midlog.FatalE(err, "Scan error")
		}
		rowNum++

		if !member_id.Valid {
			unexpected("NULL member_id")
			continue
		}
		if !pool.Valid {
			unexpected("NULL pool")
			continue
		}
		if !change_type.Valid {
			unexpected("NULL change_type")
			continue
		}
		if !event_id.Valid {
			unexpected("NULL event_id")
			continue
		}
		if !block_timestamp.Valid {
			unexpected("NULL block_timestamp")
			continue
		}
		if !rune_e8.Valid || !asset_e8.Valid || !units.Valid {
			unexpected("NULL rune_e8, asset_e8, or units")
			continue
		}

		{
			member_id := member_id.String
			pool := pool.String
			change_type := change_type.String
			rune_e8 := rune_e8.Int64
			asset_e8 := asset_e8.Int64
			units := units.Int64

			if strings.HasPrefix(pool, "ETH.") && asset_addr.Valid {
				asset_addr.String = strings.ToLower(asset_addr.String)
			}

			if change_type != "withdraw" {
				if rune_addr.Valid {
					if rune_addr.String != member_id {
						unexpected("member_id != existing rune_addr")
					}
				} else if asset_addr.Valid {
					if asset_addr.String != member_id {
						unexpected("member_id != asset_addr (rune_addr NULL)")
					}
				} else {
					unexpected("both rune_add and asset_addr are NULL")
				}
			}

			if cur_member_id != member_id || cur_pool != pool {
				cur_member_id = member_id
				cur_pool = pool
				cur_units = 0
				pending_rune = 0
				pending_asset = 0
				has_pending = false
				first_timestamp = block_timestamp.Int64
				if change_type != "add" && change_type != "_pending_add" {
					unexpected("new member starts with %s", change_type)
				}
				cur_rune_addr = rune_addr
				cur_asset_addr = asset_addr
			}

			// if first_timestamp < 1647990000e9 { // after fork
			// if first_timestamp < 1631113326026380027 { // 2000000
			// if first_timestamp < 1637530037113506433 { // 3000000
			// if first_timestamp < 1643295652785752395 { // 4000000
			if first_timestamp < 0 {
				continue
			}

			cur_units += units
			if cur_units < 0 {
				unexpected("negative units")
			}

			if has_pending {
				if change_type == "_pending_withdraw" {
					if pending_rune != rune_e8 {
						unexpected("pending_withdraw rune mismatch")
					}
					if pending_asset != asset_e8 {
						unexpected("pending_withdraw asset mismatch")
					}
					pending_asset = 0
					pending_rune = 0
					has_pending = false
				}

				if change_type == "add" {
					if pending_rune > 0 {
						if pending_rune != rune_e8 {
							unexpected("add pending rune mismatch: %d != %d", pending_rune, rune_e8)
						} else {
							pending_rune -= rune_e8
						}
					}
					if pending_asset > 0 {
						if pending_asset != asset_e8 {
							unexpected("add pending asset mismatch: %d != %d", pending_asset, asset_e8)
						} else {
							pending_asset -= asset_e8
						}
					}
				}

				if pending_asset == 0 && pending_rune == 0 {
					has_pending = false
				}
			}

			if change_type == "_pending_add" {
				has_pending = true
				pending_rune += rune_e8
				pending_asset += asset_e8
			}

			if change_type != "withdraw" {
				if cur_rune_addr != rune_addr {
					unexpected("rune_addr changed: %v %v", cur_rune_addr, rune_addr)
				}
				if cur_asset_addr != asset_addr {
					unexpected("asset_addr changed: %v %v", cur_asset_addr, asset_addr)
				}
			}

			if cur_units == 0 && !has_pending {
				cur_member_id = "reset"
			}
		}
	}

	midlog.InfoT(midlog.Tags(midlog.Int("rows", rowNum), midlog.Int("errors", errors)),
		"Total rows processed")
	for k, v := range errorMap {
		midlog.InfoF("Error count %s: %d", k, v)
	}
}
