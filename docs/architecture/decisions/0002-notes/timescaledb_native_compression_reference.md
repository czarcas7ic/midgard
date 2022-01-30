# Initial size analysis
```
SELECT *, total_bytes AS total
    , pg_size_pretty(index_bytes) AS index
    , pg_size_pretty(toast_bytes) AS toast
    , pg_size_pretty(table_bytes) AS table
  FROM (
  SELECT *, total_bytes-index_bytes-coalesce(toast_bytes,0) AS table_bytes FROM (
      SELECT c.oid,nspname AS table_schema, relname AS table_name
              , c.reltuples AS row_estimate
              , pg_total_relation_size(c.oid) AS total_bytes
              , pg_indexes_size(c.oid) AS index_bytes
              , pg_total_relation_size(reltoastrelid) AS toast_bytes
          FROM pg_class c
          LEFT JOIN pg_namespace n ON n.oid = c.relnamespace
          WHERE relkind = 'r'
  ) a
) a order by total desc;
select * from (select name, hypertable_size(name) as size from (select schema_name||'.'||table_name as name from _timescaledb_catalog.hypertable) a) a order by size desc;
```
# Manual compression
```
SELECT add_compression_policy('block_pool_depths');
select compress_chunk(i) from 
select decompress_chunk(j.chunk_name) from (select i.chunk_schema||'.'||i.chunk_name chunk_name from (select * from chunk_compression_stats('block_pool_depths') where compression_status = 'Compressed') i) j;
```
# Compression analysis
```
docker exec midgard_pg_1 psql -U midgard -c "select sum(t.after_compression_total_bytes)/sum(t.before_compression_total_bytes) compression_ratio from (SELECT * FROM chunk_compression_stats('block_pool_depths')) t;" 
explain analyze select * from block_pool_depths where block_timestamp = 1623613231867172924;
select hypertable_id, attname, compression_algorithm_id , al.name
from _timescaledb_catalog.hypertable_compression hc,
     _timescaledb_catalog.hypertable ht,
      _timescaledb_catalog.compression_algorithm al
where ht.id = hc.hypertable_id and ht.table_name like 'conditions' and al.id = hc.compression_algorithm_id
ORDER BY hypertable_id, attname;
```
