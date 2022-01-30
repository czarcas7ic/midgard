➜  midgard git:(develop) ✗ go run ./cmd/loadtest_all/loadtest_all.go config/9R.json
2022-01-14T18:02:10+01:00 INF Starting midgard_url=http://localhost:8080
2022-01-14T18:02:22+01:00 INF . error="too slow" endpoint=/v2/history/swaps params= s=12.117
2022-01-14T18:02:34+01:00 INF . endpoint=/v2/history/swaps params= s_avg=8.136 s_max=12.117 s_median=6.149
2022-01-14T18:02:46+01:00 INF . endpoint=/v2/history/swaps params=interval=day&count=100 s_avg=4.062 s_max=5.03 s_median=3.589
2022-01-14T18:03:04+01:00 INF . endpoint=/v2/history/swaps params=pool=BNB.BNB s_avg=5.922 s_max=6.768 s_median=5.508
2022-01-14T18:03:15+01:00 INF . endpoint=/v2/history/swaps params=pool=BNB.BNB&interval=day&count=100 s_avg=3.597 s_max=3.643 s_median=3.575
2022-01-14T18:04:24+01:00 INF . error="too slow" endpoint=/v2/history/earnings params= s=69.6
2022-01-14T18:04:39+01:00 INF . endpoint=/v2/history/earnings params= s_avg=27.885 s_max=69.6 s_median=7.058
2022-01-14T18:04:53+01:00 INF . endpoint=/v2/history/earnings params=interval=day&count=100 s_avg=4.656 s_max=5.652 s_median=4.345
2022-01-14T18:05:09+01:00 INF . endpoint=/v2/history/liquidity_changes params= s_avg=5.66 s_max=5.799 s_median=5.657
2022-01-14T18:05:20+01:00 INF . endpoint=/v2/history/liquidity_changes params=interval=day&count=100 s_avg=3.616 s_max=3.617 s_median=3.616
2022-01-14T18:05:37+01:00 INF . endpoint=/v2/history/liquidity_changes params=pool=BNB.BNB s_avg=5.572 s_max=5.7 s_median=5.516
2022-01-14T18:05:48+01:00 INF . endpoint=/v2/history/liquidity_changes params=pool=BNB.BNB&interval=day&count=100 s_avg=3.718 s_max=3.857 s_median=3.691
2022-01-14T18:06:05+01:00 INF . error="too slow" endpoint=/v2/history/tvl params= s=16.307
2022-01-14T18:06:18+01:00 INF . endpoint=/v2/history/tvl params= s_avg=9.837 s_max=16.307 s_median=6.803
2022-01-14T18:06:31+01:00 INF . endpoint=/v2/history/tvl params=interval=day&count=100 s_avg=4.376 s_max=4.925 s_median=4.143
2022-01-14T18:06:33+01:00 INF . endpoint=/v2/actions params= s_avg=0.789 s_max=2.122 s_median=0.129
2022-01-14T18:06:34+01:00 INF . endpoint=/v2/actions params=offset=1000&limit=50 s_avg=0.353 s_max=0.829 s_median=0.119
2022-01-14T18:06:34+01:00 INF . endpoint=/v2/actions params=address=someaddr s_avg=0.04 s_max=0.118 s_median=0.002
2022-01-14T18:06:34+01:00 INF . endpoint=/v2/actions params=address=someaddr&offset=1000&limit=50 s_avg=0.001 s_max=0.001 s_median=0.001
2022-01-14T18:06:38+01:00 INF . endpoint=/v2/pools params= s_avg=1.031 s_max=2.28 s_median=0.439
2022-01-14T18:06:50+01:00 INF . error="too slow" endpoint=/v2/pool/BNB.BNB/stats params= s=12.637
2022-01-14T18:07:01+01:00 INF . error="too slow" endpoint=/v2/pool/BNB.BNB/stats params= s=11.237
2022-01-14T18:07:13+01:00 INF . error="too slow" endpoint=/v2/pool/BNB.BNB/stats params= s=11.261
2022-01-14T18:07:13+01:00 INF . endpoint=/v2/pool/BNB.BNB/stats params= s_avg=11.711 s_max=12.637 s_median=11.261
2022-01-14T18:07:13+01:00 INF . endpoint=/v2/members params= s_avg=0.184 s_max=0.193 s_median=0.187
2022-01-14T18:07:19+01:00 INF . endpoint=/v2/thorchain/inbound_addresses params= s_avg=2.027 s_max=5.673 s_median=0.208
2022-01-14T18:07:19+01:00 DBG Fetch failed error="status: 404 Not Found" time_ms=0 url=http://localhost:8080/bad
2022-01-14T18:07:19+01:00 INF . error=unhealthy endpoint=/bad params=

