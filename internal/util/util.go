package util

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"

	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

// Chains work with integers which represent fixed point decimals.
// E.g. on BTC 1 is 1e-8 bitcoin, but on ETH 1 is 1e-18 ethereum.
// This information is not important for Midgard, all the values are converted to E8 by ThorNode
// before they are sent to Midgard.
// This information is gathered only for clients.
type NativeDecimalMap map[string]NativeDecimalSingle

type NativeDecimalSingle struct {
	NativeDecimals int64    `json:"decimals"` // -1 means that only the asset name was observed without the decimal count.
	AssetSeen      []string `json:"asset_seen"`
	DecimalSource  []string `json:"decimal_source"`
}

func IntStr(v int64) string {
	return strconv.FormatInt(v, 10)
}

type Asset struct {
	Chain  string
	Ticker string
	Symbol string
	Synth  bool
}

func AssetFromString(s string) (asset Asset) {
	var parts []string
	var sym string
	if strings.Count(s, "/") > 0 {
		parts = strings.SplitN(s, "/", 2)
		asset.Synth = true
	} else {
		parts = strings.SplitN(s, ".", 2)
		asset.Synth = false
	}

	if len(parts) == 1 {
		asset.Chain = "THOR"
		sym = parts[0]
	} else {
		asset.Chain = strings.ToUpper(parts[0])
		sym = parts[1]
	}

	parts = strings.SplitN(sym, "-", 2)
	asset.Symbol = strings.ToUpper(sym)
	asset.Ticker = strings.ToUpper(parts[0])

	return
}

func ConvertNativePoolToSynth(poolName string) string {
	return strings.Replace(poolName, ".", "/", 1)
}

func ConvertSynthPoolToNative(poolName string) string {
	return strings.Replace(poolName, "/", ".", 1)
}

func ConsumeUrlParam(urlParams *url.Values, key string) (value string) {
	value = urlParams.Get(key)
	urlParams.Del(key)
	return
}

func CheckUrlEmpty(urlParams url.Values) miderr.Err {
	for k := range urlParams {
		return miderr.BadRequestF("Unknown key: %s", k)
	}
	return nil
}

// It's like bytes.ToLower but returns nil for nil.
func ToLowerBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	return bytes.ToLower(b)
}

type Number interface {
	int64 | float64
}

func Max[T Number](x, y T) T {
	if y < x {
		return x
	} else {
		return y
	}
}

func MustParseInt64(v string) int64 {
	res, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		midlog.ErrorE(err, "Cannot parse int64")
	}
	return res
}

// From THORNode
// MEMO: TXTYPE:STATE1:STATE2:STATE3:FINALMEMO
type TxType string

const (
	TxUnknown         TxType = "unknown"
	TxAdd             TxType = "add"
	TxWithdraw        TxType = "withdraw"
	TxSwap            TxType = "swap"
	TxLimitOrder      TxType = "limitOrder"
	TxOutbound        TxType = "outbound"
	TxDonate          TxType = "donate"
	TxBond            TxType = "bond"
	TxUnbond          TxType = "unbond"
	TxLeave           TxType = "leave"
	TxYggdrasilFund   TxType = "yggdrasilFund"
	TxYggdrasilReturn TxType = "yggdrasilReturn"
	TxReserve         TxType = "reserve"
	TxRefund          TxType = "refund"
	TxMigrate         TxType = "migrate"
	TxRagnarok        TxType = "ragnarok"
	TxSwitch          TxType = "switch"
	TxNoOp            TxType = "noOp"
	TxConsolidate     TxType = "consolidate"
	TxTHORName        TxType = "thorname"
	TxLoanOpen        TxType = "loanOpen"
	TxLoanRepayment   TxType = "loanRepayment"
)

var StringToTxTypeMap = map[string]TxType{
	"add":         TxAdd,
	"+":           TxAdd,
	"withdraw":    TxWithdraw,
	"wd":          TxWithdraw,
	"-":           TxWithdraw,
	"swap":        TxSwap,
	"s":           TxSwap,
	"=":           TxSwap,
	"limito":      TxLimitOrder,
	"lo":          TxLimitOrder,
	"out":         TxOutbound,
	"donate":      TxDonate,
	"d":           TxDonate,
	"bond":        TxBond,
	"unbond":      TxUnbond,
	"leave":       TxLeave,
	"yggdrasil+":  TxYggdrasilFund,
	"yggdrasil-":  TxYggdrasilReturn,
	"reserve":     TxReserve,
	"refund":      TxRefund,
	"migrate":     TxMigrate,
	"ragnarok":    TxRagnarok,
	"switch":      TxSwitch,
	"noop":        TxNoOp,
	"consolidate": TxConsolidate,
	"name":        TxTHORName,
	"n":           TxTHORName,
	"~":           TxTHORName,
	"$+":          TxLoanOpen,
	"loan+":       TxLoanOpen,
	"$-":          TxLoanRepayment,
	"loan-":       TxLoanRepayment,
}
