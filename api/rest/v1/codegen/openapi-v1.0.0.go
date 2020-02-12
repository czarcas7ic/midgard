// Package api provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen DO NOT EDIT.
package api

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"github.com/deepmap/oapi-codegen/pkg/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

// AssetDetail defines model for AssetDetail.
type AssetDetail struct {
	Asset       *Asset   `json:"asset,omitempty"`
	DateCreated *int64   `json:"dateCreated,omitempty"`
	Logo        *string  `json:"logo,omitempty"`
	Name        *string  `json:"name,omitempty"`
	PriceRune   *float64 `json:"priceRune,omitempty"`
}

// Error defines model for Error.
type Error struct {
	Error string `json:"error"`
}

// PoolDetail defines model for PoolDetail.
type PoolDetail struct {
	Asset            *Asset   `json:"asset,omitempty"`
	AssetDepth       *int64   `json:"assetDepth,omitempty"`
	AssetROI         *float64 `json:"assetROI,omitempty"`
	AssetStakedTotal *int64   `json:"assetStakedTotal,omitempty"`
	BuyAssetCount    *int64   `json:"buyAssetCount,omitempty"`
	BuyFeeAverage    *int64   `json:"buyFeeAverage,omitempty"`
	BuyFeesTotal     *int64   `json:"buyFeesTotal,omitempty"`
	BuySlipAverage   *float64 `json:"buySlipAverage,omitempty"`
	BuyTxAverage     *int64   `json:"buyTxAverage,omitempty"`
	BuyVolume        *int64   `json:"buyVolume,omitempty"`
	PoolDepth        *int64   `json:"poolDepth,omitempty"`
	PoolFeeAverage   *int64   `json:"poolFeeAverage,omitempty"`
	PoolFeesTotal    *int64   `json:"poolFeesTotal,omitempty"`
	PoolROI          *float64 `json:"poolROI,omitempty"`
	PoolROI12        *float64 `json:"poolROI12,omitempty"`
	PoolSlipAverage  *float64 `json:"poolSlipAverage,omitempty"`
	PoolStakedTotal  *int64   `json:"poolStakedTotal,omitempty"`
	PoolTxAverage    *int64   `json:"poolTxAverage,omitempty"`
	PoolUnits        *int64   `json:"poolUnits,omitempty"`
	PoolVolume       *int64   `json:"poolVolume,omitempty"`
	PoolVolume24hr   *int64   `json:"poolVolume24hr,omitempty"`
	Price            *float64 `json:"price,omitempty"`
	RuneDepth        *int64   `json:"runeDepth,omitempty"`
	RuneROI          *float64 `json:"runeROI,omitempty"`
	RuneStakedTotal  *int64   `json:"runeStakedTotal,omitempty"`
	SellAssetCount   *int64   `json:"sellAssetCount,omitempty"`
	SellFeeAverage   *int64   `json:"sellFeeAverage,omitempty"`
	SellFeesTotal    *int64   `json:"sellFeesTotal,omitempty"`
	SellSlipAverage  *float64 `json:"sellSlipAverage,omitempty"`
	SellTxAverage    *int64   `json:"sellTxAverage,omitempty"`
	SellVolume       *int64   `json:"sellVolume,omitempty"`
	StakeTxCount     *int64   `json:"stakeTxCount,omitempty"`
	StakersCount     *int64   `json:"stakersCount,omitempty"`
	StakingTxCount   *int64   `json:"stakingTxCount,omitempty"`
	Status           *string  `json:"status,omitempty"`
	SwappersCount    *int64   `json:"swappersCount,omitempty"`
	SwappingTxCount  *int64   `json:"swappingTxCount,omitempty"`
	WithdrawTxCount  *int64   `json:"withdrawTxCount,omitempty"`
}

// Stakers defines model for Stakers.
type Stakers string

// StakersAddressData defines model for StakersAddressData.
type StakersAddressData struct {
	PoolsArray  *[]Asset `json:"poolsArray,omitempty"`
	TotalEarned *int64   `json:"totalEarned,omitempty"`
	TotalROI    *float64 `json:"totalROI,omitempty"`
	TotalStaked *int64   `json:"totalStaked,omitempty"`
}

