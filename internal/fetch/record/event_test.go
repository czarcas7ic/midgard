package record

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
)

var GoldenAssets = []struct {
	Asset, Chain, Ticker, ID string
	Type                     CoinType
	NativeAsset              string
}{
	{"BTC.BTC", "BTC", "BTC", "", AssetNative, "BTC.BTC"},
	{"ETH.ETH", "ETH", "ETH", "", AssetNative, "ETH.ETH"},
	{"ETH.USDT-0xdac17f958d2ee523a2206206994597c13d831ec7", "ETH", "USDT", "0xdac17f958d2ee523a2206206994597c13d831ec7", AssetNative, "ETH.USDT-0xdac17f958d2ee523a2206206994597c13d831ec7"},
	{"BNB.BNB", "BNB", "BNB", "", AssetNative, "BNB.BNB"},
	{"BNB.RUNE-B1A", "BNB", "RUNE", "B1A", Rune, "BNB.RUNE-B1A"},
	{"THOR.RUNE", "THOR", "RUNE", "", Rune, "THOR.RUNE"},
	{"BNB/BNB", "BNB", "BNB", "", AssetSynth, "BNB.BNB"},
	{"ETH/USDT-0xdac17f958d2ee523a2206206994597c13d831ec7", "ETH", "USDT", "0xdac17f958d2ee523a2206206994597c13d831ec7", AssetSynth, "ETH.USDT-0xdac17f958d2ee523a2206206994597c13d831ec7"},
	{"", "", "", "", UnknownCoin, ""},
	{".", "", "", "", AssetNative, "."},
	{"-", "", "", "", UnknownCoin, "-"},
	{".-", "", "", "", AssetNative, ".-"},
	{"1.", "1", "", "", AssetNative, "1."},
	{"2.-", "2", "", "", AssetNative, "2.-"},
	{"A", "", "A", "", UnknownCoin, "A"},
	{".B", "", "B", "", AssetNative, ".B"},
	{"C-", "", "C", "", UnknownCoin, "C-"},
	{".D-", "", "D", "", AssetNative, ".D-"},
	{"-a", "", "", "a", UnknownCoin, "-a"},
	{".-b", "", "", "b", AssetNative, ".-b"},
}

func TestParseAsset(t *testing.T) {
	for _, gold := range GoldenAssets {
		chain, ticker, ID := ParseAsset([]byte(gold.Asset))
		if string(chain) != gold.Chain || string(ticker) != gold.Ticker || string(ID) != gold.ID {
			t.Errorf("%q got [%q %q %q], want [%q %q %q]", gold.Asset, chain, ticker, ID, gold.Chain, gold.Ticker, gold.ID)
		}
	}
}

func TestGetCoinType(t *testing.T) {
	for _, gold := range GoldenAssets {
		coinType := GetCoinType([]byte(gold.Asset))
		if coinType != gold.Type {
			t.Errorf("%q got [%q], want [%q]", gold.Asset, coinType, gold.Type)
		}
	}
}

func TestGetNativeAsset(t *testing.T) {
	for _, gold := range GoldenAssets {
		coinType := GetCoinType([]byte(gold.Asset))
		if coinType != gold.Type {
			t.Errorf("%q got [%q], want [%q]", gold.Asset, coinType, gold.Type)
		}
	}
}

