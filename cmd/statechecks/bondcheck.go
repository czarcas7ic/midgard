package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/internal/db"
)

func getThorNodeBonds(thorNodeUrl string, height int64) map[string]int64 {
	ret := map[string]int64{}
	var nodes []Node
	queryThorNode(thorNodeUrl, "/nodes", height, &nodes)
	for _, node := range nodes {
		bond, err := strconv.ParseInt(node.TotalBond, 10, 64)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed parsing node bond")
		}
		ret[node.BondAddress] = bond
	}
	return ret
}

func bondDifferences(thorNodeBonds map[string]int64, midgardBonds map[string]int64) bool {
	different := false
	seen := map[string]bool{}
	for addr, tbond := range thorNodeBonds {
		seen[addr] = true
		mbond, ok := midgardBonds[addr]
		if !ok {
			log.Debug().Int64("bond", tbond).Str("address", addr).Msg("bond only in thornode")
			different = true
		} else if tbond != mbond {
			excess := 100 * float64(mbond-tbond) / float64(tbond)
			log.Debug().Int64("thorchain bond", tbond).Int64("midgard bond", mbond).Str("excess", fmt.Sprintf("%.2f%%", excess)).Str("address", addr).Msg("bond mismatch")
			different = true
		} else {
			log.Debug().Int64("bond", tbond).Str("address", addr).Msg("bonds match")
		}
	}
	for addr, mbond := range midgardBonds {
		if !seen[addr] {
			log.Debug().Int64("bond", mbond).Str("address", addr).Msg("bond only in midgard")
			different = true
		}
	}

	return different
}

func BondDetails(ctx context.Context, thorNodeUrl string) {
	// TODO(huginn): Change to binary search if this becomes excessive.
	// Currently, this make a thor node query for every block that has a bond event.

	log.Debug().Msg("======== Scanning for bond differences")
	var lastHeight int64 = -1

	midgardBonds := map[string]int64{}

	bondEventsQ := `
		SELECT
			COALESCE(from_addr, to_addr),
			bond_type,
			E8,
			height
		FROM bond_events
		JOIN block_log ON block_timestamp = timestamp
		ORDER BY block_timestamp ASC
	`
	rows, err := db.Query(ctx, bondEventsQ)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to run query")
	}
	defer rows.Close()

	for rows.Next() {
		var bondAddress string
		var bond_type string
		var amount, height int64
		err := rows.Scan(&bondAddress, &bond_type, &amount, &height)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to scan rows")
		}

		if height != lastHeight {
			if lastHeight != -1 {
				if bondDifferences(getThorNodeBonds(thorNodeUrl, lastHeight), midgardBonds) {
					log.Warn().Int64("height", lastHeight).Msg("bond divergence detected")
					return
				}
			}
			lastHeight = height
		}

		if bond_type == "bond_returned" || bond_type == "bond_cost" {
			amount *= -1
		}
		midgardBonds[bondAddress] += amount
	}
}
