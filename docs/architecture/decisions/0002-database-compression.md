# 2. Database compression

Date: 2022-01-30

## Status

Accepted

## Context
As of today (end of January 2022) midgard's database grew to a substantial size since the productive launch (April 2021). The postgresql database volume has reached ~60 GByte in size.
Midgard has to be prepared for accelerating data growth, since greater activity is expected on the DEX with the introduction of new coins and tokens in the forthcomming years.

We have investigated various techniques to lessen midgard's storage impact by trading off features and performance for reduced space:
- [ZFS and LZ4 compression](#zfs-and-lz4-compression)
- [TimescaleDB native compression](#timescaledb-native-compression)
- [Dictionary compression](#dictionary-compression)

### ZFS and LZ4 compression
One option to save space is to use a storage volume with zfs file system configured with lz4compression. This way each storage block is compressed with the lz4 algorithm.

**Pros**
- transparent to the overlying DB system
- relatively intact read and write performance

**Cons**
- compression ratio potential is capped because of the small file system block size (roughly a compression ratio of 4:1)
- due to it's special non GPL licensing, it is no part of the mainline linux kernel
- cloud environments might not support zfs
- configuration management is more difficult, since storage volume configuration is outside of the scope for docker-compose

### TimescaleDB native compression
TimescaleDB has a very efficient and advanced compression mechanism, converting row based storage to columnar storage along a select category column. It automatically detects and applies the adequate compression algorithm (run length encoding, delta-of-delta encoding on timestamps, dictionary compression for low cardinality text, xor-encoding for floats, LZ array compression)

**Pros**
- excellent compression ratio, during our tests the `block_pool_depths` table was compressed down to 3%
- easy to set up
- compression can be scheduled on timescaledb chunk tables

**Cons**
- has significant performance impact, measured with the load_test_all.go the various response times took a 10x hit, as seen below:

| Endpoint | Before | After |
| -        | -      | -     |
| endpoint=/v2/history/swaps params=interval=day&count=100 | s_avg=1.609 s_max=4.397 s_median=0.216 | s_avg=4.062 s_max=5.03 s_median=3.589 |
| endpoint=/v2/history/swaps params=pool=BNB.BNB | s_avg=0.763 s_max=1.666 s_median=0.317 | s_avg=5.922 s_max=6.768 s_median=5.508 |
| endpoint=/v2/history/swaps params=pool=BNB.BNB&interval=day&count=100 | s_avg=0.258 s_max=0.359 s_median=0.209 | s_avg=3.597 s_max=3.643 s_median=3.575 |
| endpoint=/v2/history/earnings | s=77.04 | s=69.6 |
| endpoint=/v2/history/earnings | s_avg=26.913 s_max=77.04 s_median=1.866 | s_avg=27.885 s_max=69.6 s_median=7.058 |
| endpoint=/v2/history/earnings params=interval=day&count=100 | s_avg=0.792 s_max=1.288 s_median=0.569 | s_avg=4.656 s_max=5.652 s_median=4.345 |
| endpoint=/v2/history/liquidity_changes | s_avg=0.276 s_max=0.282 s_median=0.276 | s_avg=5.66 s_max=5.799 s_median=5.657 |
| endpoint=/v2/history/liquidity_changes params=interval=day&count=100 | s_avg=0.252 s_max=0.258 s_median=0.25 | s_avg=3.616 s_max=3.617 s_median=3.616 |
| endpoint=/v2/history/liquidity_changes  params=pool=BNB.BNB | s_avg=0.38 s_max=0.615 s_median=0.268 | s_avg=5.572 s_max=5.7 s_median=5.516 |
| endpoint=/v2/history/liquidity_changes   params=pool=BNB.BNB&interval=day&count=100 | s_avg=0.254 s_max=0.275 s_median=0.273 | s_avg=3.718 s_max=3.857 s_median=3.691 |

For further endpoints see: [load_test_before_compression](0002-notes/load_test_before_compression.md), [load_test_after_compression](0002-notes/load_test_after_compression.md)

- indexes are ignored, 
- no delete support (not relevant for midgard)
- out of order data should be avoided (not relevant for midgard)
- update is not automatic, needs manual de and recompression (not relevant for midgard)

For further reference see: [timescaledb_native_compression_reference](0002-notes/timescaledb_native_compression_reference.md)

### Dictionary compression
As of the 10th of January 2022, 75% of the data is concentrated on the following tables
| Table | Size |
| ----  | ---- |
|midgard.block_pool_depths                         | 16715743232|
|midgard.transfer_events                           | 12850782208|
|midgard.message_events                            | 10962427904|
|midgard.rewards_event_entries                     |  7890305024|

Since the address columns of the various tables are dictionary compressable, we explored this opportunity.
During our tests, the compression ratio (considering indexes) was only 50%
(see [dictionary_compression_reference](0002-notes/dictionary_compression_reference.md)).

### DB cleanup
Since the `midgard.transfer_events` and `midgard.message_events` tables are not yet used for any feature and their size is significant, for short term, switching off the recording of these events gives the most benefit.

## Decision

TODO 

## Consequences

TODO