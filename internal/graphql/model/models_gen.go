// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

import (
	"fmt"
	"io"
	"strconv"
)

type Asset struct {
	// Asset name
	Asset string `json:"asset"`
	// Date this asset was created
	Created string `json:"created"`
	// Current price of the asset in RUNE
	Price float64 `json:"price"`
}

type BondMetrics struct {
	// Bond Metrics for active nodes
	Active *BondMetricsStat `json:"active"`
	// Bond Metrics for standby nodes
	Standby *BondMetricsStat `json:"standby"`
}

type BondMetricsStat struct {
	// Average bond of nodes
	AverageBond float64 `json:"averageBond"`
	// Maximum bond of nodes
	MaximumBond int64 `json:"maximumBond"`
	// Median bond of nodes
	MedianBond int64 `json:"medianBond"`
	// Minimum bond of nodes
	MinimumBond int64 `json:"minimumBond"`
	// Total bond of nodes
	TotalBond int64 `json:"totalBond"`
}

type JailInfo struct {
	NodeAddr      string `json:"nodeAddr"`
	ReleaseHeight int64  `json:"releaseHeight"`
	Reason        string `json:"reason"`
}

type Network struct {
	// List of active bonds
	ActiveBonds []*int64 `json:"activeBonds"`
	// Number of active bonds
	ActiveNodeCount int64        `json:"activeNodeCount"`
	BondMetrics     *BondMetrics `json:"bondMetrics"`
	// List of standby bonds
	StandbyBonds []*int64 `json:"standbyBonds"`
	// Number of standby bonds
	StandbyNodeCount int64 `json:"standbyNodeCount"`
	// Total Rune Staked in Pools
	TotalStaked int64 `json:"totalStaked"`
}

type Node struct {
	// Public keys of node
	PublicKeys *PublicKeys `json:"publicKeys"`
	// Node address
	Address string `json:"address"`
	// Node status
	Status string `json:"status"`
	// Amount bonded
	Bond int64 `json:"bond"`
	// Whether not was requested to leave
	RequestedToLeave bool `json:"requestedToLeave"`
	// Whether not was forced to leave
	ForcedToLeave bool `json:"forcedToLeave"`
	// The leave height
	LeaveHeight int64 `json:"leaveHeight"`
	// Node IP address
	IPAddress string `json:"ipAddress"`
	// Node version
	Version string `json:"version"`
	// Node slash points
	SlashPoints int64 `json:"slashPoints"`
	// Node jail info
	Jail *JailInfo `json:"jail"`
	// Node current award
	CurrentAward int64 `json:"currentAward"`
}

// The current state of a pool.
// To get historical data or averages use the history queries.
type Pool struct {
	// Asset name in the format "CHAIN.TICKER-SYMBOL" e.g. "BNB.BTCB-101
	Asset string `json:"asset"`
	// Pool Status (bootstrapped, enabled, disabled)
	Status string `json:"status"`
	// Current price of the asset in RUNE
	Price float64 `json:"price"`
	// Total stake units (LP shares) in that pool
	Units int64 `json:"units"`
	// Pool's Stakes
	Stakes *PoolStakes `json:"stakes"`
	// Pool's Depth
	Depth *PoolDepth `json:"depth"`
	// Pool's ROI
	Roi *Roi `json:"roi"`
}

type PoolDepth struct {
	// Current asset balance in ASSET
	AssetDepth int64 `json:"assetDepth"`
	// Current balance in RUNE
	RuneDepth int64 `json:"runeDepth"`
	// Combined total balance: 2 * runeDepth
	PoolDepth int64 `json:"poolDepth"`
}

type PoolHistoryBucket struct {
	// The starting timestamp of the interval
	Time int64 `json:"time"`
	// The amount of Rune in the pool at the beginning of this period
	Rune int64 `json:"rune"`
	// The amount of Asset in the pool at the beginning of this period
	Asset int64 `json:"asset"`
	// Price at the beginning, it's equal to rune/asset
	Price float64 `json:"price"`
}

type PoolHistoryDetails struct {
	// Overall Depth History Stats for given time interval
	Meta *PoolHistoryMeta `json:"meta"`
	// Depth History Stats by time interval
	Intervals []*PoolHistoryBucket `json:"intervals"`
}

type PoolHistoryMeta struct {
	// The beginning timestamp of the first interval. Can be smaller then from
	First int64 `json:"first"`
	// The beginning timestamp of the last interval. It is smaller then until
	Last int64 `json:"last"`
	// The rune at the beginning of the first interval
	//   or at from timestamp if it was given.
	RuneFirst int64 `json:"runeFirst"`
	// The rune amount at the beginning of the last interval.
	RuneLast int64 `json:"runeLast"`
	// The asset at the beginning of the first interval
	//   or at from timestamp if it was given.
	AssetFirst int64 `json:"assetFirst"`
	// The asset amount at the beginning of the last interval.
	AssetLast int64 `json:"assetLast"`
	// runeFirst / assetFirst
	PriceFirst float64 `json:"priceFirst"`
	// runeLast / assetLast
	PriceLast float64 `json:"priceLast"`
}

