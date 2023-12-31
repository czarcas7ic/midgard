package timeseries

import (
	"context"
	"strings"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
)

// Represents membership in a pool
type membership struct {
	runeAddress    string
	assetAddress   string
	liquidityUnits int64
}

type addrIndex map[string](map[string]*membership)

func (index addrIndex) getMembership(address, pool string) (*membership, bool) {
	_, ok := index[address]
	if ok {
		ret, ok := index[address][pool]
		return ret, ok
	} else {
		return nil, false
	}
}

func (index addrIndex) setMembership(address, pool string, newMembership *membership) {
	if index[address] == nil {
		index[address] = make(map[string]*membership)
	}
	index[address][pool] = newMembership
}

// MemberAddrs gets all member known addresses.
// When there's a rune/asset address pair or a rune addres for the member,
// the rune asset is shown.
// Else the asset address is shown.
// If an address participates in multiple pools it will be shown only once
func GetMemberAddrs(ctx context.Context, pool *string) (addrs []string, err error) {
	// Build indexes: nested maps -> address and pools for each address as keys
	// Needed to access each member from any address and also to identify unique addresses

	// runeAddrIndex: all memberships with a rune address
	// using the rune address as key
	runeAddrIndex := make(addrIndex)

	// asymAddrIndex: all memberships with only an asset address
	// none of the pointes here should be stored in runeAddrIndex
	// A single asset address can stake in different pools
	// (E.g.: ETH address in mutiple ERC20 tokens)
	asymAssetAddrIndex := make(addrIndex)

	poolFilter := ""
	qargs := []interface{}{}
	if pool != nil {
		poolFilter = "pool = $1"
		qargs = append(qargs, pool)
	}

	// Rune asset queryies. If a liquidity provider has a rune address then it is identified
	// by its rune address.
	// NOTE: Assumes only a single asset address per pool can be paired with a single rune
	// address
	runeALQ := `
		SELECT
			rune_addr,
			COALESCE(MAX(asset_addr), ''),
			pool,
			SUM(stake_units) as liquidity_units
		FROM stake_events
		` + db.Where(poolFilter, "rune_addr IS NOT NULL") + `
		GROUP BY rune_addr, pool
	`
	runeALRows, err := db.Query(ctx, runeALQ, qargs...)
	if err != nil {
		return nil, err
	}
	defer runeALRows.Close()

	for runeALRows.Next() {
		var newMembership membership
		var pool string
		err := runeALRows.Scan(
			&newMembership.runeAddress,
			&newMembership.assetAddress,
			&pool,
			&newMembership.liquidityUnits)
		if err != nil {
			return nil, err
		}
		runeAddrIndex.setMembership(newMembership.runeAddress, pool, &newMembership)
	}

	// Asymmetrical addLiquidity with asset only
	// part of asym membership (as if there was a rune address present, the liquidity provider
	// would be matched using the rune address)
	asymAssetALQ := `
		SELECT
			asset_addr,
			pool,
			SUM(stake_units) as liquidity_units
		FROM stake_events
		` + db.Where(poolFilter, "asset_addr IS NOT NULL AND rune_addr IS NULL") + `
		GROUP BY asset_addr, pool
	`

	asymAssetALRows, err := db.Query(ctx, asymAssetALQ, qargs...)
	if err != nil {
		return nil, err
	}
	defer asymAssetALRows.Close()
	for asymAssetALRows.Next() {
		var assetAddress, pool string
		var liquidityUnits int64
		err := asymAssetALRows.Scan(&assetAddress, &pool, &liquidityUnits)
		if err != nil {
			return nil, err
		}
		newMembership := membership{
			assetAddress:   assetAddress,
			liquidityUnits: liquidityUnits,
		}
		asymAssetAddrIndex.setMembership(assetAddress, pool, &newMembership)
	}

	// Withdraws: try matching from address to a membreship from
	// the index and subtract addLiquidityUnits.
	// If there's no match either there's an error with the
	// implementation or the Thorchain events.
	withdrawQ := `
		SELECT
			from_addr,
			pool,
			SUM(stake_units) as liquidity_units
		FROM unstake_events
		` + db.Where(poolFilter) + `
		GROUP BY from_addr, pool
	`
	withdrawRows, err := db.Query(ctx, withdrawQ, qargs...)
	if err != nil {
		return nil, err
	}
	defer withdrawRows.Close()

	for withdrawRows.Next() {
		var fromAddr, pool string
		var liquidityUnits int64
		err := withdrawRows.Scan(&fromAddr, &pool, &liquidityUnits)
		if err != nil {
			return nil, err
		}

		existingMembership, ok := runeAddrIndex.getMembership(fromAddr, pool)
		if ok && (existingMembership.runeAddress == fromAddr) {
			existingMembership.liquidityUnits -= liquidityUnits
			continue
		}

		existingMembership, ok = asymAssetAddrIndex.getMembership(fromAddr, pool)
		if ok && (existingMembership.assetAddress == fromAddr) {
			existingMembership.liquidityUnits -= liquidityUnits
			continue
		}

		// TODO(acsaba): Fix underlying events.
		// Discarding this error for now
		// return nil, fmt.Errorf("Address %s, pool %s, found in withdraw events should have a matching membership", fromAddr, pool)
	}

	// Lookup membership addresses:
	// Either in runeIndex or asymIndex with at least one pool
	// with positive liquidityUnits balance
	addrs = make([]string, 0, len(runeAddrIndex)+len(asymAssetAddrIndex))

	for address, poolMemberships := range runeAddrIndex {
		// if it has at least a non zero balance, add it to the result
		isMember := false
		for _, memb := range poolMemberships {
			if memb.liquidityUnits > 0 {
				isMember = true
				break
			}
		}

		if isMember {
			addrs = append(addrs, address)
		}
	}

	for address, poolMemberships := range asymAssetAddrIndex {
		// if it has at least a non zero balance, add it to the result
		isMember := false
		for _, memb := range poolMemberships {
			if memb.liquidityUnits > 0 {
				isMember = true
				break
			}
		}

		if isMember {
			addrs = append(addrs, address)
		}
	}

	return addrs, nil
}

