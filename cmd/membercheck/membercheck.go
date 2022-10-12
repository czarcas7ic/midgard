// This tool checks if pool units reported reported for each member sum by ThorNode sums up to the
// total units of the pool.
// At the time of writing it does.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/api"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/dbinit"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

const usageStr = `Check pool units share of each member
Usage:
$ go run ./cmd/membercheck config pool heightOrBlockTimestamp
or
$ go run ./cmd/membercheck --allpools  config heightOrBlockTimestamp
`

func init() {
	flag.Usage = func() {
		fmt.Print(usageStr)
		flag.PrintDefaults()
	}
}

var AllPoolsStructured = flag.Bool("allpools", false,
	"No binary search, only the latest depth differences in structured form.")

type ThorNodeSummary struct {
	TotalUnits int64 `json:"LP_units,string"`
}

// MemberChange may represent the state of a Member, an Add or a Withdraw.
// In case of withdraw the units are negative.
type MemberChange struct {
	Units        int64  `json:"units,string"`
	RuneAddress  string `json:"rune_address"`
	AssetAddress string `json:"asset_address"`
}

func main() {
	midlog.LogCommandLine()

	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Println("Not enough arguments!")
		flag.Usage()
		return
	}

	config.ReadGlobalFrom(flag.Arg(0))

	dbinit.Setup()

	db.InitializeChainVarsFromThorNode()
	db.EnsureDBMatchesChain()

	if *AllPoolsStructured {
		CheckAllPoolsStructured()
	} else {
		CheckOnePool()
	}
}

func findHeight(param string) (height int64, timestamp db.Nano) {
	idStr := param

	heightOrTimestamp, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		midlog.FatalF("Couldn't parse height or timestamp: %s", idStr)
	}
	height, timestamp, err = api.TimestampAndHeight(context.Background(), heightOrTimestamp)
	if err != nil {
		midlog.FatalE(err, "Couldn't find height or timestamp. ")
	}
	return
}

func CheckOnePool() {
	if flag.NArg() != 3 {
		fmt.Println("provide 3 args!")
		flag.Usage()
		return
	}

	ctx := context.Background()
	pool := flag.Arg(1)
	idStr := flag.Arg(2)

	height, timestamp := findHeight(idStr)
	thorNodeMembers := getThorNodeMembers(pool, height)
	midlog.DebugF("Thornode rune addresses: %d assetOnly addresses: %d assetToRuneMap: %d",
		len(thorNodeMembers.RuneMemberUnits),
		len(thorNodeMembers.AssetMemberUnits),
		len(thorNodeMembers.AssetToRuneMap))

	midgardMembers := getMidgardMembers(ctx, pool, timestamp)
	midlog.DebugF("Midgard rune addresses: %d assetOnly addresses: %d assetToRuneMap: %d",
		len(midgardMembers.RuneMemberUnits),
		len(midgardMembers.AssetMemberUnits),
		len(midgardMembers.AssetToRuneMap))

	memberDiff(thorNodeMembers, midgardMembers)
}

func CheckAllPoolsStructured() {
	if flag.NArg() != 2 {
		fmt.Println("provide 2 args!")
		flag.Usage()
		return
	}

	ctx := context.Background()
	idStr := flag.Arg(1)

	height, timestamp := findHeight(idStr)

	poolsWithStatus, err := timeseries.GetPoolsStatuses(ctx, timestamp)
	if err != nil {
		midlog.FatalE(err, "Error getting Midgard pool status")
	}
	sortedPools := []string{}
	for k := range poolsWithStatus {
		sortedPools = append(sortedPools, k)
	}
	sort.Strings(sortedPools)
	for _, pool := range sortedPools {
		status := poolsWithStatus[pool]
		if status == "suspended" {
			continue
		}
		thorNodeMembers := getThorNodeMembers(pool, height)
		midlog.DebugF(
			"Thornode rune addresses: %d assetOnly addresses: %d assetToRuneMap: %d",
			len(thorNodeMembers.RuneMemberUnits),
			len(thorNodeMembers.AssetMemberUnits),
			len(thorNodeMembers.AssetToRuneMap))

		midgardMembers := getMidgardMembers(ctx, pool, timestamp)
		midlog.DebugF(
			"Midgard rune addresses: %d assetOnly addresses: %d assetToRuneMap: %d",
			len(midgardMembers.RuneMemberUnits), len(midgardMembers.AssetMemberUnits), len(midgardMembers.AssetToRuneMap))

		saveStructuredDiffs(pool, thorNodeMembers, midgardMembers)
	}
	printStructuredDiffs()
}