// StakersAssetData defines model for StakersAssetData.
type StakersAssetData struct {
	Asset           *Asset   `json:"asset,omitempty"`
	AssetEarned     *int64   `json:"assetEarned,omitempty"`
	AssetROI        *float64 `json:"assetROI,omitempty"`
	AssetStaked     *int64   `json:"assetStaked,omitempty"`
	DateFirstStaked *int64   `json:"dateFirstStaked,omitempty"`
	PoolEarned      *int64   `json:"poolEarned,omitempty"`
	PoolROI         *float64 `json:"poolROI,omitempty"`
	PoolStaked      *int64   `json:"poolStaked,omitempty"`
	RuneEarned      *int64   `json:"runeEarned,omitempty"`
	RuneROI         *float64 `json:"runeROI,omitempty"`
	RuneStaked      *int64   `json:"runeStaked,omitempty"`
	StakeUnits      *int64   `json:"stakeUnits,omitempty"`
}

// StatsData defines model for StatsData.
type StatsData struct {
	DailyActiveUsers   *int64 `json:"dailyActiveUsers,omitempty"`
	DailyTx            *int64 `json:"dailyTx,omitempty"`
	MonthlyActiveUsers *int64 `json:"monthlyActiveUsers,omitempty"`
	MonthlyTx          *int64 `json:"monthlyTx,omitempty"`
	PoolCount          *int64 `json:"poolCount,omitempty"`
	TotalAssetBuys     *int64 `json:"totalAssetBuys,omitempty"`
	TotalAssetSells    *int64 `json:"totalAssetSells,omitempty"`
	TotalDepth         *int64 `json:"totalDepth,omitempty"`
	TotalEarned        *int64 `json:"totalEarned,omitempty"`
	TotalStakeTx       *int64 `json:"totalStakeTx,omitempty"`
	TotalStaked        *int64 `json:"totalStaked,omitempty"`
	TotalTx            *int64 `json:"totalTx,omitempty"`
	TotalUsers         *int64 `json:"totalUsers,omitempty"`
	TotalVolume        *int64 `json:"totalVolume,omitempty"`
	TotalVolume24hr    *int64 `json:"totalVolume24hr,omitempty"`
	TotalWithdrawTx    *int64 `json:"totalWithdrawTx,omitempty"`
}

// ThorchainEndpoint defines model for ThorchainEndpoint.
type ThorchainEndpoint struct {
	Address *string `json:"address,omitempty"`
	Chain   *string `json:"chain,omitempty"`
	PubKey  *string `json:"pub_key,omitempty"`
}

// ThorchainEndpoints defines model for ThorchainEndpoints.
type ThorchainEndpoints struct {
	Current *[]ThorchainEndpoint `json:"current,omitempty"`
}

// TxDetails defines model for TxDetails.
type TxDetails struct {
	Date    *int64  `json:"date,omitempty"`
	Events  *Event  `json:"events,omitempty"`
	Gas     *Gas    `json:"gas,omitempty"`
	Height  *int64  `json:"height,omitempty"`
	In      *Tx     `json:"in,omitempty"`
	Options *Option `json:"options,omitempty"`
	Out     *[]Tx   `json:"out,omitempty"`
	Pool    *Asset  `json:"pool,omitempty"`
	Status  *string `json:"status,omitempty"`
	Type    *string `json:"type,omitempty"`
}

// Asset defines model for asset.
type Asset string

// Coin defines model for coin.
type Coin struct {
	Amount *int64 `json:"amount,omitempty"`
	Asset  *Asset `json:"asset,omitempty"`
}

// Coins defines model for coins.
type Coins []Coin

// Event defines model for event.
type Event struct {
	Fee        *int64   `json:"fee,omitempty"`
	Slip       *float64 `json:"slip,omitempty"`
	StakeUnits *int64   `json:"stakeUnits,omitempty"`
}

// Gas defines model for gas.
type Gas struct {
	Amount *int64 `json:"amount,omitempty"`
	Asset  *Asset `json:"asset,omitempty"`
}

// Option defines model for option.
type Option struct {
	Asymmetry           *float64 `json:"asymmetry,omitempty"`
	PriceTarget         *int64   `json:"priceTarget,omitempty"`
	WithdrawBasisPoints *int64   `json:"withdrawBasisPoints,omitempty"`
}

