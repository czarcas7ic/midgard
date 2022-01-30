➜  midgard git:(develop) ✗ go run ./cmd/loadtest_all/loadtest_all.go config/9R.json 
2022-01-14T17:11:36+01:00 INF Starting midgard_url=http://localhost:8080
2022-01-14T17:12:34+01:00 DBG Fetch failed error="Get \"http://localhost:8080/v2/history/swaps\": EOF" time_ms=57305 url=http://localhost:8080/v2/history/swaps
2022-01-14T17:12:34+01:00 INF . error=unhealthy endpoint=/v2/history/swaps params=
2022-01-14T17:12:38+01:00 INF . endpoint=/v2/history/swaps params=interval=day&count=100 s_avg=1.609 s_max=4.397 s_median=0.216
2022-01-14T17:12:41+01:00 INF . endpoint=/v2/history/swaps params=pool=BNB.BNB s_avg=0.763 s_max=1.666 s_median=0.317
2022-01-14T17:12:41+01:00 INF . endpoint=/v2/history/swaps params=pool=BNB.BNB&interval=day&count=100 s_avg=0.258 s_max=0.359 s_median=0.209
2022-01-14T17:13:58+01:00 INF . error="too slow" endpoint=/v2/history/earnings params= s=77.04
2022-01-14T17:14:02+01:00 INF . endpoint=/v2/history/earnings params= s_avg=26.913 s_max=77.04 s_median=1.866
2022-01-14T17:14:05+01:00 INF . endpoint=/v2/history/earnings params=interval=day&count=100 s_avg=0.792 s_max=1.288 s_median=0.569
2022-01-14T17:14:05+01:00 INF . endpoint=/v2/history/liquidity_changes params= s_avg=0.276 s_max=0.282 s_median=0.276
2022-01-14T17:14:06+01:00 INF . endpoint=/v2/history/liquidity_changes params=interval=day&count=100 s_avg=0.252 s_max=0.258 s_median=0.25
2022-01-14T17:14:07+01:00 INF . endpoint=/v2/history/liquidity_changes params=pool=BNB.BNB s_avg=0.38 s_max=0.615 s_median=0.268
2022-01-14T17:14:08+01:00 INF . endpoint=/v2/history/liquidity_changes params=pool=BNB.BNB&interval=day&count=100 s_avg=0.254 s_max=0.275 s_median=0.273
2022-01-14T17:14:21+01:00 INF . error="too slow" endpoint=/v2/history/tvl params= s=13.406
2022-01-14T17:14:24+01:00 INF . endpoint=/v2/history/tvl params= s_avg=5.275 s_max=13.406 s_median=1.242
2022-01-14T17:14:27+01:00 INF . endpoint=/v2/history/tvl params=interval=day&count=100 s_avg=1.093 s_max=1.722 s_median=0.801
2022-01-14T17:14:30+01:00 INF . endpoint=/v2/actions params= s_avg=0.979 s_max=2.692 s_median=0.124
2022-01-14T17:14:30+01:00 INF . endpoint=/v2/actions params=offset=1000&limit=50 s_avg=0.124 s_max=0.127 s_median=0.123
2022-01-14T17:14:31+01:00 INF . endpoint=/v2/actions params=address=someaddr s_avg=0.007 s_max=0.019 s_median=0.002
2022-01-14T17:14:31+01:00 INF . endpoint=/v2/actions params=address=someaddr&offset=1000&limit=50 s_avg=0.001 s_max=0.002 s_median=0.001
2022-01-14T17:14:36+01:00 INF . endpoint=/v2/pools params= s_avg=1.69 s_max=4.271 s_median=0.419
2022-01-14T17:14:39+01:00 INF . endpoint=/v2/pool/BNB.BNB/stats params= s_avg=1.212 s_max=2.186 s_median=0.732
2022-01-14T17:14:40+01:00 INF . endpoint=/v2/members params= s_avg=0.188 s_max=0.204 s_median=0.182
2022-01-14T17:14:50+01:00 DBG Fetch failed error="status: 502 Bad Gateway" time_ms=10160 url=http://localhost:8080/v2/thorchain/inbound_addresses
2022-01-14T17:14:50+01:00 INF . error=unhealthy endpoint=/v2/thorchain/inbound_addresses params=
2022-01-14T17:14:50+01:00 DBG Fetch failed error="status: 404 Not Found" time_ms=0 url=http://localhost:8080/bad
2022-01-14T17:14:50+01:00 INF . error=unhealthy endpoint=/bad params=

