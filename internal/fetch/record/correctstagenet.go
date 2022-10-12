package record

func loadStagenetCorrections(rootChainID string) {
	switch rootChainID {
	case "thorchain-stagenet":
		// There was a case where the first stagenet churn resulted in a node getting churned
		// in that didn't have the minimum bond, so it had a status of "Active" with a
		// preflight status "Standby" and the `UpdateNodeAccountStatus` event was never sent.
		AdditionalEvents.Add(1, func(meta *Metadata) {
			updateNodeAccountStatus := UpdateNodeAccountStatus{
				NodeAddr: []byte("sthor1vzenszq5gh0rsnft55kwfgk3vzfme4pks8r0se"),
				Former:   empty,
				Current:  []byte("Active"),
			}
			Recorder.OnUpdateNodeAccountStatus(&updateNodeAccountStatus, meta)
		})

		// The TERRA.USD pool was renamed to TERRA.UST in a state migration. This creates
		// pool events and the corresponding liquidity add for the event that occurred
		// before the store migration.
		AdditionalEvents.Add(36631, func(meta *Metadata) {
			pool := Pool{
				Asset:  []byte("TERRA.UST"),
				Status: []byte("Staged"),
			}
			Recorder.OnPool(&pool, meta)
			deposit := Desposit{
				AddBase: AddBase{
					Pool:       []byte("TERRA.UST"),
					AssetTx:    []byte("5094157A89137CD762EDDC94E08016CB57D3717FF950D8CE227FDBD7A942479E"),
					AssetChain: []byte("TERRA"),
					AssetAddr:  []byte("terra1nrajxfwzc6s85h88vtwp9l4y3mnc5dx5uyas4u"),
					AssetE8:    13898654000,
					RuneTx:     []byte("7E43E29054F36854A74BDA8BFE8385E9ED85994FA8C30D394107DA25FA0F9A3C"),
					RuneChain:  []byte("THOR"),
					RuneAddr:   []byte("sthor19phfqh3ce3nnjhh0cssn433nydq9shx76s8qgg"),
					RuneE8:     3135000000,
				},
				StakeUnits: 3135000000,
			}
			Recorder.OnStake(&deposit, meta)
		})
		AdditionalEvents.Add(36720, func(meta *Metadata) {
			pool := Pool{
				Asset:  []byte("TERRA.USD"),
				Status: []byte("Suspended"),
			}
			Recorder.OnPool(&pool, meta)
			pool = Pool{
				Asset:  []byte("TERRA.UST"),
				Status: []byte("Available"),
			}
			Recorder.OnPool(&pool, meta)
		})

		AdditionalEvents.Add(627001, func(meta *Metadata) {
			// During the first fork of stagenet we manually modified pool balances in the
			// genesis file for the underlying vaults, pools, and LP positions, which became
			// inconsistent as the result of a exploit and subsequent manual KV store migrations
			// in thornode. This set of corrections makes the midgard state consistent with
			// thornode after the fork.
			withdraw := Withdraw{
				FromAddr:    []byte(""),
				Chain:       []byte("TERRA"),
				Pool:        []byte("TERRA.LUNA"),
				Asset:       []byte("THOR.RUNE"),
				ToAddr:      []byte(""),
				Memo:        []byte(""),
				Tx:          []byte(""),
				EmitRuneE8:  10423579354,
				EmitAssetE8: 492518419,
				StakeUnits:  0,
			}
			Recorder.OnWithdraw(&withdraw, meta)
			deposit := Desposit{
				AddBase: AddBase{
					Pool:       []byte("TERRA.LUNA"),
					AssetTx:    []byte(""),
					AssetChain: []byte("TERRA"),
					AssetAddr:  []byte("terra1nrajxfwzc6s85h88vtwp9l4y3mnc5dx5uyas4u"),
					AssetE8:    658291800,
					RuneTx:     []byte(""),
					RuneChain:  []byte("THOR"),
					RuneAddr:   []byte(""),
					RuneE8:     10423579354,
				},
				StakeUnits: 10423580154,
			}
			Recorder.OnStake(&deposit, meta)

			// Note that the liquidity providers for the UST pool are inconsistent with the
			// pool units - this is known and will be rectified on a subsequent stagenet fork.
			withdraw = Withdraw{
				FromAddr:    []byte(""),
				Chain:       []byte("TERRA"),
				Pool:        []byte("TERRA.UST"),
				Asset:       []byte("THOR.RUNE"),
				ToAddr:      []byte(""),
				Memo:        []byte(""),
				Tx:          []byte(""),
				EmitAssetE8: 927005400,
				EmitRuneE8:  722219743,
				StakeUnits:  0,
			}
			Recorder.OnWithdraw(&withdraw, meta)
		})
	}
}