type MemberMap struct {
	// RuneMemberUnits is keyed by rune address if the member has one, otherwise asset address
	RuneMemberUnits map[string]int64

	// Only if it doesn't have a rune address
	AssetMemberUnits map[string]int64

	// Disjoint with AssetMemberUnits
	AssetToRuneMap map[string]string
}

func NewMemberMap() MemberMap {
	return MemberMap{
		RuneMemberUnits:  map[string]int64{},
		AssetMemberUnits: map[string]int64{},
		AssetToRuneMap:   map[string]string{},
	}
}

func check(ok bool, v ...interface{}) {
	if !ok {
		log.Fatal(v...)
	}
}

func (x *MemberMap) AddMemberSimple(m MemberChange) {
	rAddr := m.RuneAddress
	aAddr := m.AssetAddress

	if rAddr != "" {
		x.RuneMemberUnits[rAddr] += m.Units
	} else {
		check(aAddr != "", "Empty rune and asset address")
		x.AssetMemberUnits[aAddr] += m.Units
	}
}

// If there is an asset address without rune address
// then it looks for previous usage of the asset address to find an adequate rune adresses.
func (x *MemberMap) AddMemberClustered(m MemberChange) {
	rAddr := m.RuneAddress
	aAddr := m.AssetAddress

	if rAddr != "" {
		x.RuneMemberUnits[rAddr] += m.Units

		if aAddr != "" {
			assetUnits, previouslyAssetOnly := x.AssetMemberUnits[aAddr]
			if previouslyAssetOnly {
				x.RuneMemberUnits[rAddr] += assetUnits
				delete(x.AssetMemberUnits, aAddr)
			}

			previousRuneAddr, assetAddrAlreadyRegistered := x.AssetToRuneMap[aAddr]
			if assetAddrAlreadyRegistered {
				if previousRuneAddr != rAddr {
					midlog.FatalF("AssetAddress registered with multiple rune addresses %s %s",
						rAddr, previousRuneAddr)
				}
			} else {
				x.AssetToRuneMap[aAddr] = m.RuneAddress
			}
		}
	} else {
		check(aAddr != "", "Empty rune and asset address")
		previousRuneAddr, hasRunePair := x.AssetToRuneMap[aAddr]
		if hasRunePair {
			x.RuneMemberUnits[previousRuneAddr] += m.Units
		} else {
			x.AssetMemberUnits[aAddr] += m.Units
		}
	}
}

func (x *MemberMap) RemoveZero() {
	for k, v := range x.RuneMemberUnits {
		if v == 0 {
			delete(x.RuneMemberUnits, k)
		}
	}
	for k, v := range x.AssetMemberUnits {
		if v == 0 {
			delete(x.AssetMemberUnits, k)
		}
	}
}

func mapSum(m map[string]int64) int64 {
	var ret int64
	for _, v := range m {
		ret += v
	}
	return ret
}

func (x *MemberMap) TotalUnits() int64 {
	return mapSum(x.RuneMemberUnits) + mapSum(x.AssetMemberUnits)
}

func getThorNodeMembers(pool string, height int64) MemberMap {
	midlog.InfoF("Checking pool units sum. Pool: %s Height: %d", pool, height)

	thorNodeURL := config.Global.ThorChain.ThorNodeURL

	var summary ThorNodeSummary
	queryThorNode(thorNodeURL, "/pool/"+pool, height, &summary)
	midlog.InfoF("ThorNode global units: %d", summary.TotalUnits)

	var thornodeBreakdown []MemberChange
	queryThorNode(thorNodeURL, "/pool/"+pool+"/liquidity_providers", height, &thornodeBreakdown)

	ret := NewMemberMap()

	var sum2 int64
	for _, member := range thornodeBreakdown {
		sum2 += member.Units
		ret.AddMemberSimple(member)
	}
	midlog.InfoF("ThorNode units per member summed up: %d", sum2)
	if sum2 == summary.TotalUnits {
		midlog.Info("thornode is consistent")
	} else {
		midlog.FatalF(
			"thornode INCONSISTENT.\nPools Total units: %d\n Member units sum: %d\nDiff: %d",
			summary.TotalUnits,
			sum2,
			summary.TotalUnits-sum2,
		)
	}

	ret.RemoveZero()
	return ret
}