type PoolStakeHistory struct {
	// Overall Stake History Stats for given time interval
	Meta *PoolStakeHistoryMeta `json:"meta"`
	// Stake History Stats by time interval
	Intervals []*PoolStakeHistoryBucket `json:"intervals"`
}

type PoolStakeHistoryBucket struct {
	// The starting timestamp of the interval
	Time int64 `json:"time"`
	// Total number of stakes in this period (TxCount)
	Count int64 `json:"count"`
	// Total volume of stakes in RUNE (RuneE8Total)
	RuneVolume int64 `json:"runeVolume"`
	// Total volume of stakes in Asset (AssetE8Total)
	AssetVolume int64 `json:"assetVolume"`
	// Total stake units (StakeUnitsTotal)
	Units int64 `json:"units"`
}

type PoolStakeHistoryMeta struct {
	// The beginning timestamp of the first interval. Can be smaller then from
	First int64 `json:"first"`
	// The beginning timestamp of the last interval. It is smaller then until
	Last int64 `json:"last"`
	// Total number of stakes in this query (TxCount)
	Count int64 `json:"count"`
	// Total volume of stakes in RUNE (RuneE8Total)
	RuneVolume int64 `json:"runeVolume"`
	// Total volume of stakes in Asset (AssetE8Total)
	AssetVolume int64 `json:"assetVolume"`
	// Total stake units (StakeUnitsTotal)
	Units int64 `json:"units"`
}

type PoolStakes struct {
	// Sum of all ASSET stakes for all time since pool creation denominated in ASSET
	AssetStaked int64 `json:"assetStaked"`
	// Sum of all RUNE stakes for all time since pool creation denominated in RUNE
	RuneStaked int64 `json:"runeStaked"`
	// RUNE value staked total: runeStakedTotal + (assetStakedTotal * assetPrice)
	PoolStaked int64 `json:"poolStaked"`
}

// First and last buckets will have truncated stats if
// from/until are not on interval boundaries.
type PoolVolumeHistory struct {
	// Overall Swap History Stats for given time interval
	Meta *PoolVolumeHistoryMeta `json:"meta"`
	// Swaps History Stats by time interval
	Intervals []*PoolVolumeHistoryBucket `json:"intervals"`
}

type PoolVolumeHistoryBucket struct {
	// The starting timestamp of the interval
	Time int64 `json:"time"`
	// Combined stats for swaps from asset to rune and from rune to asset
	Combined *VolumeStats `json:"combined"`
	// Stats for swaps from asset to rune
	ToRune *VolumeStats `json:"toRune"`
	// Stats for swaps from rune to asset
	ToAsset *VolumeStats `json:"toAsset"`
}

type PoolVolumeHistoryMeta struct {
	// The beginning timestamp of the first interval. Can be smaller then from
	First int64 `json:"first"`
	// The beginning timestamp of the last interval. It is smaller then until
	Last int64 `json:"last"`
	// Combined stats for swaps from asset to rune and from rune to asset
	Combined *VolumeStats `json:"combined"`
	// Stats for swaps from asset to rune
	ToRune *VolumeStats `json:"toRune"`
	// Stats for swaps from rune to asset
	ToAsset *VolumeStats `json:"toAsset"`
}

type PublicKeys struct {
	// secp256k1 public key
	Secp256k1 string `json:"secp256k1"`
	// ed25519 public key
	Ed25519 string `json:"ed25519"`
}

type Roi struct {
	// Current ASSET ROI
	AssetRoi float64 `json:"assetROI"`
	// Current RUNE ROI
	RuneRoi float64 `json:"runeROI"`
}

type Staker struct {
	// Unique staker address
	Address string `json:"address"`
	// List of staked pools
	PoolsArray []*string `json:"poolsArray"`
	// Total staked (in RUNE) across all pools.
	TotalStaked int64 `json:"totalStaked"`
}