func addressIsRune(address string) bool {
	return strings.HasPrefix(address, "thor") || strings.HasPrefix(address, "tthor")
}

// Info of a member in a specific pool.
type MemberPool struct {
	Pool           string
	RuneAddress    string
	AssetAddress   string
	LiquidityUnits int64
	RuneAdded      int64
	AssetAdded     int64
	DateFirstAdded int64
	DateLastAdded  int64
	RuneWithdrawn  int64
	AssetWithdrawn int64
}

func (memberPool MemberPool) toOapigen() oapigen.MemberPool {
	return oapigen.MemberPool{
		Pool:           memberPool.Pool,
		RuneAddress:    memberPool.RuneAddress,
		AssetAddress:   memberPool.AssetAddress,
		LiquidityUnits: util.IntStr(memberPool.LiquidityUnits),
		RuneAdded:      util.IntStr(memberPool.RuneAdded),
		AssetAdded:     util.IntStr(memberPool.AssetAdded),
		RuneWithdrawn:  util.IntStr(memberPool.RuneWithdrawn),
		AssetWithdrawn: util.IntStr(memberPool.AssetWithdrawn),
		DateFirstAdded: util.IntStr(memberPool.DateFirstAdded),
		DateLastAdded:  util.IntStr(memberPool.DateLastAdded),
	}
}

// Pools data associated with a single member
type MemberPools []MemberPool

func (memberPools MemberPools) ToOapigen() []oapigen.MemberPool {
	ret := make([]oapigen.MemberPool, len(memberPools))
	for i, memberPool := range memberPools {
		ret[i] = memberPool.toOapigen()
	}

	return ret
}

func GetMemberPools(ctx context.Context, address string) (MemberPools, error) {
	if addressIsRune(address) {
		return memberDetailsRune(ctx, address)
	} else {
		return memberDetailsAsset(ctx, address)
	}
}

const mpAddLiquidityQFields = `
		COALESCE(SUM(asset_E8), 0),
		COALESCE(SUM(rune_E8), 0),
		COALESCE(SUM(stake_units), 0),
		COALESCE(MIN(block_timestamp) / 1000000000, 0),
		COALESCE(MAX(block_timestamp) / 1000000000, 0)
`

const mpWithdrawQFields = `
		COALESCE(SUM(emit_asset_e8), 0),
		COALESCE(SUM(emit_rune_e8), 0),
		COALESCE(SUM(stake_units), 0)
`