func queryThorNode(thorNodeUrl string, urlPath string, height int64, dest interface{}) {
	url := thorNodeUrl + urlPath
	if 0 < height {
		url += "?height=" + strconv.FormatInt(height, 10)
	}
	midlog.DebugF("Querying thornode: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		midlog.FatalE(err, "Querying ThorNode")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal(body, dest)
	if err != nil {
		midlog.FatalE(err, "Error unmarshaling ThorNode response")
	}
}

func getMidgardMembers(ctx context.Context, pool string, timestamp db.Nano) MemberMap {
	ret := NewMemberMap()

	addQ := `
		SELECT rune_addr, asset_addr, stake_units
		FROM deposit_events
		WHERE pool = $1 and block_timestamp <= $2
		ORDER BY block_timestamp
	`
	addRows, err := db.Query(ctx, addQ, pool, timestamp)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer addRows.Close()

	for addRows.Next() {
		var runeAddress, assetAddress sql.NullString
		var add MemberChange
		err := addRows.Scan(
			&runeAddress,
			&assetAddress,
			&add.Units)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
		if runeAddress.Valid {
			add.RuneAddress = runeAddress.String
		}
		if assetAddress.Valid {
			add.AssetAddress = assetAddress.String
		}
		ret.AddMemberSimple(add)
	}

	withdrawQ := `
		SELECT from_addr, stake_units
		FROM withdraw_events
		WHERE pool = $1 and block_timestamp <= $2
		ORDER BY block_timestamp
	`

	withdrawRows, err := db.Query(ctx, withdrawQ, pool, timestamp)
	if err != nil {
		midlog.FatalE(err, "Query error")
	}
	defer withdrawRows.Close()

	for withdrawRows.Next() {
		var fromAddr string
		var units int64
		err := withdrawRows.Scan(
			&fromAddr,
			&units)
		if err != nil {
			midlog.FatalE(err, "Query error")
		}
		withdraw := MemberChange{Units: -units}
		if record.AddressIsRune(fromAddr) {
			withdraw.RuneAddress = fromAddr
		} else {
			withdraw.AssetAddress = fromAddr
		}
		ret.AddMemberClustered(withdraw)
	}

	ret.RemoveZero()
	return ret
}

func mapDiff(thorNodeMap map[string]int64, midgardMap map[string]int64) {
	diffCount := 0
	for k, tValue := range thorNodeMap {
		mValue, mOk := midgardMap[k]
		if !mOk {
			midlog.WarnF("Missing address in Midgard: %s ThorNode units: %d", k, tValue)
			diffCount++
		} else if mValue != tValue {
			midlog.WarnF(
				"Mismatch units for address: %s  ThorNode: %d  Midgard: %d  diff: %d",
				k, tValue, mValue, tValue-mValue)
			diffCount++
		}
	}
	for k, mValue := range midgardMap {
		_, tOk := thorNodeMap[k]
		if !tOk {
			midlog.WarnF("Extra address in Midgard: %s  Midgard units: %d", k, mValue)
			diffCount++
		}
	}
	if diffCount == 0 {
		midlog.Info("No difference")
	}
}

func memberDiff(thorNodeMembers MemberMap, midgardMembers MemberMap) {
	midlog.Info("Checking Rune adresses")
	mapDiff(thorNodeMembers.RuneMemberUnits, midgardMembers.RuneMemberUnits)
	midlog.Info("Checking Asset adresses")
	mapDiff(thorNodeMembers.AssetMemberUnits, midgardMembers.AssetMemberUnits)

	thorNodeUnits := thorNodeMembers.TotalUnits()
	midgardUnits := midgardMembers.TotalUnits()
	if thorNodeUnits != midgardUnits {
		midlog.WarnF("Total units mismatch. ThorNode: %d  Midgard: %d", thorNodeUnits, midgardUnits)
	} else {
		midlog.Info("Total units are equal")
	}
}

var structuredBuff strings.Builder

func saveStructuredDiffs(pool string, thorNodeMembers MemberMap, midgardMembers MemberMap) {
	diffValue := map[string]int64{}
	accumulate := func(thorNodeMap map[string]int64, midgardMap map[string]int64) {
		for k, tValue := range thorNodeMap {
			mValue, mOk := midgardMap[k]
			if !mOk {
				diffValue[k] = tValue
			} else if mValue != tValue {
				diffValue[k] = tValue - mValue
			}
		}
		for k, mValue := range midgardMap {
			_, tOk := thorNodeMap[k]
			if !tOk {
				diffValue[k] = -mValue
			}
		}
	}
	accumulate(thorNodeMembers.RuneMemberUnits, midgardMembers.RuneMemberUnits)
	accumulate(thorNodeMembers.AssetMemberUnits, midgardMembers.AssetMemberUnits)

	keys := []string{}
	for k := range diffValue {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := diffValue[k]
		fmt.Fprintf(&structuredBuff, `{"%s", "%s", %d},`+"\n", pool, k, v)
	}
}

func printStructuredDiffs() {
	midlog.InfoF("Needed changes to Midgard:\n%v", structuredBuff.String())
}
