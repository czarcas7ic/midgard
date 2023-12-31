// Sometimes ThorNode state is updated but the events doesn't reflect that perfectly.
//
// In these cases we open a bug report so future events are correct, but the old events will
// stay the same, and we apply these corrections to the existing events.
package record

import (
	"gitlab.com/thorchain/midgard/internal/db"
)

const ChainIDMainnet202104 = "7D37DEF6E1BE23C912092069325C4A51E66B9EF7DDBDE004FF730CFABC0307B1"

func CorrectWithdaws(withdraw *Unstake, meta *Metadata) {
	f, ok := WithdrawCorrections[meta.BlockHeight]
	if ok {
		f(withdraw, meta)
	}
}

func AddMissingEvents(d *Demux, meta *Metadata) {
	f, ok := AdditionalEvents[meta.BlockHeight]
	if ok {
		f(d, meta)
	}
	// TODO(muninn): move fixes into separate files
	switch db.ChainID() {
	case ChainIDMainnet202104:
		// Chaosnet started on 2021-04-10
		switch meta.BlockHeight {
		case 12824:
			// Genesis node bonded rune and became listed as Active without any events.
			d.reuse.UpdateNodeAccountStatus = UpdateNodeAccountStatus{
				NodeAddr: []byte("thor1xfqaqhk5r6x9hdwlvmye0w9agv8ynljacmxulf"),
				Former:   []byte("Ready"),
				Current:  []byte("Active"),
			}
			Recorder.OnUpdateNodeAccountStatus(&d.reuse.UpdateNodeAccountStatus, meta)
		case 63519:
			// Fix was applied in https://gitlab.com/thorchain/thornode/-/merge_requests/1643
			//
			// TODO(acsaba): add PR/issue id as context for this, update reason.
			// TODO(muninn): clarify with core team about
			reason := []byte("Midgard fix for assymetric rune withdraw problem")
			d.reuse.Unstake = Unstake{
				FromAddr:   []byte("thor1tl9k7fjvye4hkvwdnl363g3f2xlpwwh7k7msaw"),
				Chain:      []byte("BNB"),
				Pool:       []byte("BNB.BNB"),
				Asset:      []byte("THOR.RUNE"),
				ToAddr:     reason,
				Memo:       reason,
				Tx:         reason,
				EmitRuneE8: 1999997,
				StakeUnits: 1029728,
			}
			Recorder.OnUnstake(&d.reuse.Unstake, meta)
		}

	case "8371BCEB807EEC52AC6A23E2FFC300D18FD3938374D3F4FC78EEB5FE33F78AF7":
		// Testnet started on 2021-04-10
		if meta.BlockHeight == 28795 {
			// Withdraw id 57BD5B26B0D78CD4A0340F8ECA2356B23B029157E43DE99EF03114CC15577C8A
			// failed, still pool balances were changed.
			// Fix for future was submitted on Thornode:
			// https://gitlab.com/thorchain/thornode/-/merge_requests/1634
			d.reuse.PoolBalanceChange = PoolBalanceChange{
				Asset:    []byte("LTC.LTC"),
				RuneAmt:  1985607,
				RuneAdd:  false,
				AssetAmt: 93468,
				AssetAdd: false,
				Reason:   "Midgard fix: Reserve didn't have rune for gas",
			}
			Recorder.OnPoolBalanceChange(&d.reuse.PoolBalanceChange, meta)
		}
	default:
	}
}

type AddEventsFunc func(d *Demux, meta *Metadata)
type AddEventsFuncMap map[int64]AddEventsFunc

var AdditionalEvents AddEventsFuncMap

type WithdrawCorrection func(withdraw *Unstake, meta *Metadata)
type WithdrawCorrectionMap map[int64]WithdrawCorrection

var WithdrawCorrections WithdrawCorrectionMap

func LoadCorrections(chainID string) {
	if chainID == "" {
		return
	}
	AdditionalEvents = AddEventsFuncMap{}
	WithdrawCorrections = WithdrawCorrectionMap{}

	LoadCorrectionsWithdrawImpLoss(chainID)
	loadWithdrawForwardedAssetCorrections(chainID)
}

// Note: we have copypasted Add functionsn because golang doesn't has templates yet.
func (m AddEventsFuncMap) Add(height int64, f AddEventsFunc) {
	fOrig, alreadyExists := m[height]
	if alreadyExists {
		m[height] = func(d *Demux, meta *Metadata) {
			fOrig(d, meta)
			f(d, meta)
		}
		return
	}
	m[height] = f
}

func (m WithdrawCorrectionMap) Add(height int64, f WithdrawCorrection) {
	fOrig, alreadyExists := m[height]
	if alreadyExists {
		m[height] = func(withdraw *Unstake, meta *Metadata) {
			fOrig(withdraw, meta)
			f(withdraw, meta)
		}
		return
	}
	m[height] = f
}