// Tx defines model for tx.
type Tx struct {
	Address *string `json:"address,omitempty"`
	Coins   *Coins  `json:"coins,omitempty"`
	Memo    *string `json:"memo,omitempty"`
	TxID    *string `json:"txID,omitempty"`
}

// AssetsDetailedResponse defines model for AssetsDetailedResponse.
type AssetsDetailedResponse AssetDetail

// GeneralErrorResponse defines model for GeneralErrorResponse.
type GeneralErrorResponse Error

// PoolsDetailedResponse defines model for PoolsDetailedResponse.
type PoolsDetailedResponse PoolDetail

// PoolsResponse defines model for PoolsResponse.
type PoolsResponse []Asset

// StakersAddressDataResponse defines model for StakersAddressDataResponse.
type StakersAddressDataResponse StakersAddressData

// StakersAssetDataResponse defines model for StakersAssetDataResponse.
type StakersAssetDataResponse StakersAssetData

// StakersResponse defines model for StakersResponse.
type StakersResponse []Stakers

// StatsResponse defines model for StatsResponse.
type StatsResponse StatsData

// ThorchainEndpointsResponse defines model for ThorchainEndpointsResponse.
type ThorchainEndpointsResponse ThorchainEndpoints

// TxDetailedResponse defines model for TxDetailedResponse.
type TxDetailedResponse []TxDetails

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Get Asset Information// (GET /v1/assets/{asset})
	GetAssetInfo(ctx echo.Context, asset string) error
	// Get Documents// (GET /v1/doc)
	GetDocs(ctx echo.Context) error
	// Get Health// (GET /v1/health)
	GetHealth(ctx echo.Context) error
	// Get Asset Pools// (GET /v1/pools)
	GetPools(ctx echo.Context) error
	// Get Pools Data// (GET /v1/pools/{asset})
	GetPoolsData(ctx echo.Context, asset string) error
	// Get Stakers// (GET /v1/stakers)
	GetStakersData(ctx echo.Context) error
	// Get Staker Data// (GET /v1/stakers/{address})
	GetStakersAddressData(ctx echo.Context, address string) error
	// Get Staker Pool Data// (GET /v1/stakers/{address}/{asset})
	GetStakersAddressAndAssetData(ctx echo.Context, address string, asset string) error
	// Get Global Stats// (GET /v1/stats)
	GetStats(ctx echo.Context) error
	// Get Swagger// (GET /v1/swagger.json)
	GetSwagger(ctx echo.Context) error
	// Get the Proxied Pool Addresses// (GET /v1/thorchain/pool_addresses)
	GetThorchainProxiedEndpoints(ctx echo.Context) error
	// Get transaction// (GET /v1/tx/asset/{asset})
	GetTxDetailsByAsset(ctx echo.Context, asset string) error
	// Get transaction// (GET /v1/tx/{address})
	GetTxDetails(ctx echo.Context, address string) error
	// Get transaction// (GET /v1/tx/{address}/asset/{asset})
	GetTxDetailsByAddressAsset(ctx echo.Context, address string, asset string) error
	// Get transaction// (GET /v1/tx/{address}/txid/{txid})
	GetTxDetailsByAddressTxId(ctx echo.Context, address string, txid string) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// GetAssetInfo converts echo context to params.
