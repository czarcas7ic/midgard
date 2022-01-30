# Preliminary analysis queries
```
select * from (select name, hypertable_size(name) as size from (select schema_name||'.'||table_name as name from _timescaledb_catalog.hypertable) a) a order by size desc;
select * from (select pool, count(*) count from block_pool_depths group by pool) a order by count desc;
select pool,count,length(pool)*count originalBytes, 2*count newBytes from (select pool, count(*) count from block_pool_depths group by pool) a order by count desc;
```
# Dictionary encoding applied to the block_pool_depths table
```
create table pools(id smallserial not null, pool varchar(60) not null, primary key(id));
create unique index on pools(pool);
insert into pools(pool) (select distinct pool from block_pool_depths order by pool) on conflict do nothing;
create table block_pool_depths_2(pool_id smallint not null references pools(id), asset_e8 bigint not null, rune_e8 bigint not null, synth_e8 bigint not null, block_timestamp bigint not null);
call setup_hypertable('block_pool_depths_2');
CREATE INDEX ON block_pool_depths_2(pool_id, block_timestamp DESC);
insert into block_pool_depths_2(pool_id,block_timestamp,synth_e8,rune_e8,asset_e8) (select p.id, bpd.block_timestamp, bpd.synth_e8, bpd.rune_e8, bpd.asset_e8 from block_pool_depths bpd join pools p on p.pool = bpd.pool order by block_timestamp);
VACUUM;
```
# Size comparison
```
select 'block_pool_depths_original' table_name,* from hypertable_detailed_size('block_pool_depths') union select 'block_pool_depths_dictionary_compression' table_name, * from hypertable_detailed_size('block_pool_depths_2');
```
