package record_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/fetch/record"
)

func intToBytes(n int64) []byte {
	return []byte(base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(int(n)))))
}

func clearTable() {
	_, _ = db.TheDB.Exec("DELETE FROM swap_events")
}

func insertOne(t *testing.T, n int64) {
	e := record.Swap{
		Tx:             intToBytes(n),
		Chain:          []byte("chain"),
		FromAddr:       intToBytes(n),
		ToAddr:         intToBytes(n),
		FromAsset:      []byte("BNB.BNB"),
		FromE8:         n,
		ToAsset:        []byte("THOR.RUNE"),
		ToE8:           n,
		Memo:           intToBytes(n),
		Pool:           []byte("BNB.BNB"),
		ToE8Min:        n,
		SwapSlipBP:     n,
		LiqFeeE8:       n,
		LiqFeeInRuneE8: n,
	}
	height := n

	err := db.Inserter.StartBlock()
	if err != nil {
		t.Error("failed to StartBlock: ", err)
		return
	}

	q := []string{"tx", "chain", "from_addr", "to_addr", "from_asset", "from_e8", "to_asset", "to_e8", "memo", "pool", "to_e8_min", "swap_slip_bp", "liq_fee_e8", "liq_fee_in_rune_e8", "block_timestamp"}
	err = db.Inserter.Insert("swap_events", q,
		e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.FromAsset, e.FromE8, e.ToAsset, e.ToE8, e.Memo,
		e.Pool, e.ToE8Min, e.SwapSlipBP, e.LiqFeeE8, e.LiqFeeInRuneE8, height)
	if err != nil {
		t.Error("failed to insert:", err)
		return
	}

	err = db.Inserter.EndBlock()
	if err != nil {
		t.Error("failed to EndBlock: ", err)
		return
	}
}

func valueStringIterator(argNum int) func() string {
	argCount := 0
	// return a string like ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) but from argcount
	return func() string {
		bb := bytes.Buffer{}
		bb.WriteString(" (")
		for k := 1; k <= argNum; k++ {
			argCount++

			if k != 1 {
				bb.WriteRune(',')
			}
			bb.WriteRune('$')
			bb.WriteString(strconv.Itoa(argCount))
		}
		bb.WriteString(")")
		return bb.String()
	}
}

func insertBatch(t *testing.T, from, to int64) {
	length := int(to - from)
	argNum := 15
	valueStrs := make([]string, 0, length)
	valueArgs := make([]interface{}, 0, argNum*length)
	insertIt := valueStringIterator(argNum)
	for n := from; n < to; n++ {
		e := record.Swap{
			Tx:             intToBytes(n),
			Chain:          []byte("chain"),
			FromAddr:       intToBytes(n),
			ToAddr:         intToBytes(n),
			FromAsset:      []byte("BNB.BNB"),
			FromE8:         n,
			ToAsset:        []byte("THOR.RUNE"),
			ToE8:           n,
			Memo:           intToBytes(n),
			Pool:           []byte("BNB.BNB"),
			ToE8Min:        n,
			SwapSlipBP:     n,
			LiqFeeE8:       n,
			LiqFeeInRuneE8: n,
		}
		height := n
		valueStrs = append(valueStrs, insertIt())
		valueArgs = append(valueArgs, e.Tx, e.Chain, e.FromAddr, e.ToAddr, e.FromAsset, e.FromE8, e.ToAsset, e.ToE8, e.Memo,
			e.Pool, e.ToE8Min, e.SwapSlipBP, e.LiqFeeE8, e.LiqFeeInRuneE8, height)
	}
	q := fmt.Sprintf(
		`INSERT INTO swap_events (tx, chain, from_addr, to_addr, from_asset, from_E8, to_asset, to_E8, memo, pool, to_E8_min, swap_slip_BP, liq_fee_E8, liq_fee_in_rune_E8, block_timestamp)
	VALUES %s`, strings.Join(valueStrs, ","))

	// TODO(huginn): Use BatchInserter to test this instead
	result, err := db.TheDB.Exec(q, valueArgs...)
	if err != nil {
		t.Error("failed to insert:", err)
		return
	}
	k, err := result.RowsAffected()
	if err != nil {
		t.Error("failed to insert2: ", err)
		return
	}
	if int(k) != length {
		t.Error("not one insert:", k)
	}
}

func TestInsertOne(t *testing.T) {
	testdb.SetupTestDB(t)
	clearTable()
	insertOne(t, 0)
}

func TestInsertBatch(t *testing.T) {
	testdb.SetupTestDB(t)
	clearTable()
	insertBatch(t, 0, 4000)
}

func BenchmarkInsertOne(b *testing.B) {
	testdb.SetupTestDB(nil)
	clearTable()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		insertOne(nil, int64(i))
	}
}

// Max batch size we can use is ~4000 because there is a 64k limit on the
// sql argumentum size and we have 14 args per insert.
//
// The improvement is 73x:
//
// $ go test -run=NONE -bench Insert -v -p 1 ./...internal/timeseries...
// goos: linux
// goarch: amd64
// pkg: gitlab.com/thorchain/midgard/internal/timeseries
// BenchmarkInsertOne
// BenchmarkInsertOne-8                 682           1634591 ns/op
// BenchmarkInsertBatch
// BenchmarkInsertBatch-8             58502             22192 ns/op
// PASS
func BenchmarkInsertBatch(b *testing.B) {
	testdb.SetupTestDB(nil)
	clearTable()
	b.ResetTimer()
	batchSize := 4000
	for i := 0; i < b.N; i += batchSize {
		to := i + batchSize
		if b.N < to {
			to = b.N
		}
		insertBatch(nil, int64(i), int64(to))
	}
}
