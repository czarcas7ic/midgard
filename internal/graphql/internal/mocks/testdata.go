package mocks

import (
	"time"

	"gitlab.com/thorchain/midgard/internal/graphql/model"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
)

func i64(n int64) *int64 {
	return &n
}

var first, _ = time.Parse(time.RFC3339, "2020-09-21 21:05:48.968821065 +0900 JST")
var last, _ = time.Parse(time.RFC3339, "2020-09-21 22:28:45.352308164 +0900 JST")

var TestData = Data{
	Pools: []Pool{
		Pool{
			Asset:  "TEST.COIN",
			Status: "Enabled",
			Ae8pp:  3949032195733,
			Re8pp:  135631226606311,
			Price:  34.34543449730872,

			StakeTxCount:         1236,
			StakeAssetE8Total:    5002997091788,
			StakeRuneE8Total:     341573223004012,
			StakeStakeUnitsTotal: 133851876377593,
			StakeFirst:           "2020-08-26 17:52:47.685651618 +0900 JST",
			StakeLast:            "2020-09-15 19:26:32.905629669 +0900 JST",

			UnstakeTxCount:          274,
			UnstakeAssetE8Total:     190,
			UnstakeRuneE8Total:      220000082,
			UnstakeStakeUnitsTotal:  67286385293693,
			UnstakeBasisPointsTotal: 2150910,

			SwapsFromRuneBucket: []stat.PoolSwaps{
				stat.PoolSwaps{
					First:               first,
					Last:                last,
					TxCount:             94,
					AssetE8Total:        0,
					RuneE8Total:         14286622385430,
					LiqFeeE8Total:       369368251,
					LiqFeeInRuneE8Total: 20500035455,
					TradeSlipBPTotal:    1741,
				},
				stat.PoolSwaps{
					First:               first,
					Last:                last,
					TxCount:             112,
					AssetE8Total:        0,
					RuneE8Total:         23245220000000,
					LiqFeeE8Total:       665305942,
					LiqFeeInRuneE8Total: 41429706330,
					TradeSlipBPTotal:    2696,
				},
			},
			SwapsToRuneBucket: []stat.PoolSwaps{
				stat.PoolSwaps{
					First:               first,
					Last:                last,
					TxCount:             52,
					AssetE8Total:        131277554216,
					RuneE8Total:         0,
					LiqFeeE8Total:       10096073016,
					LiqFeeInRuneE8Total: 10096073016,
					TradeSlipBPTotal:    893,
				},
				stat.PoolSwaps{
					First:               first,
					Last:                last,
					TxCount:             78,
					AssetE8Total:        160846334530,
					RuneE8Total:         0,
					LiqFeeE8Total:       11983916834,
					LiqFeeInRuneE8Total: 11983916834,
					TradeSlipBPTotal:    1144,
				},
			},
			StakeHistory: []stat.PoolStakes{
				stat.PoolStakes{
					TxCount:         38,
					AssetE8Total:    92761734194,
					RuneE8Total:     6276432493131,
					StakeUnitsTotal: 2310888431619,
					First:           first,
					Last:            last,
				},
			},

			Expected: ExpectedResponse{
				Pool: model.Pool{
					Asset:  "TEST.COIN",
					Status: "Enabled",
					Price:  34.34543449730872,
					Units:  66565491083900,
					Depth: &model.PoolDepth{
						AssetDepth: 3949032195733,
						RuneDepth:  135631226606311,
						PoolDepth:  271262453212622,
					},
					Stakes: &model.PoolStakes{
						AssetStaked: 5002997091788,
						RuneStaked:  341573003003930,
						PoolStaked:  513403111903635,
					},
					Roi: &model.Roi{
						AssetRoi: -0.2106667016926757,
						RuneRoi:  -0.6029217022027046,
					},
				},

				SwapHistory: model.PoolSwapHistory{
					Meta: &model.PoolSwapHistoryBucket{
						First: i64(1600683392),
						Last:  i64(1600683392),
						ToRune: &model.SwapStats{
							Count:        i64(52),
							FeesInRune:   i64(10096073016),
							VolumeInRune: i64(0),
						},
						ToAsset: &model.SwapStats{
							Count:        i64(94),
							FeesInRune:   i64(20500035455),
							VolumeInRune: i64(14286622385430),
						},
						Combined: &model.SwapStats{
							Count:        i64(336),
							FeesInRune:   i64(84009731635),
							VolumeInRune: i64(37531842385430),
						},
					},
					Intervals: []*model.PoolSwapHistoryBucket{
						&model.PoolSwapHistoryBucket{
							First: i64(1600683392),
							Last:  i64(1600683392),
							ToRune: &model.SwapStats{
								Count:        i64(52),
								FeesInRune:   i64(10096073016),
								VolumeInRune: i64(0),
							},
							ToAsset: &model.SwapStats{
								Count:        i64(94),
								FeesInRune:   i64(20500035455),
								VolumeInRune: i64(14286622385430),
							},
							Combined: &model.SwapStats{
								Count:        i64(146),
								FeesInRune:   i64(30596108471),
								VolumeInRune: i64(14286622385430),
							},
						},
						&model.PoolSwapHistoryBucket{
							First: i64(1600683740),
							Last:  i64(1600683740),
							ToRune: &model.SwapStats{
								Count:        i64(78),
								FeesInRune:   i64(11983916834),
								VolumeInRune: i64(0),
							},
							ToAsset: &model.SwapStats{
								Count:        i64(112),
								FeesInRune:   i64(41429706330),
								VolumeInRune: i64(23245220000000),
							},
							Combined: &model.SwapStats{
								Count:        i64(190),
								FeesInRune:   i64(53413623164),
								VolumeInRune: i64(23245220000000),
							},
						},
					},
				},
				StakeHistory: model.PoolStakeHistory{
					Intervals: []*model.PoolStakeHistoryBucket{
						&model.PoolStakeHistoryBucket{
							First:         i64(first.Unix()),
							Last:          i64(last.Unix()),
							Count:         i64(38),
							VolumeInRune:  i64(6276432493131),
							VolumeInAsset: i64(92761734194),
							Units:         i64(2310888431619),
						},
					},
				},
			},
		},
	},
	Timestamp: "2020-09-08 20:01:39.74967453 +0900 JST",
}