func TestOutbound(t *testing.T) {
	var event Outbound
	err := event.LoadTendermint(toAttrs(map[string]string{
		"chain":    "BTC",
		"coin":     "23282731 BTC.BTC",
		"from":     "bcrt1q53nknrl2d2nmvguhhvacd4dfsm4jlv8c46ed3y",
		"id":       "0000000000000000000000000000000000000000000000000000000000000000",
		"in_tx_id": "04FFE1117647700F48F678DF53372D503F31C745D6DDE3599D9CB6381188620E",
		"memo":     "OUTBOUND:04FFE1117647700F48F678DF53372D503F31C745D6DDE3599D9CB6381188620E",
		"to":       "bcrt1q0s4mg25tu6termrk8egltfyme4q7sg3h8kkydt",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if event.Tx != nil {
		t.Errorf("got tx %#x, want nil for zeros only", event.Tx)
	}
}

func TestScheduledOutbound(t *testing.T) {
	var event ScheduledOutbound
	err := event.LoadTendermint(toAttrs(map[string]string{
		"chain":              "BTC",
		"coin_amount":        "3137611526",
		"coin_asset":         "BTC.BTC",
		"coin_decimals":      "0",
		"gas_rate":           "22",
		"in_hash":            "40A3D546F349F9CF8E907B6676E6187AD01F24C76AD9D0B9D2958E8C9E059E2C",
		"max_gas_amount_0":   "22500",
		"max_gas_asset_0":    "BTC.BTC",
		"max_gas_decimals_0": "8",
		"memo":               "OUT:40A3D546F349F9CF8E907B6676E6187AD01F24C76AD9D0B9D2958E8C9E059E2C",
		"to_address":         "bc1qwxwl5l209je4c2ycr8hc7dq7jqfptk23esmn5s",
		"vault_pub_key":      "thorpub1addwnpepqg2qqe0fc9h09q2hy3lkmyprkfe2r4ycymxxdlguzae52z7k9wk9yvmh8eu",
	}))
	if err != nil {
		t.Fatal(err)
	}
}

// DoubleAsset returns the follow-up pool or nil. Follow-ups occur in so-called
// double-swaps, whereby the trader sells .Pool asset with this event, and then
// consecutively buys DoubleAsset in another event (with the same .Tx).
func (e *Swap) DoubleAsset() (asset []byte) {
	if IsRune(e.ToAsset) {
		params := bytes.SplitN(e.Memo, []byte{':'}, 3)
		if len(params) > 1 && !bytes.Equal(params[1], e.Pool) {
			return params[1]
		}
	}
	return nil
}

func TestSwap(t *testing.T) {
	var event Swap
	err := event.LoadTendermint(toAttrs(map[string]string{
		"chain":                 "BNB",
		"coin":                  "500000 BNB.BNB",
		"emit_asset":            "1 THOR.RUNE",
		"from":                  "tbnb157dxmw9jz5emuf0apj4d6p3ee42ck0uwksxfff",
		"id":                    "0F1DE3EC877075636F21AF1E7399AA9B9C710A4989E61A9F5942A78B9FA96621",
		"liquidity_fee":         "259372",
		"liquidity_fee_in_rune": "259372",
		"memo":                  "SWAP:BTC.BTC:bcrt1qqqnde7kqe5sf96j6zf8jpzwr44dh4gkd3ehaqh",
		"pool":                  "BNB.BNB",
		"swap_target":           "1",
		"to":                    "tbnb153nknrl2d2nmvguhhvacd4dfsm4jlv8c87nscv",
		"swap_slip":             "33",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if event.FromE8 != 500000 || string(event.FromAsset) != "BNB.BNB" {
		t.Errorf(`got from %d %q with "coin": "500000 BNB.BNB"`, event.FromE8, event.FromAsset)
	}
	if got := event.DoubleAsset(); string(got) != "BTC.BTC" {
		t.Errorf("got asset %q, want BitCoin", got)
	}
}

func TestRefund(t *testing.T) {
	var event Refund
	err := event.LoadTendermint(toAttrs(map[string]string{
		"chain":  "BNB",
		"code":   "105",
		"coin":   "150000000 BNB.BNB, 50000000000 BNB.RUNE-67C",
		"from":   "tbnb189az9plcke2c00vns0zfmllfpfdw67dtv25kgx",
		"id":     "98C1864036571E805BB0E0CCBAFF0F8D80F69BDEA32D5B26E0DDB95301C74D0C",
		"memo":   "",
		"reason": "memo can't be empty",
		"to":     "tbnb153nknrl2d2nmvguhhvacd4dfsm4jlv8c87nscv",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if event.AssetE8 != 150000000 || string(event.Asset) != "BNB.BNB" || event.Asset2ndE8 != 50000000000 || string(event.Asset2nd) != "BNB.RUNE-67C" {
		t.Errorf(`got %d %q and %d %q with "coin": "150000000 BNB.BNB, 50000000000 BNB.RUNE-67C"`, event.AssetE8, event.Asset, event.Asset2ndE8, event.Asset2nd)
	}
}

func TestTransfer(t *testing.T) {
	var event Transfer
	err := event.LoadTendermint(toAttrs(map[string]string{
		"sender":    "tthoraddr1",
		"recipient": "tthoraddr2",
		"amount":    "123rune",
	}))
	require.NoError(t, err)
	require.Equal(t, int64(123), event.AmountE8)
	require.Equal(t, nativeRune, string(event.Asset))
	require.Equal(t, "tthoraddr1", string(event.FromAddr))
	require.Equal(t, "tthoraddr2", string(event.ToAddr))

	event = Transfer{}
	err = event.LoadTendermint(toAttrs(map[string]string{
		"sender":    "tthoraddr1",
		"recipient": "tthoraddr2",
		"amount":    "987bnb/bnb",
	}))
	require.NoError(t, err)
	require.Equal(t, int64(987), event.AmountE8)
	require.Equal(t, "BNB/BNB", string(event.Asset))
}

func TestBond(t *testing.T) {
	var event Bond
	err := event.LoadTendermint(toAttrs(map[string]string{
		// "bond_type": "0", // Because of the nature of this test
		// (and THORNode's EventBond Attributes() using string(m.BondType) rather than m.BondType.String()),
		// non-string bond_type cannot be represented.
		"amount": "100",
		"chain":  "THOR",
		"coin":   "100 THOR.RUNE",
		"from":   "tthor1zf3gsk7edzwl9syyefvfhle37cjtql35h6k85m",
		"id":     "98C1864036571E805BB0E0CCBAFF0F8D80F69BDEA32D5B26E0DDB95301C74D0C",
		"memo":   "BOND:tthor1zf3gsk7edzwl9syyefvfhle37cjtql35h6k85m",
		"to":     "tthor17gw75axcnr8747pkanye45pnrwk7p9c3uhzgff",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if event.E8 != 100 || event.AssetE8 != 100 || string(event.Asset) != "THOR.RUNE" {
		t.Errorf(`got %d / %d / %q when expecting 100 / 100 / THOR.RUNE"`, event.E8, event.AssetE8, event.Asset)
	}
}

func TestMintBurn(t *testing.T) {
	var event MintBurn
	err := event.LoadTendermint(toAttrs(map[string]string{
		"supply": "burn",
		"denom":  "bnb/bnb",
		"amount": "10",
		"reason": "failed_refund",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if string(event.Supply) != "burn" || string(event.Asset) != "BNB/BNB" || event.AssetE8 != 10 || string(event.Reason) != "failed_refund" {
		t.Errorf(`got %s / %q / %d / %s when expecting burn / BNB/BNB / 10 / failed_refund"`, event.Supply, event.Asset, event.AssetE8, event.Reason)
	}
}

func toAttrs(m map[string]string) []abci.EventAttribute {
	a := make([]abci.EventAttribute, 0, len(m))
	for k, v := range m {
		a = append(a, abci.EventAttribute{Key: []byte(k), Value: []byte(v), Index: true})
	}
	return a
}
