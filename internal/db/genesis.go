package db

import (
	"encoding/json"
	"os"
	"strconv"
	"time"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/util"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
)

var (
	GenesisInfo GenesisInfoType
	GenesisData GenesisType
)

// Genesis Type - Maybe it should be seperate file?
type GenesisInfoType struct {
	Height int64
	Hash   string
}

func (s GenesisInfoType) set(height int64, hash string) {
	GenesisInfo = GenesisInfoType{
		Height: height,
		Hash:   hash,
	}
}

func (s GenesisInfoType) Get() GenesisInfoType {
	if !ConfigHasGenesis() {
		return GenesisInfoType{}
	}
	return GenesisInfo
}

type JsonMap map[string]interface{}

// This genesis type is custom made from THORNode:
// https://gitlab.com/thorchain/thornode/-/blob/95ece18f92e363381aa0d09a9df779b4d63318f5/x/thorchain/genesis.pb.go#L132
type GenesisType struct {
	GenesisTime   time.Time `json:"genesis_time"`
	ChainID       string    `json:"chain_id"`
	InitialHeight string    `json:"initial_height"`
	AppState      AppState  `json:"app_state,omitempty"`
}

type AppState struct {
	Auth      JsonMap   `json:"auth"`
	Bank      Bank      `json:"bank"`
	Thorchain Thorchain `json:"thorchain"`
}

type Bank struct {
	Balances []Balance `json:"balances"`
	Supplies []Coin    `json:"supply"`
}

type Balance struct {
	Address string `json:"address"`
	Coins   []Coin `json:"coins"`
}

type Coin struct {
	Amount int64  `json:"amount,string"`
	Denom  string `json:"denom"`
}

type Node struct {
	NodeAddress string `json:"node_address"`
	Status      string `json:"status"`
	BondE8      int64  `json:"bond,string"`
	BondAddr    string `json:"bond_address"`
}

type Thorchain struct {
	LPs       []LP       `json:"liquidity_providers"`
	Loans     []Loan     `json:"loans"`
	Pools     []Pool     `json:"pools"`
	THORNames []THORName `json:"THORNames"`
	Nodes     []Node     `json:"node_accounts"`
}

type LP struct {
	Pool         string `json:"asset"`
	AssetAddr    string `json:"asset_address"`
	AssetE8      int64  `json:"asset_deposit_value,string"`
	PendingAsset int64  `json:"pending_asset,string"`
	RuneAddr     string `json:"rune_address"`
	RuneE8       int64  `json:"rune_deposit_value,string"`
	PendingRune  int64  `json:"pending_rune,string"`
	Units        int64  `json:"units,string"`
	LastHeight   int64  `json:"last_add_height,string"`
}

type Pool struct {
	BalanceRune         int64  `json:"balance_rune,string"`
	BalanceAsset        int64  `json:"balance_asset,string"`
	Asset               string `json:"asset"`
	LPUnits             int64  `json:"LP_units,string"`
	Status              string `json:"status"`
	StatusSince         int64  `json:"status_since,string"`
	Decimals            int64  `json:"decimals,string"`
	SynthUnits          int64  `json:"synth_units,string"`
	PendingInboundRune  int64  `json:"pending_inbound_rune,string"`
	PendingInboundAsset int64  `json:"pending_inbound_asset,string"`
}

type THORName struct {
	Name              string          `json:"name"`
	ExpireBlockHeight int64           `json:"expire_block_height,string"`
	Owner             string          `json:"owner"`
	PreferredAsset    string          `json:"preferred_asset"`
	Aliases           []THORNameAlias `json:"aliases"`
}

type THORNameAlias struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
}

type Loan struct {
	Asset               string `json:"asset"`
	Owner               string `json:"owner"`
	CollateralDeposited int64  `json:"collateral_deposited,string"`
	CollateralWithdrawn int64  `json:"collateral_withdrawn,string"`
	DebtIssued          int64  `json:"debt_issued,string"`
	DebtRepaid          int64  `json:"debt_repaid,string"`
	LastOpenHeight      int64  `json:"last_open_height,string"`
}

func GenesisExits() bool {
	value := readDbConstant("genesis")
	if value != nil {
		return true
	}
	return false
}

func (g *GenesisType) GetGenesisHeight() (height int64) {
	height, err := strconv.ParseInt(g.InitialHeight, 10, 64)
	if err != nil {
		midlog.Fatal("Can't parse the genesis initial height")
	}
	return
}

func DestoryGenesis() {
	GenesisData = GenesisType{}
}

func ConfigHasGenesis() bool {
	return config.Global.Genesis.Local != ""
}

func ReadDBGenesisHeight() int64 {
	value := readDbConstant("genesis")
	if value == nil {
		return 0
	}
	return util.MustParseInt64(string(value))
}

func InitGenesis() {
	if !ConfigHasGenesis() {
		return
	}

	genesisFile, err := os.ReadFile(config.Global.Genesis.Local)
	if err != nil {
		midlog.Fatal("Can't read genesis file!")
	}

	var genesisData GenesisType
	err = json.Unmarshal(genesisFile, &genesisData)
	if err != nil {
		midlog.ErrorE(err, "Can't unmarshal genesis file! seems the file is not right.")
	}

	genesisChainId := genesisData.ChainID
	genesisRootId := GetRootFromChainIdName(genesisChainId)
	rootFromThorNodeSatusChainId := RootChain.Get().Name
	if rootFromThorNodeSatusChainId != genesisRootId {
		midlog.FatalF("Genesis file chain id mismatch: root chain: %s, genesis: %s",
			RootChain.Get().Name, genesisChainId)
	}

	height := genesisData.GetGenesisHeight()
	if height <= 0 {
		midlog.Fatal("Genesis block height should be more than zero.")
	}

	dbHeight := ReadDBGenesisHeight()
	if dbHeight > 0 && dbHeight != height {
		midlog.Fatal("The DB current genesis height is not the same as the file, Please nukedb first.")
	}

	if config.Global.Genesis.InitialBlockHash == "" {
		midlog.Fatal("There is no hash in genesis config! Please add genesis block hash to config.")
	}
	GenesisInfo.set(height, config.Global.Genesis.InitialBlockHash)

	GenesisData = genesisData
}