func (w *ServerInterfaceWrapper) GetAssetInfo(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "asset" -------------
	var asset string

	err = runtime.BindStyledParameter("simple", false, "asset", ctx.Param("asset"), &asset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetAssetInfo(ctx, asset)
	return err
}

// GetDocs converts echo context to params.
func (w *ServerInterfaceWrapper) GetDocs(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetDocs(ctx)
	return err
}

// GetHealth converts echo context to params.
func (w *ServerInterfaceWrapper) GetHealth(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetHealth(ctx)
	return err
}

// GetPools converts echo context to params.
func (w *ServerInterfaceWrapper) GetPools(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetPools(ctx)
	return err
}

// GetPoolsData converts echo context to params.
func (w *ServerInterfaceWrapper) GetPoolsData(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "asset" -------------
	var asset string

	err = runtime.BindStyledParameter("simple", false, "asset", ctx.Param("asset"), &asset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetPoolsData(ctx, asset)
	return err
}

// GetStakersData converts echo context to params.
func (w *ServerInterfaceWrapper) GetStakersData(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetStakersData(ctx)
	return err
}

// GetStakersAddressData converts echo context to params.
func (w *ServerInterfaceWrapper) GetStakersAddressData(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameter("simple", false, "address", ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetStakersAddressData(ctx, address)
	return err
}

// GetStakersAddressAndAssetData converts echo context to params.
func (w *ServerInterfaceWrapper) GetStakersAddressAndAssetData(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameter("simple", false, "address", ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	// ------------- Path parameter "asset" -------------
	var asset string

	err = runtime.BindStyledParameter("simple", false, "asset", ctx.Param("asset"), &asset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetStakersAddressAndAssetData(ctx, address, asset)
	return err
}

// GetStats converts echo context to params.
func (w *ServerInterfaceWrapper) GetStats(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetStats(ctx)
	return err
}

// GetSwagger converts echo context to params.
func (w *ServerInterfaceWrapper) GetSwagger(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetSwagger(ctx)
	return err
}

// GetThorchainProxiedEndpoints converts echo context to params.
func (w *ServerInterfaceWrapper) GetThorchainProxiedEndpoints(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetThorchainProxiedEndpoints(ctx)
	return err
}

// GetTxDetailsByAsset converts echo context to params.
func (w *ServerInterfaceWrapper) GetTxDetailsByAsset(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "asset" -------------
	var asset string

	err = runtime.BindStyledParameter("simple", false, "asset", ctx.Param("asset"), &asset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetTxDetailsByAsset(ctx, asset)
	return err
}

// GetTxDetails converts echo context to params.
func (w *ServerInterfaceWrapper) GetTxDetails(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameter("simple", false, "address", ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetTxDetails(ctx, address)
	return err
}

// GetTxDetailsByAddressAsset converts echo context to params.
func (w *ServerInterfaceWrapper) GetTxDetailsByAddressAsset(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameter("simple", false, "address", ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	// ------------- Path parameter "asset" -------------
	var asset string

	err = runtime.BindStyledParameter("simple", false, "asset", ctx.Param("asset"), &asset)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter asset: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetTxDetailsByAddressAsset(ctx, address, asset)
	return err
}

// GetTxDetailsByAddressTxId converts echo context to params.
func (w *ServerInterfaceWrapper) GetTxDetailsByAddressTxId(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "address" -------------
	var address string

	err = runtime.BindStyledParameter("simple", false, "address", ctx.Param("address"), &address)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter address: %s", err))
	}

	// ------------- Path parameter "txid" -------------
	var txid string

	err = runtime.BindStyledParameter("simple", false, "txid", ctx.Param("txid"), &txid)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter txid: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetTxDetailsByAddressTxId(ctx, address, txid)
	return err
}

// RegisterHandlers adds each server route to the EchoRouter.
func RegisterHandlers(router runtime.EchoRouter, si ServerInterface) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.GET("/v1/assets/:asset", wrapper.GetAssetInfo)
	router.GET("/v1/doc", wrapper.GetDocs)
	router.GET("/v1/health", wrapper.GetHealth)
	router.GET("/v1/pools", wrapper.GetPools)
	router.GET("/v1/pools/:asset", wrapper.GetPoolsData)
	router.GET("/v1/stakers", wrapper.GetStakersData)
	router.GET("/v1/stakers/:address", wrapper.GetStakersAddressData)
	router.GET("/v1/stakers/:address/:asset", wrapper.GetStakersAddressAndAssetData)
	router.GET("/v1/stats", wrapper.GetStats)
	router.GET("/v1/swagger.json", wrapper.GetSwagger)
	router.GET("/v1/thorchain/pool_addresses", wrapper.GetThorchainProxiedEndpoints)
	router.GET("/v1/tx/asset/:asset", wrapper.GetTxDetailsByAsset)
	router.GET("/v1/tx/:address", wrapper.GetTxDetails)
	router.GET("/v1/tx/:address/asset/:asset", wrapper.GetTxDetailsByAddressAsset)
	router.GET("/v1/tx/:address/txid/:txid", wrapper.GetTxDetailsByAddressTxId)

}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
	"AAA=",
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file.
func GetSwagger() (*openapi3.Swagger, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %s", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}

	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error loading Swagger: %s", err)
	}
	return swagger, nil
}