type Stats struct {
	// Daily active users (unique addresses interacting)
	DailyActiveUsers int64 `json:"dailyActiveUsers"`
	// Daily transactions
	DailyTx int64 `json:"dailyTx"`
	// Monthly active users
	MonthlyActiveUsers int64 `json:"monthlyActiveUsers"`
	// Monthly transactions
	MonthlyTx int64 `json:"monthlyTx"`
	// Total buying transactions
	TotalAssetBuys int64 `json:"totalAssetBuys"`
	// Total selling transactions
	TotalAssetSells int64 `json:"totalAssetSells"`
	// Total RUNE balances
	TotalDepth int64 `json:"totalDepth"`
	// Total staking transactions
	TotalStakeTx int64 `json:"totalStakeTx"`
	// Total staked (in RUNE Value).
	TotalStaked int64 `json:"totalStaked"`
	// Total transactions
	TotalTx int64 `json:"totalTx"`
	// Total unique swappers \u0026 stakers
	TotalUsers int64 `json:"totalUsers"`
	// Total (in RUNE Value) of all assets swapped since start.
	TotalVolume int64 `json:"totalVolume"`
	// Total withdrawing transactions
	TotalWithdrawTx int64 `json:"totalWithdrawTx"`
}

// Stats about swaps in any given interval
// This can represent volume of swaps from or to RUNE and also combined stats.
type VolumeStats struct {
	// Total number of swaps in this period (TxCount)
	Count int64 `json:"count"`
	// Total volume of swaps in RUNE (RuneE8Total) in this period
	VolumeInRune int64 `json:"volumeInRune"`
	// Total fees in RUNE (LiqFeeInRuneE8Total) in this period
	FeesInRune int64 `json:"feesInRune"`
}

type Interval string

const (
	// 5 minute period
	IntervalMinute5 Interval = "MINUTE5"
	// Hour period
	IntervalHour Interval = "HOUR"
	// Day period
	IntervalDay Interval = "DAY"
	// Week period
	IntervalWeek Interval = "WEEK"
	// Month period
	IntervalMonth Interval = "MONTH"
	// Quarter period
	IntervalQuarter Interval = "QUARTER"
	// Year period
	IntervalYear Interval = "YEAR"
)

var AllInterval = []Interval{
	IntervalMinute5,
	IntervalHour,
	IntervalDay,
	IntervalWeek,
	IntervalMonth,
	IntervalQuarter,
	IntervalYear,
}

func (e Interval) IsValid() bool {
	switch e {
	case IntervalMinute5, IntervalHour, IntervalDay, IntervalWeek, IntervalMonth, IntervalQuarter, IntervalYear:
		return true
	}
	return false
}

func (e Interval) String() string {
	return string(e)
}

func (e *Interval) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = Interval(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid Interval", str)
	}
	return nil
}

func (e Interval) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// Time Interval used for querying histories
type LegacyInterval string

const (
	// 24 hour period
	LegacyIntervalDay LegacyInterval = "DAY"
	// 7 day period
	LegacyIntervalWeek LegacyInterval = "WEEK"
	// Month period
	LegacyIntervalMonth LegacyInterval = "MONTH"
)

var AllLegacyInterval = []LegacyInterval{
	LegacyIntervalDay,
	LegacyIntervalWeek,
	LegacyIntervalMonth,
}

func (e LegacyInterval) IsValid() bool {
	switch e {
	case LegacyIntervalDay, LegacyIntervalWeek, LegacyIntervalMonth:
		return true
	}
	return false
}

func (e LegacyInterval) String() string {
	return string(e)
}

func (e *LegacyInterval) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = LegacyInterval(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid LegacyInterval", str)
	}
	return nil
}

func (e LegacyInterval) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type NodeStatus string

const (
	NodeStatusActive   NodeStatus = "ACTIVE"
	NodeStatusStandby  NodeStatus = "STANDBY"
	NodeStatusDisabled NodeStatus = "DISABLED"
	NodeStatusJailed   NodeStatus = "JAILED"
)

var AllNodeStatus = []NodeStatus{
	NodeStatusActive,
	NodeStatusStandby,
	NodeStatusDisabled,
	NodeStatusJailed,
}

func (e NodeStatus) IsValid() bool {
	switch e {
	case NodeStatusActive, NodeStatusStandby, NodeStatusDisabled, NodeStatusJailed:
		return true
	}
	return false
}

func (e NodeStatus) String() string {
	return string(e)
}

func (e *NodeStatus) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = NodeStatus(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid NodeStatus", str)
	}
	return nil
}

func (e NodeStatus) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

type PoolOrderAttribute string

const (
	PoolOrderAttributeDepth  PoolOrderAttribute = "DEPTH"
	PoolOrderAttributeVolume PoolOrderAttribute = "VOLUME"
)

var AllPoolOrderAttribute = []PoolOrderAttribute{
	PoolOrderAttributeDepth,
	PoolOrderAttributeVolume,
}

func (e PoolOrderAttribute) IsValid() bool {
	switch e {
	case PoolOrderAttributeDepth, PoolOrderAttributeVolume:
		return true
	}
	return false
}

func (e PoolOrderAttribute) String() string {
	return string(e)
}

func (e *PoolOrderAttribute) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = PoolOrderAttribute(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid PoolOrderAttribute", str)
	}
	return nil
}

func (e PoolOrderAttribute) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
