package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/decimal"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
	"gitlab.com/thorchain/midgard/openapi/generated/oapigen"
	"gopkg.in/yaml.v3"
)

// If you want to update decimal of the pools, run this script in the command line: `go run ./cmd/decimal`
// If the script succeeds it will create the result in the `internal/decimal/decimals.json`

type ResultMap decimal.NativeDecimalMap

func main() {
	midlog.LogCommandLine()
	config.ReadGlobal()

	thorNodePools := readFromThorNodePools()
	midgardPools := readFromMidgardPools()
	manualPools := readManualJson()

	finalMergedPools := make(ResultMap)
	finalMergedPools.mergeFrom(thorNodePools, midgardPools, manualPools)
	finalMergedPools.mergeFrom(getERC20decimal(finalMergedPools))

	checkMissingDecimals(finalMergedPools)

	content, err := json.MarshalIndent(finalMergedPools, "", " ")
	if err != nil {
		midlog.FatalE(err, "Can't Marshal the resulted decimal pools to json.")
	}

	err = ioutil.WriteFile("./internal/decimal/decimals.json", content, 0644)
	if err != nil {
		midlog.FatalE(err, "Can't Marshal pools to decimals json.")
	}

	midlog.Info("decimals.json is created successfully.")
}

type PoolsResponse struct {
	Pools []struct {
		Asset   string `json:"asset"`
		Decimal int64  `json:"decimals"` // This field is might be filled only in the ThorNode response
	}
}

type UrlEndpoint struct {
	url     string
	network string
}

func readFromThorNodePools() ResultMap {
	urls := []UrlEndpoint{
		{
			url:     "https://thornode.ninerealms.com",
			network: "thornode-mainnet",
		},
		{
			url:     "https://stagenet-thornode.ninerealms.com",
			network: "thornode-stagenet",
		},
	}

	pools := ResultMap{}
	for _, ue := range urls {
		var res PoolsResponse
		queryEndpoint(ue.url, "/thorchain/pools", &res.Pools)
		pools.mergeFrom(res.toResultMap(ue.network))
	}

	return pools
}

func readFromMidgardPools() ResultMap {
	urls := []UrlEndpoint{
		{
			url:     "https://midgard.thorchain.info",
			network: "midgard-mainnet",
		},
		{
			url:     "https://stagenet-midgard.ninerealms.com",
			network: "midgard-stagenet",
		},
	}

	pools := ResultMap{}
	for _, ue := range urls {
		var res oapigen.KnownPools
		queryEndpoint(ue.url, "/v2/knownpools", &res.AdditionalProperties)
		pools.mergeFrom(knownPoolsToResultMap(res, ue.network))
	}

	return pools
}

func (pr PoolsResponse) toResultMap(network string) ResultMap {
	mapPools := ResultMap{}
	for _, p := range pr.Pools {
		decimals := p.Decimal
		decimalSource := []string{}
		if decimals == 0 {
			decimals = -1
		} else if 0 < decimals {
			decimalSource = append(decimalSource, network)
		}
		mapPools[p.Asset] = decimal.NativeDecimalSingle{
			NativeDecimals: decimals,
			AssetSeen:      []string{network},
			DecimalSource:  decimalSource,
		}
	}
	return mapPools
}

func knownPoolsToResultMap(knownPools oapigen.KnownPools, network string) ResultMap {
	mapPools := ResultMap{}
	for p := range knownPools.AdditionalProperties {
		mapPools[p] = decimal.NativeDecimalSingle{
			NativeDecimals: -1,
			AssetSeen:      []string{network},
			DecimalSource:  []string{},
		}
	}
	return mapPools
}

func (to *ResultMap) mergeFrom(from ...ResultMap) {
	for _, f := range from {
		for poolName, fromInfo := range f {
			toInfo, ok := (*to)[poolName]
			if !ok {
				toInfo.NativeDecimals = -1
			}
			toInfo.AssetSeen = append(toInfo.AssetSeen, fromInfo.AssetSeen...)
			toInfo.DecimalSource = append(toInfo.DecimalSource, fromInfo.DecimalSource...)
			if toInfo.DecimalSource == nil {
				toInfo.DecimalSource = []string{}
			}
			if toInfo.NativeDecimals == -1 {
				toInfo.NativeDecimals = fromInfo.NativeDecimals
			} else {
				if -1 < fromInfo.NativeDecimals && fromInfo.NativeDecimals != toInfo.NativeDecimals {
					midlog.Fatal(fmt.Sprintf(
						"The %s source has %d decimal which is different than %d decimals on %v",
						fromInfo.AssetSeen,
						fromInfo.NativeDecimals,
						toInfo.NativeDecimals,
						toInfo.AssetSeen))
				}
			}
			(*to)[poolName] = toInfo
		}
	}
}

