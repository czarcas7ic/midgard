// Package event provides the blockchain data in a structured way.
//
// All asset amounts are fixed to 8 decimals. The resolution is made
// explicit with an E8 in the respective names.
//
// Numeric values are 64 bits wide, instead of the conventional 256
// bits used by most blockchains.
//
//	9 223 372 036 854 775 807  64-bit signed integer maximum
//	               00 000 000  decimals for fractions
//	   50 000 000 0·· ··· ···  500 M Rune total
//	    2 100 000 0·· ··· ···  21 M BitCoin total
//	   20 000 000 0·· ··· ···  200 M Ether total
package event

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/tendermint/tendermint/libs/kv"
)

// Asset Labels
const (
	// Native asset on THORChain.
	Rune = "THOR.RUNE"
	// Asset on binance test net.
	rune67C = "BNB.RUNE-67C"
	// Asset on Binance main net.
	runeB1A = "BNB.RUNE-B1A"

	Bitcoin     = "BTC.BTC"
	Ether       = "ETH.ETH"
	BinanceCoin = "BNB.BNB"
)

func IsRune(asset []byte) bool {
	switch string(asset) {
	case Rune, rune67C, runeB1A:
		return true
	}
	return false
}

// CoinSep is the separator for coin lists.
var coinSep = []byte{',', ' '}

type Amount struct {
	Asset []byte
	E8    int64
}

/*************************************************************/
/* Data models with Tendermint bindings in alphabetic order: */

// BUG(pascaldekloe): Duplicate keys in Tendermint transactions overwrite on another.

// Add defines the "add" event type.
type Add struct {
	Tx       []byte
	Chain    []byte
	FromAddr []byte
	ToAddr   []byte
	Asset    []byte
	AssetE8  int64 // Asset quantity times 100 M
	Memo     []byte

	RuneE8 int64 // Number of runes times 100 M

	Pool []byte
}

