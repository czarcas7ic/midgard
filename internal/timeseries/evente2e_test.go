package timeseries_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/thorchain/midgard/internal/db/testdb"
	"gitlab.com/thorchain/midgard/internal/timeseries"
)

func TestScheduledOutbound(t *testing.T) {
	blocks := testdb.InitTestBlocks(t)

	blocks.NewBlock(t, "2020-09-01 00:00:00",
		testdb.PoolActivate("BTC.BTC"),
		testdb.AddLiquidity{
			Pool:                   "BTC.BTC",
			LiquidityProviderUnits: 42,
			RuneAmount:             1000000,
			AssetAmount:            500000000,
			RuneTxID:               "tx1",
			RuneAddress:            "runeaddr",
		},
	)
	blocks.NewBlock(t, "2020-09-01 00:00:01",
		testdb.Swap{
			TxID:               "67890",
			Coin:               "100 THOR.RUNE",
			EmitAsset:          "55000 BTC.BTC",
			Pool:               "BTC.BTC",
			Slip:               200,
			LiquidityFeeInRune: 20000,
			PriceTarget:        50000,
			FromAddress:        "thoraddr",
			ToAddress:          "btcadddr",
		},
		testdb.Fee{
			TxID:  "67890",
			Coins: "1 THOR.RUNE",
		},
		testdb.ScheduledOutbound{
			Chain:         "BTC",
			CoinAmount:    "55000",
			CoinAsset:     "BTC.BTC",
			CoinDecimals:  "0",
			GasRate:       "22",
			InHash:        "67890",
			MaxGasAmount:  []string{"200"},
			MaxGasAsset:   []string{"BTC.BTC"},
			MaxGasDecimal: []string{"8"},
			Memo:          "OUT:67890",
			ToAddress:     "btcadddr",
		},
	)

	scheduledOutboundQ := `
	SELECT COUNT(*) 
	FROM scheduled_outbound_events`

	// Check if outbound is in the table
	var outboundCount string
	err := timeseries.QueryOneValue(&outboundCount, context.Background(), scheduledOutboundQ)
	require.NoError(t, err)
	require.Equal(t, "1", outboundCount)
}