func checkMissingDecimals(pools ResultMap) {
	for poolName, pool := range pools {
		if pool.NativeDecimals == -1 {
			midlog.Warn(fmt.Sprintf("%s pool doesn't have native decimal. Please add it to manual.yaml", poolName))
		}
	}
}

func queryEndpoint(urlAddress string, urlPath string, dest interface{}) {
	url := urlAddress + urlPath
	midlog.DebugF("Querying the endpoint: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		midlog.FatalE(err, fmt.Sprintf("Error while querying endpoint: %s", url+urlPath))
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		midlog.Fatal("Cannot read the body of the response")
	}

	err = json.Unmarshal(body, dest)
	if err != nil {
		midlog.FatalE(err, fmt.Sprintf("Error while querying endpoint: %s", url+urlPath))
	}

}

func queryEthplorerAsset(assetAddress string) int64 {
	url := fmt.Sprintf("https://api.ethplorer.io/getTokenInfo/%s?apiKey=freekey", assetAddress)

	midlog.DebugF("Querying Ethplorer: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		midlog.FatalE(err, "Error querying Ethplorer")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		midlog.Fatal("Can't read the reponse body.")
	}

	var dest EthResponse
	err = json.Unmarshal(body, &dest)
	if err != nil {
		midlog.WarnF("Json unmarshal error for url: %s", url)
		midlog.FatalE(err, "Error unmarshalling ThorNode response")
	}

	decimal, err := strconv.ParseInt(dest.Decimals, 10, 64)
	if err != nil {
		midlog.FatalE(err, "Can't parse the decimal")
	}

	return decimal
}

type EthResponse struct {
	Decimals string `json:"decimals"`
	Result   string `json:"result"`
}

func getERC20decimal(pools ResultMap) ResultMap {
	ercMap := decimal.NativeDecimalMap{}
	cnt := 0
	for k := range pools {
		if strings.HasPrefix(k, "ETH") && k != "ETH.ETH" {
			r := strings.Split(k, "-")
			// There is rare case that pool `ETH/ETH` (suspended) in stagnet `knownpools` endpoint,
			// is malformatted and can't be parsed
			if len(r) < 2 {
				continue
			}
			nativeDecimal := queryEthplorerAsset(r[1])
			if nativeDecimal != 0 && nativeDecimal != -1 {
				ercMap[k] = decimal.NativeDecimalSingle{
					NativeDecimals: nativeDecimal,
					AssetSeen:      []string{},
					DecimalSource:  []string{"ERC20"},
				}
			}
			cnt++
			// sleeps for 1 seconds to aviod Freekey limit
			if cnt%2 == 0 {
				time.Sleep(1 * time.Second)
			}
		}
	}

	return ercMap
}

func readManualJson() ResultMap {
	yamlFile, err := os.Open("./cmd/decimal/manual.yaml")
	manualResult := make(ResultMap)
	if err != nil {
		midlog.Fatal("There was no manual.yaml file")
		return manualResult
	}
	defer yamlFile.Close()

	var rawPools map[string]int64
	if err == nil {
		rawData, err := ioutil.ReadAll(yamlFile)
		if err != nil {
			midlog.FatalE(err, "Can't read manual.yaml")
		}
		err = yaml.Unmarshal(rawData, &rawPools)
		if err != nil {
			midlog.FatalE(err, "Can't Unmarshal manual pools yaml.")
		}
	}

	for p, v := range rawPools {
		manualResult[p] = decimal.NativeDecimalSingle{
			NativeDecimals: v,
			AssetSeen:      []string{"constants"},
			DecimalSource:  []string{"constants"},
		}
	}

	return manualResult
}
