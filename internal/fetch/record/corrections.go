// Sometimes ThorNode state is updated but the events doesn't reflect that perfectly.
//
// In these cases we open a bug report so future events are correct, but the old events will
// stay the same, and we apply these corrections to the existing events.
package record

import (
	"strconv"
	"strings"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
)

const MidgardBalanceCorrectionAddress = "MidgardBalanceCorrectionAddress"

func LoadCorrections(chainID string) {
	if chainID == "" {
		return
	}

	loadMainnet202104Corrections(chainID)
	loadTestnet202111Corrections(chainID)
	loadStagenetCorrections(chainID)
}

/////////////// Corrections for Missing Events

func AddMissingEvents(meta *Metadata) {
	f, ok := AdditionalEvents[meta.BlockHeight]
	if ok {
		f(meta)
	}
}

type (
	AddEventsFunc    func(meta *Metadata)
	AddEventsFuncMap map[int64]AddEventsFunc
)

var AdditionalEvents = AddEventsFuncMap{}

/////////////// Corrections for Withdraws

type KeepOrDiscard int

const (
	Keep KeepOrDiscard = iota
	Discard
)

func CorrectWithdraw(withdraw *Withdraw, meta *Metadata) (ret KeepOrDiscard) {
	if GlobalWithdrawCorrection != nil && GlobalWithdrawCorrection(withdraw, meta) == Discard {
		return Discard
	}

	if f, ok := WithdrawCorrections[meta.BlockHeight]; ok {
		return f(withdraw, meta)
	}
	return Keep
}

type (
	WithdrawCorrection    func(withdraw *Withdraw, meta *Metadata) KeepOrDiscard
	WithdrawCorrectionMap map[int64]WithdrawCorrection
)

var WithdrawCorrections = WithdrawCorrectionMap{}
var GlobalWithdrawCorrection WithdrawCorrection = nil

/////////////// Blacklist of fee events

func CorrectionsFeeEventIsOK(fee *Fee, meta *Metadata) bool {
	f, ok := FeeAcceptFuncs[meta.BlockHeight]
	if !ok {
		return true
	}
	return f(fee, meta)
}

type (
	FeeAcceptFunc func(fee *Fee, meta *Metadata) bool
	FeeAcceptMap  map[int64]FeeAcceptFunc
)

var FeeAcceptFuncs = FeeAcceptMap{}

func (m FeeAcceptMap) Add(height int64, f FeeAcceptFunc) {
	fOrig, alreadyExists := m[height]
	if alreadyExists {
		m[height] = func(fee *Fee, meta *Metadata) bool {
			accepted := fOrig(fee, meta)
			if !accepted {
				return false
			}
			return f(fee, meta)
		}
	} else {
		m[height] = f
	}
}

/////////////// Artificial deposits to fix member pool units.

type artificialUnitChange struct {
	Pool  string
	Addr  string
	Units int64
}

type artificialUnitChanges map[int64][]artificialUnitChange

func registerArtificialDeposits(unitChanges artificialUnitChanges) {
	addAddEvent := func(meta *Metadata) {
		changes, ok := unitChanges[meta.BlockHeight]
		if ok {
			for _, change := range changes {
				if 0 <= change.Units {
					deposit := Desposit{
						AddBase: AddBase{
							Pool: []byte(change.Pool),
						},
						StakeUnits: change.Units,
					}
					if AddressIsRune(change.Addr) {
						deposit.RuneAddr = []byte(change.Addr)
					} else {
						deposit.AssetAddr = []byte(change.Addr)
					}
					Recorder.OnDeposit(&deposit, meta)
				} else {
					withdraw := Withdraw{
						Pool:       []byte(change.Pool),
						Asset:      []byte(change.Pool),
						FromAddr:   []byte(change.Addr),
						ToAddr:     []byte(change.Addr),
						StakeUnits: -change.Units,
						Tx:         []byte(change.Addr + strconv.Itoa(int(-change.Units))),
						Chain:      []byte(strings.Split(change.Pool, ".")[0]),
						Memo:       []byte("Midgard Fix"),
					}
					Recorder.OnWithdraw(&withdraw, meta)
				}
			}
		}
	}
	for height := range unitChanges {
		AdditionalEvents.Add(height, addAddEvent)
	}
}

/////////////// Artificial pool balance changes to fix ThorNode/Midgard depth divergences.

type artificialPoolBallanceChange struct {
	Pool  string
	Rune  int64
	Asset int64
}

func absAndSign(x int64) (abs int64, pos bool) {
	if 0 <= x {
		return x, true
	} else {
		return -x, false
	}
}

func (x artificialPoolBallanceChange) toEvent() PoolBalanceChange {
	ret := PoolBalanceChange{
		Asset: []byte(x.Pool),
	}
	ret.RuneAmt, ret.RuneAdd = absAndSign(x.Rune)
	ret.AssetAmt, ret.AssetAdd = absAndSign(x.Asset)
	ret.Reason = "Fix in Midgard"
	return ret
}

type artificialPoolBallanceChanges map[int64][]artificialPoolBallanceChange

func registerArtificialPoolBallanceChanges(changes artificialPoolBallanceChanges, reason string) {
	addPoolBallanceChangeEvent := func(meta *Metadata) {
		changesAtHeight, ok := changes[meta.BlockHeight]
		if ok {
			for _, change := range changesAtHeight {
				poolBalanceChange := change.toEvent()
				poolBalanceChange.Reason = reason
				Recorder.OnPoolBalanceChange(&poolBalanceChange, meta)
			}
		}
	}
	for height := range changes {
		AdditionalEvents.Add(height, addPoolBallanceChangeEvent)
	}
}

/////////////// Old style withdraws

// Logic for withdraw changed since start of chaosnet 2021-04. This variable describes the height
// where the logic change happened.
var withdrawCoinKeptHeight int64 = 0

func (m AddEventsFuncMap) Add(height int64, f AddEventsFunc) {
	fOrig, alreadyExists := m[height]
	if alreadyExists {
		m[height] = func(meta *Metadata) {
			fOrig(meta)
			f(meta)
		}
		return
	}
	m[height] = f
}

func (m WithdrawCorrectionMap) Add(height int64, f WithdrawCorrection) {
	fOrig, alreadyExists := m[height]
	if alreadyExists {
		m[height] = func(withdraw *Withdraw, meta *Metadata) KeepOrDiscard {
			if fOrig(withdraw, meta) == Discard {
				return Discard
			}
			return f(withdraw, meta)
		}
		return
	}
	m[height] = f
}

/////////////// Block Corrections

func applyBlockCorrections(block *chain.Block) {
	applyTimestampCorrections(block)
}

/////////////// Timestamp corrections

var TimestampCorrections = map[int64]db.Second{}

func applyTimestampCorrections(block *chain.Block) {
	if sec, ok := TimestampCorrections[block.Height]; ok {
		block.Time = sec.ToTime()
	}
}
