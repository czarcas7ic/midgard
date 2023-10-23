package record

import (
	"strconv"
	"strings"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

var metaData Metadata

func recordGenPools() {
	for i, e := range db.GenesisData.AppState.Thorchain.Pools {
		cols := []string{
			"asset", "rune_amt", "rune_add", "asset_amt", "asset_add", "reason"}
		err := InsertWithMeta("pool_balance_change_events", &metaData, cols,
			e.Asset, e.BalanceRune, true, e.BalanceAsset, true, "genesisAdd")

		if err != nil {
			miderr.LogEventParseErrorF(
				"add event from genesis lost index: %d, err: %s", i, err)
			return
		}

		cols = []string{"asset", "status"}
		err = InsertWithMeta("pool_events", &metaData, cols, e.Asset, e.Status)
		if err != nil {
			miderr.LogEventParseErrorF("pool event from height %d lost on %s", i, err)
		}

		if err != nil {
			miderr.LogEventParseErrorF(
				"add event from genesis lost index: %d, err: %s", i, err)
			return
		}

		Recorder.AddPoolAssetE8Depth([]byte(e.Asset), e.BalanceAsset)
		Recorder.AddPoolRuneE8Depth([]byte(e.Asset), e.BalanceRune)
	}
}

func recordGenSupplies() {
	for _, e := range db.GenesisData.AppState.Bank.Supplies {
		poolName := strings.ToUpper(e.Denom)
		if util.AssetFromString(e.Denom).Synth {
			poolName = util.ConvertSynthPoolToNative(poolName)
		}

		Recorder.AddPoolSynthE8Depth([]byte(poolName), e.Amount)
	}
}

func recordGenTransfers() {
	if config.Global.EventRecorder.OnTransferEnabled {
		accBalances := db.GenesisData.AppState.Bank.Balances

		for i, b := range accBalances {
			for _, c := range b.Coins {
				cols := []string{"from_addr", "to_addr", "asset", "amount_e8"}
				err := InsertWithMeta("transfer_events", &metaData, cols,
					"genesis", b.Address, c.Denom, c.Amount)

				if err != nil {
					miderr.LogEventParseErrorF(
						"transfer event from genesis lost index: %d, err: %s", i, err)
					return
				}
			}
		}
	}
}

func recordGenLPs() {
	for i, e := range db.GenesisData.AppState.Thorchain.LPs {
		if e.PendingAsset > 0 || e.PendingRune > 0 {
			cols := []string{
				"pool", "asset_tx", "asset_chain", "asset_addr", "asset_e8",
				"rune_tx", "rune_addr", "rune_e8",
				"pending_type"}
			err := InsertWithMeta("pending_liquidity_events", &metaData, cols, e.Pool,
				"genesisTx", util.AssetFromString(e.Pool).Chain, e.AssetAddr, e.AssetE8,
				"genesisTx", e.RuneAddr, e.RuneE8,
				"add")

			if err != nil {
				miderr.LogEventParseErrorF(
					"pending_liquidity event from height %d lost on %s",
					i, err)
				return
			}
		}

		if e.Units > 0 {
			aE8, rE8, _ := Recorder.CurrentDepths([]byte(e.Pool))
			var assetInRune int64
			if aE8 != 0 {
				assetInRune = int64(float64(e.AssetE8)*(float64(rE8)/float64(aE8)) + 0.5)
			}

			cols := []string{
				"pool", "asset_tx", "asset_chain",
				"asset_addr", "asset_e8", "stake_units", "rune_tx", "rune_addr", "rune_e8",
				"_asset_in_rune_e8"}
			err := InsertWithMeta(
				"stake_events", &metaData, cols,
				e.Pool, "genesisTx", util.AssetFromString(e.Pool).Chain,
				e.AssetAddr, e.AssetE8, e.Units, "genesisTx", e.RuneAddr, e.RuneE8,
				assetInRune)

			if err != nil {
				miderr.LogEventParseErrorF(
					"add event from genesis lost index: %d, err: %s", i, err)
				return
			}
		}
	}
}

func recordGenTHORNames() {
	for i, e := range db.GenesisData.AppState.Thorchain.THORNames {
		for j, a := range e.Aliases {
			cols := []string{
				"name", "chain", "address", "registration_fee_e8", "fund_amount_e8", "expire", "owner"}
			err := InsertWithMeta("thorname_change_events", &metaData, cols,
				e.Name, a.Chain, a.Address, 0, 0, e.ExpireBlockHeight, e.Owner)

			if err != nil {
				miderr.LogEventParseErrorF(
					"thorname event from genesis item %d, aliases %d lost on %s",
					i, j, err)
			}
		}
	}
}

func recordGenNodes() {
	for i, e := range db.GenesisData.AppState.Thorchain.Nodes {
		cols := []string{
			"node_addr", "former", "current"}
		err := InsertWithMeta("update_node_account_status_events", &metaData, cols,
			e.NodeAddress, []byte{}, e.Status)

		if err != nil {
			miderr.LogEventParseErrorF(
				"node account change event from genesis item %d, lost on %s",
				i, err)
		}

		cols = []string{
			"tx", "chain", "from_addr", "to_addr", "asset", "asset_e8", "memo", "bond_type", "e8"}
		err = InsertWithMeta("bond_events", &metaData, cols,
			"genesisTx", "THOR", e.BondAddr, "", "THOR.RUNE", 0, "", "bond_paid", e.BondE8)

		if err != nil {
			miderr.LogEventParseErrorF("bond event from genesis item %d lost on %s", i, err)
		}
	}
}

func recordGenLoans() {
	for i, e := range db.GenesisData.AppState.Thorchain.Loans {
		cols := []string{"owner", "collateral_deposited", "debt_issued",
			"collateralization_ratio", "collateral_asset", "target_asset"}

		tm := metaData
		tm.BlockHeight = e.LastOpenHeight
		tm.EventId = db.EventId{
			BlockHeight: e.LastOpenHeight,
		}
		err := InsertWithMeta("loan_open_events", &tm, cols,
			e.Owner, e.CollateralDeposited, e.DebtIssued, 0, e.Asset, "")

		if err != nil {
			miderr.LogEventParseErrorF(
				"loan open event from genesis item %d, lost on %s",
				i, err)
		}

		if e.CollateralWithdrawn > 0 || e.DebtRepaid > 0 {
			cols = []string{"owner", "collateral_withdrawn", "debt_repaid", "collateral_asset"}
			err = InsertWithMeta("loan_repayment_events", &metaData, cols,
				e.Owner, e.CollateralWithdrawn, e.DebtRepaid, e.Asset)

			if err != nil {
				miderr.LogEventParseErrorF("loan repayment event from genesis item %d lost on %s", i, err)
			}
		}
	}
}

func LoadGenesis() {
	if !db.ConfigHasGenesis() {
		return
	}

	defer db.DestoryGenesis()

	genesisBlockHeight := db.GenesisData.GetGenesisHeight()
	recordBlockHeight := genesisBlockHeight - 1

	lastCommited := db.LastCommittedBlock.Get().Height

	if db.GenesisExits() {
		midlog.Info("The Genesis is already written.")
		// This means block log was empty and first block of genesis hasn't been fetched.
		if lastCommited == 0 {
			midlog.Info("The Genesis is written but there was no block fetch.")
			db.LastCommittedBlock.Set(recordBlockHeight, db.TimeToNano(db.GenesisData.GenesisTime.Add(-1)))
		}
		return
	}

	if genesisBlockHeight < lastCommited {
		midlog.Info("The Genesis was already written in a previous run.")
		return
	}

	// Initilize metadata
	metaData := Metadata{
		BlockHeight:    recordBlockHeight,
		BlockTimestamp: db.GenesisData.GenesisTime.Add(-1),
		EventId: db.EventId{
			BlockHeight: recordBlockHeight,
		},
	}

	BeginBlockEventsTotal.Add(uint64(0))
	metaData.EventId.Location = db.BeginBlockEvents
	metaData.EventId.EventIndex = 1

	err := db.Inserter.StartBlock()
	if err != nil {
		midlog.FatalE(err, "Failed to StartBlock")
		return
	}

	// Add Genesis KV to the records
	recordGenPools()
	recordGenSupplies()
	recordGenTransfers()
	recordGenLPs()
	recordGenTHORNames()
	recordGenNodes()
	recordGenLoans()

	// Set genesis constant to db
	setGenesisConstant(genesisBlockHeight)

	err = db.Inserter.EndBlock()
	if err != nil {
		midlog.FatalE(err, "Failed to EndBlock")
		deleteDBIfFail()
		return
	}

	err = db.Inserter.Flush()
	if err != nil {
		midlog.FatalE(err, "Failed to Flush")
		deleteDBIfFail()
		return
	}

	db.LastCommittedBlock.Set(recordBlockHeight, db.TimeToNano(db.GenesisData.GenesisTime.Add(-1)))
}

func setGenesisConstant(genesisBlockHeight int64) {
	_, err := db.TheDB.Exec(`INSERT INTO constants (key, value) VALUES ('genesis', $1)
			ON CONFLICT (key) DO UPDATE SET value = $1`, []byte(strconv.FormatInt(genesisBlockHeight, 10)))
	if err != nil {
		midlog.FatalE(err, "Failed to Insert into constants")
		return
	}
}

func deleteDBIfFail() {
	_, err := db.TheDB.Exec(`DELETE FROM constants WHERE key = 'ddl_hash'`)
	if err != nil {
		midlog.FatalE(err, "Failed to delete ddl hash.")
		return
	}
}