// LoadTendermint adopts the attributes.
func (e *Add) LoadTendermint(attrs []kv.Pair) error {
	*e = Add{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "id":
			e.Tx = attr.Value
		case "chain":
			e.Chain = attr.Value
		case "from":
			e.FromAddr = attr.Value
		case "to":
			e.ToAddr = attr.Value
		case "coin":
			b := attr.Value
			for len(b) != 0 {
				var asset []byte
				var amountE8 int64
				if i := bytes.Index(b, coinSep); i >= 0 {
					asset, amountE8, err = parseCoin(b[:i])
					b = b[i+len(coinSep):]
				} else {
					asset, amountE8, err = parseCoin(b)
					b = nil
				}
				if err != nil {
					return fmt.Errorf("malformed coin: %w", err)
				}

				if IsRune(asset) {
					e.RuneE8 = amountE8
				} else {
					e.AssetE8 = amountE8
					e.Asset = asset
				}
			}
		case "memo":
			e.Memo = attr.Value

		case "pool":
			e.Pool = attr.Value

		default:
			log.Printf("unknown add event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Bond defines the "bond" event type."
type Bond struct {
	Tx       []byte
	Chain    []byte
	FromAddr []byte
	ToAddr   []byte
	Asset    []byte
	AssetE8  int64 // Asset quantity times 100 M
	Memo     []byte

	BoundType []byte
	E8        int64
}

// LoadTendermint adopts the attributes.
func (e *Bond) LoadTendermint(attrs []kv.Pair) error {
	*e = Bond{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "id":
			e.Tx = attr.Value
		case "chain":
			e.Chain = attr.Value
		case "from":
			e.FromAddr = attr.Value
		case "to":
			e.ToAddr = attr.Value
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin(attr.Value)
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "memo":
			e.Memo = attr.Value

		case "amount":
			e.E8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed amount: %w", err)
			}
		case "bound_type":
			e.BoundType = attr.Value

		default:
			log.Printf("unknown bond event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Errata defines the "errata" event type.
type Errata struct {
	InTx    []byte
	Asset   []byte
	AssetE8 int64 // Asset quantity times 100 M
	RuneE8  int64 // Number of runes times 100 M
}

// LoadTendermint adopts the attributes.
func (e *Errata) LoadTendermint(attrs []kv.Pair) error {
	*e = Errata{}

	var flipAsset, flipRune bool

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "in_tx_id":
			e.InTx = attr.Value
		case "asset":
			e.Asset = attr.Value
		case "asset_amt":
			e.AssetE8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed asset_amt: %w", err)
			}
		case "asset_add":
			add, err := strconv.ParseBool(string(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed asset_add: %w", err)
			}
			flipAsset = !add
		case "rune_amt":
			e.RuneE8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed rune_amt: %w", err)
			}
		case "rune_add":
			add, err := strconv.ParseBool(string(attr.Value))
			if err != nil {
				return fmt.Errorf("malformed rune_add: %w", err)
			}
			flipRune = !add
		default:
			log.Printf("unknown errata event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	if flipAsset {
		e.AssetE8 = -e.AssetE8
	}
	if flipRune {
		e.RuneE8 = -e.RuneE8
	}

	return nil
}

// Fee defines the "fee" event type.
type Fee struct {
	Tx         []byte
	Asset      []byte
	AssetE8    int64 // Asset quantity times 100 M
	PoolDeduct int64 // rune quantity times 100 M
}

// LoadTendermint adopts the attributes.
func (e *Fee) LoadTendermint(attrs []kv.Pair) error {
	*e = Fee{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "tx_id":
			e.Tx = attr.Value
		case "coins":
			e.Asset, e.AssetE8, err = parseCoin(attr.Value)
			if err != nil {
				return fmt.Errorf("malformed coins: %w", err)
			}
		case "pool_deduct":
			e.PoolDeduct, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed pool_deduct: %w", err)
			}

		default:
			log.Printf("unknown fee event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Gas defines the "gas" event type.
type Gas struct {
	Asset   []byte
	AssetE8 int64 // Asset quantity times 100 M
	RuneE8  int64 // Number of runes times 100 M
	TxCount int64
}

// LoadTendermint adopts the attributes.
func (e *Gas) LoadTendermint(attrs []kv.Pair) error {
	*e = Gas{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "asset":
			e.Asset = attr.Value
		case "asset_amt":
			e.AssetE8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed asset_amt: %w", err)
			}
		case "rune_amt":
			e.RuneE8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed rune_amt: %w", err)
			}
		case "transaction_count":
			e.TxCount, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed transaction_count: %w", err)
			}

		default:
			log.Printf("unknown gas event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Outbound is a transfer confirmation of pool withdrawal.
type Outbound struct {
	Tx       []byte // THORChain transaction ID
	Chain    []byte // transfer backend ID
	FromAddr []byte // transfer pool address
	ToAddr   []byte // transfer contender address
	Asset    []byte // transfer unit ID
	AssetE8  int64  // transfer quantity times 100 M
	Memo     []byte // transfer description
	InTx     []byte // THORCHAIN transaction ID of subject
}

// LoadTendermint adopts the attributes.
func (e *Outbound) LoadTendermint(attrs []kv.Pair) error {
	*e = Outbound{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "id":
			e.Tx = attr.Value
		case "chain":
			e.Chain = attr.Value
		case "from":
			e.FromAddr = attr.Value
		case "to":
			e.ToAddr = attr.Value
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin(attr.Value)
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "memo":
			e.Memo = attr.Value

		case "in_tx_id":
			e.InTx = attr.Value

		default:
			log.Printf("unknown outbound event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Pool defines the "pool" event type.
type Pool struct {
	Asset  []byte
	Status []byte
}

// LoadTendermint adopts the attributes.
func (e *Pool) LoadTendermint(attrs []kv.Pair) error {
	*e = Pool{}

	for _, attr := range attrs {
		switch string(attr.Key) {
		case "pool":
			e.Asset = attr.Value
		case "pool_status":
			e.Status = attr.Value

		default:
			log.Printf("unknown pool event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Refund defines the "refund" event type.
type Refund struct {
	Tx       []byte
	Chain    []byte
	FromAddr []byte
	ToAddr   []byte
	Asset    []byte
	AssetE8  int64 // Asset quantity times 100 M
	Memo     []byte

	Code   int64
	Reason []byte
}

// LoadTendermint adopts the attributes.
func (e *Refund) LoadTendermint(attrs []kv.Pair) error {
	*e = Refund{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "id":
			e.Tx = attr.Value
		case "chain":
			e.Chain = attr.Value
		case "from":
			e.FromAddr = attr.Value
		case "to":
			e.ToAddr = attr.Value
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin(attr.Value)
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "memo":
			e.Memo = attr.Value

		case "code":
			e.Code, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed code: %w", err)
			}
		case "reason":
			e.Reason = attr.Value

		default:
			log.Printf("unknown refund event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Reserve defines the "reserve" event type.
type Reserve struct {
	Tx       []byte
	Chain    []byte // redundant to asset
	FromAddr []byte
	ToAddr   []byte // may have multiple, separated by space
	Asset    []byte
	AssetE8  int64 // Asset quantity times 100 M
	Memo     []byte

	Addr []byte
	E8   int64 // Number of runes times 100 M
}

// LoadTendermint adopts the attributes.
func (e *Reserve) LoadTendermint(attrs []kv.Pair) error {
	*e = Reserve{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		// thornode: common.Tx
		case "id":
			e.Tx = attr.Value
		case "chain":
			e.Chain = attr.Value
		case "from":
			e.FromAddr = attr.Value
		case "to":
			e.ToAddr = attr.Value
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin(attr.Value)
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "memo":
			e.Memo = attr.Value

		case "contributor_address":
			e.Addr = attr.Value
		case "amount":
			e.E8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed amount: %w", err)
			}

		default:
			log.Printf("unknown reserve event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Rewards defines the "rewards" event type.
type Rewards struct {
	BondE8 int64 // rune amount times 100 M
	Pool   []Amount
}

// LoadTendermint adopts the attributes.
func (e *Rewards) LoadTendermint(attrs []kv.Pair) error {
	*e = Rewards{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "bond_reward":
			e.BondE8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed bond_reward: %w", err)
			}

		default:
			v, err := strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				log.Printf("unknown rewards event attribute %q=%q", attr.Key, attr.Value)
				break
			}
			e.Pool = append(e.Pool, Amount{attr.Key, v})
		}
	}

	return nil
}

// Stake is a participation result.
type Stake struct {
	Pool       []byte // asset ID
	AssetTx    []byte // transfer transaction ID (may equal RuneTx)
	AssetChain []byte // transfer backend ID
	AssetE8    int64  // transfer asset quantity times 100 M
	RuneTx     []byte // pool transaction ID
	RuneChain  []byte // pool backend ID
	RuneAddr   []byte // pool contender address
	RuneE8     int64  // pool transaction quantity times 100 M
	StakeUnits int64  // pool's liquidiy tokens—gained quantity
}

var txIDSuffix = []byte("_txid")

// LoadTendermint adopts the attributes.
func (e *Stake) LoadTendermint(attrs []kv.Pair) error {
	*e = Stake{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "pool":
			e.Pool = attr.Value
		case "stake_units":
			e.StakeUnits, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed stake_units: %w", err)
			}
		case "THORChain_txid", "BNBChain_txid": // BNBChain for Binance test & main net
			e.RuneTx = attr.Value
			e.RuneChain = attr.Key[:len(attr.Key)-10]
		case "rune_address":
			e.RuneAddr = attr.Value
		case "rune_amount":
			e.RuneE8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed rune_amount: %w", err)
			}
		case "asset_amount":
			e.AssetE8, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed asset_amount: %w", err)
			}

		default:
			switch {
			case bytes.HasSuffix(attr.Key, txIDSuffix):
				if e.AssetChain != nil {
					// Can only have one additional (non-rune) tx.
					// Maybe the .RuneTx keys are incomplete?
					return fmt.Errorf("%q preceded by %q%s", attr.Key, e.AssetChain, txIDSuffix)
				}
				e.AssetChain = attr.Key[:len(attr.Key)-len(txIDSuffix)]

				e.AssetTx = attr.Value

			default:
				log.Printf("unknown stake event attribute %q=%q", attr.Key, attr.Value)
			}
		}
	}

	if e.RuneTx == nil {
		// omitted when equal
		e.RuneTx = e.AssetTx
	}

	return nil
}

// Slash defines the "slash" event type.
type Slash struct {
	Pool    []byte
	Amounts []Amount
}

// LoadTendermint adopts the attributes.
func (e *Slash) LoadTendermint(attrs []kv.Pair) error {
	*e = Slash{}

	for _, attr := range attrs {
		switch string(attr.Key) {
		case "pool":
			e.Pool = attr.Value

		default:
			v, err := strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				log.Printf("unknown slash event attribute %q=%q", attr.Key, attr.Value)
				break
			}
			e.Amounts = append(e.Amounts, Amount{attr.Key, v})
		}
	}

	return nil
}

// Swap defines the "swap" event type.
type Swap struct {
	Tx       []byte
	Chain    []byte
	FromAddr []byte
	ToAddr   []byte
	Asset    []byte
	AssetE8  int64 // Asset quantity times 100 M
	Memo     []byte

	Pool         []byte
	PriceTarget  int64
	TradeSlip    int64
	LiqFee       int64
	LiqFeeInRune int64
}

// LoadTendermint adopts the attributes.
func (e *Swap) LoadTendermint(attrs []kv.Pair) error {
	*e = Swap{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "id":
			e.Tx = attr.Value
		case "chain":
			e.Chain = attr.Value
		case "from":
			e.FromAddr = attr.Value
		case "to":
			e.ToAddr = attr.Value
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin(attr.Value)
			if err != nil {
				return fmt.Errorf("malformed coins: %w", err)
			}
		case "memo":
			e.Memo = attr.Value

		case "pool":
			e.Pool = attr.Value
		case "price_target":
			e.PriceTarget, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed price_target: %w", err)
			}
		case "trade_slip":
			e.TradeSlip, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed trade_slip: %w", err)
			}
		case "liquidity_fee":
			e.LiqFee, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed liquidity_fee: %w", err)
			}
		case "liquidity_fee_in_rune":
			e.LiqFeeInRune, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed liquidity_fee_in_rune: %w", err)
			}

		default:
			log.Printf("unknown swap event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

// Unstake is a pool withdrawal result.
type Unstake struct {
	Tx          []byte // THORChain transaction ID
	Chain       []byte // transfer backend ID
	FromAddr    []byte // transfer pool address
	ToAddr      []byte // transfer contender address
	Asset       []byte // transfer unit ID
	AssetE8     int64  // transfer quantity times 100 M
	Memo        []byte
	Pool        []byte  // asset ID
	StakeUnits  int64   // pool's liquidiy tokens—lost quantity
	BasisPoints int64   // ‱ of what?
	Asymmetry   float64 // lossy conversion of what?
}

// LoadTendermint adopts the attributes.
func (e *Unstake) LoadTendermint(attrs []kv.Pair) error {
	*e = Unstake{}

	for _, attr := range attrs {
		var err error
		switch string(attr.Key) {
		case "id":
			e.Tx = attr.Value
		case "chain":
			e.Chain = attr.Value
		case "from":
			e.FromAddr = attr.Value
		case "to":
			e.ToAddr = attr.Value
		case "coin":
			e.Asset, e.AssetE8, err = parseCoin(attr.Value)
			if err != nil {
				return fmt.Errorf("malformed coin: %w", err)
			}
		case "memo":
			e.Memo = attr.Value

		case "pool":
			e.Pool = attr.Value
		case "stake_units":
			e.StakeUnits, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed stake_units: %w", err)
			}
		case "basis_points":
			e.BasisPoints, err = strconv.ParseInt(string(attr.Value), 10, 64)
			if err != nil {
				return fmt.Errorf("malformed basis_points: %w", err)
			}
		case "asymmetry":
			e.Asymmetry, err = strconv.ParseFloat(string(attr.Value), 64)
			if err != nil {
				return fmt.Errorf("malformed asymmetry: %w", err)
			}

		default:
			log.Printf("unknown unstake event attribute %q=%q", attr.Key, attr.Value)
		}
	}

	return nil
}

var errNoSep = errors.New("separator not found")

func parseCoin(b []byte) (asset []byte, amountE8 int64, err error) {
	i := bytes.IndexByte(b, ' ')
	if i < 0 {
		return nil, 0, errNoSep
	}
	asset = b[i+1:]
	amountE8, err = strconv.ParseInt(string(b[:i]), 10, 64)
	return
}