// RUNE addresses
func memberDetailsRune(ctx context.Context, runeAddress string) (MemberPools, error) {
	// If a member has a RUNE address then it is identified in the KV store with that address
	// so we know there's one member per pool with a given RUNE address
	// NOTE: We asume that no rune address can have different asset addresses for the same pool
	// as Thornode seems to return an error when a different rune address is added
	addLiquidityQ := `SELECT
		pool,
		COALESCE(MAX(asset_addr), ''),
	` + mpAddLiquidityQFields + `
	FROM stake_events
	WHERE rune_addr = $1
	GROUP BY pool`

	addLiquidityRows, err := db.Query(ctx, addLiquidityQ, runeAddress)
	if err != nil {
		return nil, err
	}
	defer addLiquidityRows.Close()

	memberPoolsMap := make(map[string]MemberPool)

	for addLiquidityRows.Next() {
		memberPool := MemberPool{}
		err := addLiquidityRows.Scan(
			&memberPool.Pool,
			&memberPool.AssetAddress,
			&memberPool.AssetAdded,
			&memberPool.RuneAdded,
			&memberPool.LiquidityUnits,
			&memberPool.DateFirstAdded,
			&memberPool.DateLastAdded,
		)
		if err != nil {
			return nil, err
		}

		memberPool.RuneAddress = runeAddress
		memberPoolsMap[memberPool.Pool] = memberPool
	}

	// As members need to use the RUNE addresss to withdraw we use it to match each pool
	withdrawQ := `SELECT
		pool,
		` + mpWithdrawQFields + `
	FROM unstake_events
	WHERE from_addr = $1
	GROUP BY pool
	`

	withdrawRows, err := db.Query(ctx, withdrawQ, runeAddress)
	if err != nil {
		return nil, err
	}
	defer withdrawRows.Close()

	for withdrawRows.Next() {
		var pool string
		var assetWithdrawn, runeWithdrawn, unitsWithdrawn int64
		err := withdrawRows.Scan(&pool, &assetWithdrawn, &runeWithdrawn, &unitsWithdrawn)
		if err != nil {
			return nil, err
		}

		memberPool := memberPoolsMap[pool]
		memberPool.AssetWithdrawn = assetWithdrawn
		memberPool.RuneWithdrawn = runeWithdrawn
		memberPool.LiquidityUnits -= unitsWithdrawn

		memberPoolsMap[pool] = memberPool
	}

	ret := make(MemberPools, 0, len(memberPoolsMap))
	for _, memberPool := range memberPoolsMap {
		if memberPool.LiquidityUnits > 0 {
			ret = append(ret, memberPool)
		}
	}

	return ret, nil
}

func memberDetailsAsset(ctx context.Context, assetAddress string) (MemberPools, error) {
	// Get all the rune addresses the asset address is paired with
	addressesQ := `SELECT
		se.pool,
		COALESCE(se.rune_addr, '') as pair_rune_addr
	FROM stake_events AS se
	WHERE se.asset_addr = $1
	GROUP BY pool, pair_rune_addr
	`

	addressesRows, err := db.Query(ctx, addressesQ, assetAddress)
	if err != nil {
		return nil, err
	}
	defer addressesRows.Close()

	var memberPools MemberPools
	for addressesRows.Next() {
		memberPool := MemberPool{AssetAddress: assetAddress}
		err := addressesRows.Scan(&memberPool.Pool, &memberPool.RuneAddress)

		var whereAddLiquidityAddresses, queryAddress string
		if memberPool.RuneAddress == "" {
			// asym liquidity provider, asset address is used to identify it
			// (if there is a rune_addr it will always be used to get the lp so it has to be NULL)
			whereAddLiquidityAddresses = "WHERE asset_addr = $1 AND rune_addr IS NULL"
			queryAddress = memberPool.AssetAddress
		} else {
			// sym liquidity provider, rune address is used to identify it
			whereAddLiquidityAddresses = "WHERE rune_addr = $1"
			queryAddress = memberPool.RuneAddress
		}

		addLiquidityQ := `SELECT ` + mpAddLiquidityQFields + `FROM stake_events ` + whereAddLiquidityAddresses + ` AND pool = $2`

		addLiquidityRow, err := db.Query(ctx, addLiquidityQ, queryAddress, memberPool.Pool)
		if err != nil {
			return nil, err
		}
		defer addLiquidityRow.Close()
		if addLiquidityRow.Next() {
			err := addLiquidityRow.Scan(&memberPool.AssetAdded, &memberPool.RuneAdded, &memberPool.LiquidityUnits, &memberPool.DateFirstAdded, &memberPool.DateLastAdded)
			if err != nil {
				return nil, err
			}
		}

		withdrawQ := `SELECT ` + mpWithdrawQFields + ` FROM unstake_events WHERE from_addr=$1 AND pool=$2`
		withdrawRow, err := db.Query(ctx, withdrawQ, queryAddress, memberPool.Pool)
		if err != nil {
			return nil, err
		}
		defer withdrawRow.Close()
		if withdrawRow.Next() {
			var unitsWithdrawn int64
			err = withdrawRow.Scan(&memberPool.AssetWithdrawn, &memberPool.RuneWithdrawn, &unitsWithdrawn)
			if err != nil {
				return nil, err
			}
			memberPool.LiquidityUnits -= unitsWithdrawn
		}

		if memberPool.LiquidityUnits > 0 {
			memberPools = append(memberPools, memberPool)
		}
	}

	return memberPools, nil
}
