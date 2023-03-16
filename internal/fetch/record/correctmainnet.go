package record

import (
	_ "embed"
	"encoding/json"

	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// This file contains many small independent corrections

const ChainIDMainnet202104 = "thorchain"
const ChainIDMainnet202203 = "thorchain-mainnet-v1"

func loadMainnet202104Corrections(chainID string) {
	if chainID == ChainIDMainnet202104 {
		log.Info().Msgf(
			"Loading corrections for chaosnet started on 2021-04 id: %s",
			chainID)

		loadMainnetCorrectionsWithdrawImpLoss()
		loadMainnetWithdrawForwardedAssetCorrections()
		loadMainnetWithdrawIncreasesUnits()
		loadMainnetcorrectGenesisNode()
		loadMainnetMissingWithdraws()
		loadMainnetRemoveLiquidityCorrections()
		loadMainnetBalanceCorrections()
		loadMainnetMissingRefund()
		loadMainnetPreregisterThornames()
		registerArtificialPoolBallanceChanges(
			mainnetArtificialDepthChanges, "Midgard fix on mainnet")
		// Actually block 2104917 (2021-09-15) upon the switch from v0.64.0 to v0.67.0,
		// specifically the implementation of THORNode MR !1834 resolving THORNode Issue #1052.
		withdrawCoinKeptHeight = 1970000
		GlobalWithdrawCorrection = correctWithdawsMainnetFilter
	}

	if chainID == ChainIDMainnet202203 {
		// This is the block (2023-03-16) upon the switch from v1.106.0 to v1.107.0,
		// specifically the implementation of THORNode MR !2777 resolving THORNode Issue #1415.
		withdrawCoinPooledHeight = 9989661
	}
}

//////////////////////// Activate genesis node.

// Genesis node bonded rune and became listed as Active without any events.
func loadMainnetcorrectGenesisNode() {
	AdditionalEvents.Add(12824, func(meta *Metadata) {
		updateNodeAccountStatus := UpdateNodeAccountStatus{
			NodeAddr: []byte("thor1xfqaqhk5r6x9hdwlvmye0w9agv8ynljacmxulf"),
			Former:   []byte("Ready"),
			Current:  []byte("Active"),
		}
		Recorder.OnUpdateNodeAccountStatus(&updateNodeAccountStatus, meta)
	})
}

//////////////////////// Missing Withdraws

type AdditionalWithdraw struct {
	Pool     string
	FromAddr string
	Reason   string
	RuneE8   int64
	AssetE8  int64
	Units    int64
}

func (w *AdditionalWithdraw) Record(meta *Metadata) {
	reason := []byte(w.Reason)
	chain := strings.Split(w.Pool, ".")[0]

	hashF := fnv.New32a()
	fmt.Fprint(hashF, w.Reason, w.Pool, w.FromAddr, w.RuneE8, w.AssetE8, w.Units)
	txID := strconv.Itoa(int(hashF.Sum32()))

	withdraw := Withdraw{
		FromAddr:    []byte(w.FromAddr),
		Chain:       []byte(chain),
		Pool:        []byte(w.Pool),
		Asset:       []byte("THOR.RUNE"),
		ToAddr:      reason,
		Memo:        reason,
		Tx:          []byte(txID),
		EmitRuneE8:  w.RuneE8,
		EmitAssetE8: w.AssetE8,
		StakeUnits:  w.Units,
	}
	Recorder.OnWithdraw(&withdraw, meta)
}

func addWithdraw(height int64, w AdditionalWithdraw) {
	AdditionalEvents.Add(height, w.Record)
}

func loadMainnetMissingWithdraws() {
	// A failed withdraw actually modified the pool, bug was corrected to not repeat again:
	// https://gitlab.com/thorchain/thornode/-/merge_requests/1643
	addWithdraw(63519, AdditionalWithdraw{
		Pool:     "BNB.BNB",
		FromAddr: "thor1tl9k7fjvye4hkvwdnl363g3f2xlpwwh7k7msaw",
		Reason:   "bug 1643 corrections fix for assymetric rune withdraw problem",
		RuneE8:   1999997,
		AssetE8:  0,
		Units:    1029728,
	})

	// TODO(muninn): find out reason for the divergence and document.
	// Discussion:
	// https://discord.com/channels/838986635756044328/902137599559335947
	addWithdraw(2360486, AdditionalWithdraw{
		Pool:     "BCH.BCH",
		FromAddr: "thor1nlkdr8wqaq0wtnatckj3fhem2hyzx65af8n3p7",
		Reason:   "midgard correction missing withdraw",
		RuneE8:   1934186,
		AssetE8:  29260,
		Units:    1424947,
	})
	addWithdraw(2501774, AdditionalWithdraw{
		Pool:     "BNB.BUSD-BD1",
		FromAddr: "thor1prlky34zkpr235lelpan8kj8yz30nawn2cuf8v",
		Reason:   "midgard correction missing withdraw",
		RuneE8:   1481876,
		AssetE8:  10299098,
		Units:    962674,
	})

	// On Pool suspension the withdraws had FromAddr=null and they were skipped by Midgard.
	// Later the pool was reactivated, so having correct units is important even at suspension.
	// There is a plan to fix ThorNode events:
	// https://gitlab.com/thorchain/thornode/-/issues/1164
	addWithdraw(2606240, AdditionalWithdraw{
		Pool:     "BNB.FTM-A64",
		FromAddr: "thor14sz7ca8kwhxmzslds923ucef22pm0dh28hhfve",
		Reason:   "midgard correction suspended pool withdraws missing",
		RuneE8:   0,
		AssetE8:  0,
		Units:    768586678,
	})
	addWithdraw(2606240, AdditionalWithdraw{
		Pool:     "BNB.FTM-A64",
		FromAddr: "thor1jhuy9ft2rgr4whvdks36sjxee5sxfyhratz453",
		Reason:   "midgard correction suspended pool withdraws missing",
		RuneE8:   0,
		AssetE8:  0,
		Units:    110698993,
	})
	addWithdraw(2606240, AdditionalWithdraw{
		Pool:     "BNB.FTM-A64",
		FromAddr: "thor19wcfdx2yk8wjze7l0cneynjvjyquprjwj063vh",
		Reason:   "midgard correction suspended pool withdraws missing",
		RuneE8:   0,
		AssetE8:  0,
		Units:    974165115,
	})
	addWithdraw(1166400, AdditionalWithdraw{
		Pool:     "ETH.WBTC-0X2260FAC5E5542A773AA44FBCFEDF7C193BC2C599",
		FromAddr: "thor1g6pnmnyeg48yc3lg796plt0uw50qpp7hgz477u",
		Reason:   "midgard correction suspended pool withdraws missing",
		RuneE8:   0,
		AssetE8:  0,
		Units:    2228000000,
	})
}

//////////////////////// Missing Swap Refund

// https://gitlab.com/thorchain/thornode/-/merge_requests/2716
// tried to refund ETH asset that never
// went out by sending RUNE back to the user from Asgard Module
func loadMainnetMissingRefund() {
	// First Tx
	AdditionalEvents.Add(9187906, func(meta *Metadata) {
		swap := Swap{
			// fabricated txID = keccak-256(memo)
			Tx:             []byte("EE31ACC02D631DC3220990A1DD2E9030F4CFC227A61E975B5DEF1037106D1CCD"),
			Chain:          []byte("ETH"),
			FromAddr:       []byte("0x094a8C7478fCcb34bc84577C1230DA8E7913B7D5"),
			ToAddr:         []byte("0xe4ddca21881bac219af7f217703db0475d2a9f02"),
			FromAsset:      []byte("ETH.ETH"),
			FromE8:         540874489,
			ToAsset:        []byte("THOR.RUNE"),
			ToE8:           450000000000,
			Memo:           []byte("REFUND:B07A6B1B40ADBA2E404D9BCE1BEF6EDE6F70AD135E83806E4F4B6863CF637D0B"),
			Pool:           []byte("ETH.ETH"),
			SwapSlipBP:     0,
			ToE8Min:        0,
			LiqFeeInRuneE8: 0,
			LiqFeeE8:       0,
		}
		Recorder.OnSwap(&swap, meta)
	})

	// Second Tx
	AdditionalEvents.Add(9187906, func(meta *Metadata) {
		swap := Swap{
			// fabricated txID = keccak-256(memo)
			Tx:             []byte("0A61B99DC6B1A4499A72238AC767C09C310326875F9E7B870C908357B09202E9"),
			Chain:          []byte("ETH"),
			FromAddr:       []byte("0x094a8C7478fCcb34bc84577C1230DA8E7913B7D5"),
			ToAddr:         []byte("0xe4ddca21881bac219af7f217703db0475d2a9f02"),
			FromAsset:      []byte("ETH.ETH"),
			FromE8:         541086437,
			ToAsset:        []byte("THOR.RUNE"),
			ToE8:           450000000000,
			Memo:           []byte("REFUND:4795A3C036322493A9692B5D44E7D4FF29C3E2C1E848637184E98FE8B05FD06E"),
			Pool:           []byte("ETH.ETH"),
			SwapSlipBP:     0,
			ToE8Min:        0,
			LiqFeeInRuneE8: 0,
			LiqFeeE8:       0,
		}
		Recorder.OnSwap(&swap, meta)
	})
}

//////////////////////// Fix HEGIC pool missbalance.

// When pool gets removed by thornode `removeLiquidityProviders` the event contains null value
// And gets ignored by Midgard. Here is the fix from thornode:
// https://gitlab.com/thorchain/thornode/-/merge_requests/2819

func loadMainnetRemoveLiquidityCorrections() {
	corrections := []Withdraw{
		{
			FromAddr:   []byte("0x8ea72dffeb4c752d67d6590673b1c2c3ff7afe33"),
			StakeUnits: 2530873057,
		},
		{
			FromAddr:   []byte("0x9fd89ff191c9b4c4e25e0b3e3e16783514e480eb"),
			StakeUnits: 346609448,
		},
		{
			FromAddr:   []byte("thor123hnhqjlfjny7rzj8fxzgdlkmxjvup279dpkq0"),
			StakeUnits: 291454169,
		},
		{
			FromAddr:   []byte("thor125ynwnann7969mjhynqcp5je834kunz4cx0rq7"),
			StakeUnits: 3375794,
		},
		{
			FromAddr:   []byte("thor12qjtp8c5hhl9e5rl04ynfvlwfp2xucsqhdw92s"),
			StakeUnits: 23494480,
		},
		{
			FromAddr:   []byte("thor13f2zmmjkflfyeghupyegf3z44e95s8nfg479jd"),
			StakeUnits: 1868006,
		},
		{
			FromAddr:   []byte("thor15sj246ny895psa3wymlt7am7slcr8n94t3em89"),
			StakeUnits: 12069444899,
		},
		{
			FromAddr:   []byte("thor16fyjsvr4p08rzpkaxk6r5d20mst5ln7mthq4yt"),
			StakeUnits: 346542576,
		},
		{
			FromAddr:   []byte("thor16jg26p48ue58kjxsu5l5w0gx6qdlaxhxsnrjzu"),
			StakeUnits: 465518306,
		},
		{
			FromAddr:   []byte("thor178wlyhtpvth7f7zmrusfcqurvysxv9jngalunk"),
			StakeUnits: 32844877,
		},
		{
			FromAddr:   []byte("thor18ealt7vp5w362mqsjgjwdm7dxgu0032vs4xlc0"),
			StakeUnits: 3372615,
		},
		{
			FromAddr:   []byte("thor18l7mjv9lv3j3q4f3gh3fffyms8cpvjquvywcnc"),
			StakeUnits: 18682220,
		},
		{
			FromAddr:   []byte("thor19dq0hcxmau6tp6fgkl30vf2snkntv2jpq0skfp"),
			StakeUnits: 16997920483,
		},
		{
			FromAddr:   []byte("thor19kuxhqy870ysu5ej65rw4ezu8cejjj6p88tpar"),
			StakeUnits: 328354924,
		},
		{
			FromAddr:   []byte("thor19ytzkff7a3qy7nmnkx5a82urehuuxr3sjwrgmw"),
			StakeUnits: 6982566186,
		},
		{
			FromAddr:   []byte("thor1amxc5dg4eetej7pg4nkzw60cpr6t4809xf8tnv"),
			StakeUnits: 33125146,
		},
		{
			FromAddr:   []byte("thor1axsyxqkfxzaf86kkxmy0ltjdsjneqkun5lkjpx"),
			StakeUnits: 337576,
		},
		{
			FromAddr:   []byte("thor1c7a5lqssq7esh8tp3jmezaavvax5pmg65eevt5"),
			StakeUnits: 854431703,
		},
		{
			FromAddr:   []byte("thor1ezr4ky0ve2fzqr9s4estpg9zq9g2dk9q5thzhu"),
			StakeUnits: 1255928536,
		},
		{
			FromAddr:   []byte("thor1fn46pkfs7tcme664492m4z5jwf8cmyp4anlnfe"),
			StakeUnits: 3732928,
		},
		{
			FromAddr:   []byte("thor1fq2fhrk84hxyme2ta35ne6v35u7zh4k0wqsx06"),
			StakeUnits: 299940491,
		},
		{
			FromAddr:   []byte("thor1fs5f4adte00m78j7g6ztkjvln9p8p5heymd2wz"),
			StakeUnits: 74670193,
		},
		{
			FromAddr:   []byte("thor1gjsuaqca7u64u2zx5wwded53j94k5yyrmfnz2w"),
			StakeUnits: 3416668,
		},
		{
			FromAddr:   []byte("thor1gp7dufplc92sxdlxfsmj6fx8uvya7mydcrdag9"),
			StakeUnits: 40241,
		},
		{
			FromAddr:   []byte("thor1hfvq8s40e5s83055jagmzrnhp3ugda64wwtya6"),
			StakeUnits: 35601008,
		},
		{
			FromAddr:   []byte("thor1hjfu0jhkutq942ucs7nzvfrfa30kwtxxcy7yvf"),
			StakeUnits: 383673,
		},
		{
			FromAddr:   []byte("thor1jf074m34qjaxpkh4mev2uaz6x249npfvpwwwtk"),
			StakeUnits: 696625,
		},
		{
			FromAddr:   []byte("thor1jmawmz2hvgdzmhsuh8phyktwhwrz5u9v4h63ce"),
			StakeUnits: 186818,
		},
		{
			FromAddr:   []byte("thor1kd9h7m9l2h8wf2js72caqzq4978p2mcsnvta2s"),
			StakeUnits: 100539730,
		},
		{
			FromAddr:   []byte("thor1knsxcjtz3raqyzxk5z87senlwx53v9hkncjrv4"),
			StakeUnits: 411925945,
		},
		{
			FromAddr:   []byte("thor1l5vlnutqvh2zv3uruy6kk0s6dunz0xj6z048pa"),
			StakeUnits: 331272269,
		},
		{
			FromAddr:   []byte("thor1lghm6wmqtrz5mzupm9c5lk4xtr2xv3835rf935"),
			StakeUnits: 8292276,
		},
		{
			FromAddr:   []byte("thor1m07uxkdpgxyt7um0g36z62z02ehzjfgzvpeewp"),
			StakeUnits: 2993590,
		},
		{
			FromAddr:   []byte("thor1m490synp6glkq4skext84x6xfqgg3j69l0ngpp"),
			StakeUnits: 32050412,
		},
		{
			FromAddr:   []byte("thor1mtch7p7ctmhz0fatg38zz64rlvu95c5vhsrm3f"),
			StakeUnits: 4021806,
		},
		{
			FromAddr:   []byte("thor1n00szt2yqrne3dmlpqd04swhmmrujh7hqp9a4v"),
			StakeUnits: 8043290,
		},
		{
			FromAddr:   []byte("thor1n244qlmax8kkpjre7wrluy3kxx3frnzl37m95n"),
			StakeUnits: 98542193,
		},
		{
			FromAddr:   []byte("thor1n9s7t8hscr7j8g5kzacscdream72kx2u4w3dlc"),
			StakeUnits: 3286235,
		},
		{
			FromAddr:   []byte("thor1pavmt8hlv6v7r70y8dugqfyd98hf39h94ty25j"),
			StakeUnits: 7112626,
		},
		{
			FromAddr:   []byte("thor1r0aw24dwea2373h2mfrx3ut04s2jqnvj67p3jp"),
			StakeUnits: 1871484,
		},
		{
			FromAddr:   []byte("thor1rdttlqjj3a8puat9h4xqvc7jxewaa5yf4x5tdq"),
			StakeUnits: 3736128,
		},
		{
			FromAddr:   []byte("thor1tgs7xd2xq57vwhgzzv7qv9d68plpn0hxw7ufxa"),
			StakeUnits: 37782627,
		},
		{
			FromAddr:   []byte("thor1ukag0a5l0p4gqdmmzq3lw0cycuks73nddaurpx"),
			StakeUnits: 116437252,
		},
		{
			FromAddr:   []byte("thor1ve0z79dtukzun96shykyr7z8zdufzezkqzhzqd"),
			StakeUnits: 37784440,
		},
		{
			FromAddr:   []byte("thor1vjm299wprqwz5m0kp5dnaph573u4glk2x3dx3y"),
			StakeUnits: 2939544248,
		},
		{
			FromAddr:   []byte("thor1vqd6djdqn4hlrquxuwz2na3mac8qm8qgwfu7ps"),
			StakeUnits: 66276710,
		},
		{
			FromAddr:   []byte("thor1wmme5j3jd4jtldxcvq2mrgcrc5mtctj606fjnm"),
			StakeUnits: 15084927,
		},
		{
			FromAddr:   []byte("thor1wp8s062268tv3fvxxl8tgy62aq0rk203pylh5z"),
			StakeUnits: 3154314583,
		},
		{
			FromAddr:   []byte("thor1ws0sltg9ayyxp2777xykkqakwv2hll5ywuwkzl"),
			StakeUnits: 31875829,
		},
		{
			FromAddr:   []byte("thor1wtttw6j3xvxm85j80m3qv3zhz70wem0a6xkh37"),
			StakeUnits: 1657489,
		},
		{
			FromAddr:   []byte("thor1x64thscxsl39pun3lqzwhge890tvhvgd3dxc7q"),
			StakeUnits: 46107090,
		},
		{
			FromAddr:   []byte("thor1y45hs4jaufkckmz4kj6gu30h7c9s7v4t2unwn2"),
			StakeUnits: 63077766,
		},
		{
			FromAddr:   []byte("thor1yv36jpsn2qvds7vg74g0fpyfu6d5np2hhdp627"),
			StakeUnits: 65694437,
		},
		{
			FromAddr:   []byte("thor1zw4n0zgf7v75l9t6hfyw0fgz9tn84vfu9jz77h"),
			StakeUnits: 33015622,
		},
	}

	fn := func(meta *Metadata) {
		for _, c := range corrections {
			withdraw := Withdraw{
				Asymmetry:           0.0,
				BasisPoints:         10000,
				Chain:               []byte("THOR"),
				Pool:                []byte("ETH.HEGIC-0X584BC13C7D411C00C01A62E8019472DE68768430"),
				Asset:               []byte("THOR.RUNE"),
				ToAddr:              []byte(""),
				Memo:                []byte(""),
				Tx:                  []byte("0000000000000000000000000000000000000000000000000000000000000000"),
				EmitRuneE8:          0,
				EmitAssetE8:         0,
				ImpLossProtectionE8: 0,
				FromAddr:            c.FromAddr,
				StakeUnits:          c.StakeUnits,
			}
			Recorder.OnWithdraw(&withdraw, meta)
		}
	}
	AdditionalEvents.Add(7171200, fn)
}

//////////////////////// Fix withdraw assets not forwarded.

// In the early blocks of the chain the asset sent in with the withdraw initiation
// was not forwarded back to the user. This was fixed for later blocks:
//  https://gitlab.com/thorchain/thornode/-/merge_requests/1635

func correctWithdawsForwardedAsset(withdraw *Withdraw, meta *Metadata) KeepOrDiscard {
	withdraw.AssetE8 = 0
	return Keep
}

// generate block heights where this occured:
//
//	select FORMAT('    %s,', b.height)
//	from withdraw_events as x join block_log as b on x.block_timestamp = b.timestamp
//	where asset_e8 != 0 and asset != 'THOR.RUNE' and b.height < 220000;
func loadMainnetWithdrawForwardedAssetCorrections() {
	var heightWithOldWithdraws []int64
	heightWithOldWithdraws = []int64{
		29113,
		110069,
	}
	for _, height := range heightWithOldWithdraws {
		WithdrawCorrections.Add(height, correctWithdawsForwardedAsset)
	}
}

func correctWithdawsMainnetFilter(withdraw *Withdraw, meta *Metadata) KeepOrDiscard {
	// In the beginning of the chain withdrawing pending liquidity emitted a
	// withdraw event with units=0.
	// This was later corrected, and pending_liquidity events are emitted instead.
	if withdraw.StakeUnits == 0 && meta.BlockHeight < 1000000 {
		return Discard
	}
	return Keep
}

//////////////////////// Follow ThorNode bug on withdraw (units and rune was added to the pool)

// https://gitlab.com/thorchain/thornode/-/issues/954
// ThorNode added units to a member after a withdraw instead of removing.
// The bug was corrected, but an arbitrage account hit this bug 13 times.
//
// The values were generated with cmd/statechecks
// The member address was identified with cmd/membercheck
func loadMainnetWithdrawIncreasesUnits() {
	type MissingAdd struct {
		AdditionalRune  int64
		AdditionalUnits int64
	}
	corrections := map[int64]MissingAdd{
		672275: {
			AdditionalRune:  2546508574,
			AdditionalUnits: 967149543,
		},
		674411: {
			AdditionalRune:  1831250392,
			AdditionalUnits: 704075160,
		},
		676855: {
			AdditionalRune:  1699886243,
			AdditionalUnits: 638080440,
		},
		681060: {
			AdditionalRune:  1101855537,
			AdditionalUnits: 435543069,
		},
		681061: {
			AdditionalRune:  1146177337,
			AdditionalUnits: 453014832,
		},
		681063: {
			AdditionalRune:  271977087,
			AdditionalUnits: 106952192,
		},
		681810: {
			AdditionalRune:  3830671893,
			AdditionalUnits: 1518717776,
		},
		681815: {
			AdditionalRune:  2749916233,
			AdditionalUnits: 1090492640,
		},
		681819: {
			AdditionalRune:  540182490,
			AdditionalUnits: 213215502,
		},
		682026: {
			AdditionalRune:  1108123249,
			AdditionalUnits: 443864231,
		},
		682028: {
			AdditionalRune:  394564637,
			AdditionalUnits: 158052776,
		},
		682031: {
			AdditionalRune:  1043031822,
			AdditionalUnits: 417766496,
		},
		682128: {
			AdditionalRune:  3453026237,
			AdditionalUnits: 1384445390,
		},
	}

	correct := func(meta *Metadata) {
		missingAdd := corrections[meta.BlockHeight]
		stake := Stake{
			AddBase: AddBase{
				Pool:     []byte("ETH.ETH"),
				RuneAddr: []byte("thor1hyarrh5hslcg3q5pgvl6mp6gmw92c4tpzdsjqg"),
				RuneE8:   missingAdd.AdditionalRune,
			},
			StakeUnits: missingAdd.AdditionalUnits,
		}
		Recorder.OnStake(&stake, meta)
	}
	for k := range corrections {
		AdditionalEvents.Add(k, correct)
	}
}

var mainnetArtificialDepthChanges = artificialPoolBallanceChanges{
	// Bug was fixed in ThorNode: https://gitlab.com/thorchain/thornode/-/merge_requests/1765
	1043090: {{"BCH.BCH", -1, 0}},
	// Fix for ETH chain attack was submitted to Thornode, but some needed events were not emitted:
	// https://gitlab.com/thorchain/thornode/-/merge_requests/1815/diffs?commit_id=9a7d8e4a1b0c25cb4d56c737c5fe826e7aa3e6f2
	1483166: {{"ETH.YFI-0X0BC529C00C6401AEF6D220BE8C6EA1667F6AD93E", -18571915693442, 555358575},
		{"ETH.ETH", 42277574737435, -725548423675},
		{"ETH.SUSHI-0X6B3595068778DD592E39A122F4F5A5CF09C90FE2", -19092838139426, 3571961904132},
		{"ETH.HEGIC-0X584BC13C7D411C00C01A62E8019472DE68768430", -69694964523, 2098087620642},
		{"ETH.AAVE-0X7FC66500C84A76AD7E9C93437BFC5AC33E2DDAE9", -2474742542086, 20912604795},
		{"ETH.DODO-0X43DFC4159D86F3A37A5A4B3D4580B888AD7D4DDD", -38524681038, 83806675976},
		{"ETH.KYL-0X67B6D479C7BB412C54E03DCA8E1BC6740CE6B99C", -2029858716920, 35103388382444},
	},
	// TODO(muninn): document divergency reason
	2597851: {
		{"ETH.ALCX-0XDBDB4D16EDA451D0503B854CF79D55697F90C8DF", 0, -96688561785},
		{"ETH.SUSHI-0X6B3595068778DD592E39A122F4F5A5CF09C90FE2", 0, -5159511094095},
		{"ETH.USDT-0XDAC17F958D2EE523A2206206994597C13D831EC7", 0, -99023689717400},
		{"ETH.XRUNE-0X69FA0FEE221AD11012BAB0FDB45D444D3D2CE71C", 0, -2081880169421610},
		{"ETH.YFI-0X0BC529C00C6401AEF6D220BE8C6EA1667F6AD93E", 0, -727860649},
	},
	// Store migration to fix USDC sent to an old vault that was improperly observed:
	// https://gitlab.com/thorchain/thornode/-/merge_requests/2361
	6186308: {
		{"ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 0, -2673666000000},
	},
}

//////////////////////// Balance corrections

// Due to missing transfer events, account balances diverged compared to thornode.
// These synthethic correction transfers generate compensating transfers from or to
// the midgard-balance-correction-address.
//
// The corrections are not precise, for simplicity were set to the first fork height 4786560,
// except in the case of genesis BaseAccount set to height 1.
//
// Generated with cmd/checks/balance/balancecheck.go
func loadMainnetBalanceCorrections() {
	type Correction struct {
		asset    string
		fromAddr string
		toAddr   string
		amountE8 int64
	}
	heightCorrections := map[int64][]Correction{}
	heightCorrections[1] = []Correction{
		// genesis BaseAccount
		{asset: "THOR.RUNE", fromAddr: MidgardBalanceCorrectionAddress, toAddr: "thor1xfqaqhk5r6x9hdwlvmye0w9agv8ynljacmxulf", amountE8: 100000000},
	}
	heightCorrections[4786560] = []Correction{
		{asset: "THOR.MIMIR", fromAddr: "MidgardBalanceCorrectionAddress", toAddr: "thor19pkncem64gajdwrd5kasspyj0t75hhkpqjn5qh", amountE8: 100000000000},
		{asset: "THOR.MIMIR", fromAddr: "MidgardBalanceCorrectionAddress", toAddr: "thor1xghvhe4p50aqh5zq2t2vls938as0dkr2mzgpgh", amountE8: 100000000000},
		{asset: "THOR.RUNE", fromAddr: "MidgardBalanceCorrectionAddress", toAddr: "thor17xpfvakm2amg962yls6f84z3kell8c5lk76m7z", amountE8: 857361851},
		{asset: "THOR.RUNE", fromAddr: "thor106r2jdgpdjhkv0k9apr75k35snx72ymexzesc9", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor10jr5p2ldd3whppnukeun8rqksfpktjpwkkhhfp", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 28000000},
		{asset: "THOR.RUNE", fromAddr: "thor10k9ncyq9qsqlwcdchh4628dncx77g82xknarju", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor10ne044874nkdx49xp2n8wjlr4qmmjynmll9pwg", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 26000000},
		{asset: "THOR.RUNE", fromAddr: "thor12882tsn8psfqkcr7yg9apr598eec2z6ejklheh", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor12zq08wljyqs0mculuhcv0cnzqww72rz4t8dmkk", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 20000000},
		{asset: "THOR.RUNE", fromAddr: "thor1303cvleev5v5r36xc3w785rmnpfkaq9vqfqvmp", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor139m38gmajx8k9njzpqwtpg8m5q666mru67jn64", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor13lat663qx8xuhc0yksgfcgaguud8l5v9q6476s", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 14008450},
		{asset: "THOR.RUNE", fromAddr: "thor13nlr0waphxp80pl66cljpf2dskljwuqnd6y9z6", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor145neurjz23qcnsj4wyc3p7lyvm7lxyv45pl9x0", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 14000000},
		{asset: "THOR.RUNE", fromAddr: "thor14pt4pds9ta0zutg7p9mshy9ua2s93fncarmwyf", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 60000},
		{asset: "THOR.RUNE", fromAddr: "thor156v9a0xxmlv5s0jf3qlaf56gp2haxv83qn7pym", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor15ewgz729xqj7vl4frseyejmhdgln6wyk9qdzen", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor15jk72cn4nn7y3zcnmte6uzw8hnwav68msjt2e0", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor16r7sn63534kns8un6fkma84w4nh0eyx638705z", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 10000000},
		{asset: "THOR.RUNE", fromAddr: "thor16zh2ukpgk62n9n0ghvq53ksgenfqx6e69lxm3c", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor179wpxmm5f7asaqwfwnnf8sn3rductlq3ywmrl0", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 81000000},
		{asset: "THOR.RUNE", fromAddr: "thor17c7kdsx7le2xzj5mvjeyvjv3g9rsqzct3rqrw4", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 28000000},
		{asset: "THOR.RUNE", fromAddr: "thor17fpn23us9ecygyk7hc7ys597na0y3g3f75z5jh", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 16000},
		{asset: "THOR.RUNE", fromAddr: "thor185tpa9awayq82qv8wn7a2dwnp8lkh5k8775q0p", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 28000000},
		{asset: "THOR.RUNE", fromAddr: "thor18el7shmfae98uqmu7924dmnqcwlsave3xkj4l2", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor18v9pa0vem262akwxfmury285zrzt7drmjmh69l", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor190fdyxc92whfmedsp8d0p6c8pce2ayxjm9zsl6", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor19g3xx3mm3h079uq39prx30tkah6h0cgajp9fmm", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor19zkcm4a7uvehhfem4sf83jmzazl9wljsa0w3kn", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2005600},
		{asset: "THOR.RUNE", fromAddr: "thor1anszvcrf86schunkdg6fggc5qdlv6q43cp4s2m", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 18000000},
		{asset: "THOR.RUNE", fromAddr: "thor1arr4d877nmgt9hhm58mllyt93v2dpnl53sedpv", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1cdgvlhs7m9wqc93yrpkqslnzun00vj265f9me5", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 28000000},
		{asset: "THOR.RUNE", fromAddr: "thor1cgsk8av248g75t3jk39erz5w7zcegp8atus0l2", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1d5yrsx7f244hqx0anvxzewngzjdr6pyu9j8vek", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1dm6ta7kd7906exklla76mczcq0cvq4q4dns3tj", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1dp7rglq4y3hjad3q0n7wnxx43k5n338jv3qhn4", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor1dx7x00xxey2avxkh4t7uxl0wcvmw5t6zcvrlny", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1dzglhdry3z8n2xpcr3sa36k55e4ulpu2n6dfp9", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 900},
		{asset: "THOR.RUNE", fromAddr: "thor1e3dver6l6tuqxq6pzvxv23k9harl0w0q9dj5ag", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1e4aw3hldyhf2wsntuw7uy69dpvrk8wme5p3fyy", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor1e902jc6mkwzzt06edpt8udj0s0hrh4445qef7t", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1enns4sa2weem5ee0q8fp4d2mmkx45q3lgfw6xp", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 9000},
		{asset: "THOR.RUNE", fromAddr: "thor1fjkq5t755gfxzqlxh9w34wt9d8wc750zf536k2", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor1fjpyu5wz4nrprmvchfrjaa8ml09c5c6gddyxpy", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1fzrr4smypv092dtaur9mhjzxv6hd90u2jz4wta", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1gt0jfpl3s9r9j8v4wjv2dxs4wzv9azzmpgrdaf", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 12000000},
		{asset: "THOR.RUNE", fromAddr: "thor1gz5krpemm0ce4kj8jafjvjv04hmhle576x8gms", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor1hmys2j4mk9rygywcn7nwwxkzq9z2cm2gkzqu87", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 20000000},
		{asset: "THOR.RUNE", fromAddr: "thor1jr08mgqvz3rc6x4srrkgud4ecwfyd2a97tynf2", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1klqtt0md9tlg5r29ehd3zhfdsqmmqwjvjwtsdn", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 100},
		{asset: "THOR.RUNE", fromAddr: "thor1l4dywkmf2gk4r5ezd00u73ss24k9mjtclthlm3", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1l68tc59fy3wez6ead32uvp3hdhdg2w5t9dxtf6", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1lg9qdmsmftkymtnjfeayzel62466rpq2pf4k26", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 22000000},
		{asset: "THOR.RUNE", fromAddr: "thor1ls33ayg26kmltw7jjy55p32ghjna09zp74t4az", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000},
		{asset: "THOR.RUNE", fromAddr: "thor1ls6hwrgvn303lmaafj6dqyappks80ltmsuzar0", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1m0gwuq7rr3kwxhue6579hv74mv6gvgnw5f67nh", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 16000000},
		{asset: "THOR.RUNE", fromAddr: "thor1ma6zknxflp0r7c9nkjuekjl90zfwpfx6ar5rcp", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4209000},
		{asset: "THOR.RUNE", fromAddr: "thor1msnlcmu755zxlnha0s9e7yadq2tdx33tk7d9rr", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 3000},
		{asset: "THOR.RUNE", fromAddr: "thor1nfx29v03v30rj9zmxfrqggu98q8w9uavzx9gpc", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1pe5taj0lfcfmeyse6jcs20thgrp2k2wpx2ka04", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1ptf0xerx5deren2eqwxssfu99w4y3v3dpyttxu", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1q8e586cjmefyrjhwxyhw77rcwgc9ne6yjzlk5h", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1qexyn7k7juz56xmmcyglsk7h9rlvr5ajh0fnqp", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1qx2ja7scp74y7v6z8mkurmvp4g6sxp8wty98a7", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 10000000},
		{asset: "THOR.RUNE", fromAddr: "thor1reu9yf2uvwv22n90t27n7hjfy4pjnng5pj0v8c", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1s03stghe35d3cptmq66dhaqwv7tt60aq6n9cdt", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1s8jgmfta3008lemq3x2673lhdv3qqrhw3psuhh", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4009000},
		{asset: "THOR.RUNE", fromAddr: "thor1sdehah2rl9w887qy0fhkgml3qhxrqs27cq7kh5", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 12000000},
		{asset: "THOR.RUNE", fromAddr: "thor1tcjt8wr0dcynehpf5yvwv8xrux2p3t4cxjucm5", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1tj5dcjgshep6vvc9dd587dzp0exh5cxxuls30c", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 32000000},
		{asset: "THOR.RUNE", fromAddr: "thor1tq9xzklm9nuuke8ma0kj2npkqa8jl3wsnlvgy4", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 10000000},
		{asset: "THOR.RUNE", fromAddr: "thor1tqpljp607j4szm0u6v5e0w3gw0e33e7xvcxvvy", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1u0h70rjxt8km9wtpcxar69k3me74ryj68jzjrn", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 24000000},
		{asset: "THOR.RUNE", fromAddr: "thor1udd9wjqxdynzchgt48q6vl2m8tkmx7lcnwdrg7", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 26000000},
		{asset: "THOR.RUNE", fromAddr: "thor1uenvdgn3zljqzy7zvss4mgtm6c78z5dj62pl9t", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor1uymnvlnvemfxdjucwde7gv30j3x9m2ulfgc2vw", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 22000000},
		{asset: "THOR.RUNE", fromAddr: "thor1v5adrn44u55a7pu28pzdufd09za5f5wlqv9hh3", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor1vh4ka53k4a4hd6apl5va8p6h4cevcnalm2t5hk", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 24000000},
		{asset: "THOR.RUNE", fromAddr: "thor1vhx64hwxpqx2r2zdz89p2vxkyd5m7xs5z3t2pt", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000001},
		{asset: "THOR.RUNE", fromAddr: "thor1w3455894cze7gxuce7t3dpjnkvgsku28hg8zfz", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 14000000},
		{asset: "THOR.RUNE", fromAddr: "thor1wfx7u28c32xu389v9dh0vdc5lq63lldwpzpxka", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1wvx96p8l80xhjuzd9tf037ztzc0sw73hl0e7sp", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1wy58774wagy4hkljz9mchhqtgk949zdwwe80d5", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 22009000},
		{asset: "THOR.RUNE", fromAddr: "thor1x00pfwyx8xld45sdlmyn29vjf7ev0mv3rcn9al", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 24200},
		{asset: "THOR.RUNE", fromAddr: "thor1xtd55mjchut4dm27t6utmapkckkx0l2sx0phrq", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 1600},
		{asset: "THOR.RUNE", fromAddr: "thor1y2kh2yggamf46amdpm3e9qz2mt5pugm4sq6uy9", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 14000000},
		{asset: "THOR.RUNE", fromAddr: "thor1yjawrz2dmhdyzz439gr5xtefsu6jm6n6h3mdaf", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 8000000},
		{asset: "THOR.RUNE", fromAddr: "thor1yq79qzu5k4mzlvcx7z3k90t8fxnqffx9c4msve", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 12000000},
		{asset: "THOR.RUNE", fromAddr: "thor1yyu52mkdtef2h632ydypnqnlpm4nuafqgu9mwv", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 6000000},
		{asset: "THOR.RUNE", fromAddr: "thor1z0cp2zhc8782ns3yn6t0n5rk9lff9s2mafnx59", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 4000000},
		{asset: "THOR.RUNE", fromAddr: "thor1z4upmr3mhaxhrepgdss44j5jxz373xn583l9gc", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 16000000},
		{asset: "THOR.RUNE", fromAddr: "thor1z53wwe7md6cewz9sqwqzn0aavpaun0gw0exn2r", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1z7kds2p8tftmeyevemnm8796q09f4zrekq5upk", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 2000000},
		{asset: "THOR.RUNE", fromAddr: "thor1zxdja5280ap9hwx929czll30znecpnzccyvnmh", toAddr: "MidgardBalanceCorrectionAddress", amountE8: 20000000},
	}
	for height, corrections := range heightCorrections {
		fn := func(meta *Metadata) {
			for _, c := range corrections {
				transfer := Transfer{
					FromAddr: []byte(c.fromAddr),
					ToAddr:   []byte(c.toAddr),
					Asset:    []byte(c.asset),
					AmountE8: c.amountE8,
				}
				Recorder.OnTransfer(&transfer, meta)
			}
		}
		AdditionalEvents.Add(height, fn)
	}
}

//////////////////////// Preregister Thornames

// The pre-registered thornames were loaded directly into the thornode KV in a store
// migration at V88 (height 5531995) and did not properly emit events. These are loaded
// from the configuration found in <thornode>/x/thorchain/preregister_thornames.json.

//go:embed preregister_thornames.json
var preregisterThornamesData []byte

func loadMainnetPreregisterThornames() {
	// unmarshal the configuration
	type preregisterThorname struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	thornames := []preregisterThorname{}
	err := json.Unmarshal(preregisterThornamesData, &thornames)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to unmarshal preregistered thornames")
	}

	// fake an event for each of the preregisterd thornames
	for _, tn := range thornames {
		tnc := tn // copy in scope
		AdditionalEvents.Add(5531995, func(meta *Metadata) {
			thorNameChange := THORNameChange{
				Name:         []byte(tnc.Name),
				Address:      []byte(tnc.Address),
				Owner:        []byte(tnc.Address),
				Chain:        []byte("THOR"),
				ExpireHeight: 10787995,
			}
			Recorder.OnTHORNameChange(&thorNameChange, meta)
		})
	}
}
