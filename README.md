[![pipeline status](https://gitlab.com/thorchain/midgard/badges/master/pipeline.svg)](https://gitlab.com/thorchain/midgard/commits/master)


# Midgard API

Midgard is a layer 2 REST API that provides front-end consumers with semi real-time rolled up data
and analytics of the THORChain network. Most requests to the network will come through Midgard. This
daemon is here to keep the chain itself from fielding large quantities of requests. You can think of
it as a “read-only slave” to the chain. This keeps the resources of the network focused on
processing transactions.


## Running Midgard

Midgard can be run locally with native code or via Docker Compose. Midgard populates the PSQL
database with content from the blockchain. Progress is traceable with the Prometheus Metrics
propagated on <http://localhost:8080/debug/metrics>, specifically the measurements
`midgard_chain_cursor_height` v.s. `midgard_chain_height`.
Open <http://localhost:8080/v2/doc> in your browser.

### Config

You can configure Midgard with a big config file, a list of smaller config files, or with
environment variables.

The easiest is composing multiple config files with `:`, later configs overwrite values from
earlier ones. A good starting point for config files is in `config/ex` directory. If you wish to
edit the config files you can `cp -r ./config/examples ./tmp/config` (gitignored) and edit them
there.

Second option: a full file is at `config/config.json` which assumes you are running a local ThorNode
and a PSQL. You can copy this file to `config/local.json` (which is ignored by `git`) and
make desired changes.
If you wish to connect to a specific ThorNode the proper urls can be found in
`config/ex/net-local.json` (localhost), `config/ex/net-main.json` (mainnet),
`config/ex/net-stage.json` (stagenet)

Third option: Overwrite single config values with environment variables

Fields in nested structs are accessed using underscores. Examples:
* `MIDGARD_LISTEN_PORT` env variable will override `Config.ListenPort` value
* `MIDGARD_TIMESCALE_PORT` will override `Config.TimeScale.Port` value
* `MIDGARD_USD_POOLS="A,B,C"` will override the UsdPools


### Start native Midgard

```sh
# One time setup:
docker-compose up -d pg
mkdir -p ./tmp/blockstore

# run midgard
go run ./cmd/midgard/ config/ex/base.json:config/ex/pg.json:config/ex/bs-m.json:config/ex/net-main-9r.json
```

### Docker Compose

Running with Docker Compose it's possible with a single config file at `config/local.json` or
environment variables.

To allow Midgard to connect properly to Postgres do `cp config/config.json config/local.json`
then edit `local.json` and change `timescale.host` to `"pg"`.

Then:

```sh
# One time setup:
docker-compose up -d pg

docker-compose up --build midgard
```

## Running Local ThorNode

To work on Midgard we don't need or want a proper validator setup, just the full thornode that
follows and syncs the thorchain locally.

Clone the thornode repo from: https://gitlab.com/thorchain/thornode

Look up the current version and check it out. If you need the latest verion you are probably fine
using the `chaosnet-multichain` branch.

Start the thornode by running `make run-fullnode` from `build/docker/mainnet`.

IMPORTANT! This will create a docker container named `thornode` and will store data in your home
directory, under `~/.thornode`. If you have anything important in one or the other, backup first!

To summarize:

```sh
git clone https://gitlab.com/thorchain/thornode.git
cd thornode
git checkout chaosnet-multichain
cd build/docker/mainnet
make run-fullnode
```

For midgard config use:

```json
    "tendermint_url": "http://localhost:26657/websocket",
    "thornode_url": "http://localhost:1317/thorchain",
```

### Upgrading local ThorNode

When the network switches to a newer version your local thornode will stop working:
the docker container will be in a crash loop. To upgrade, remove the container, the docker image,
pull, and restart:

```sh
docker stop thornode
docker rm thornode
docker rmi registry.gitlab.com/thorchain/thornode:mainnet
cd thornode/build/docker/mainnet
git pull
make run-fullnode
```

## Websockets

Websockets is an experimental feature supported for Linux only. If you need to use it for develop
using a different OS you may need to run Midgard using Docker.

## Testing

```bash
docker-compose up -d pgtest
go test -p 1 ./...
```

### External MR Pipeline Testing

To enable test pipelines on an external MR please consider to enable `Shared runners`

Repo `Settings` -> `CI/CD` -> `Runners` -> enable `Shared Runners`

## State Checks

A cmd that checks the state recreated by Midgard through events and the actual state stored
in the Thorchain can be run with:

```bash
go run ./cmd/statechecks config/ex/base.json:config/ex/pg.json:config/ex/bs-m.json:config/ex/net-main-9r.json:config/ex/loginfo.json
```

## Connecting to Midgard's PostgreSQL DB

To inspect Midgard's DB (run manual queries etc.) connect with `psql`. Install postgres client
tools; on Debian based systems:

```bash
sudo apt install postgres-client
```

And then:

```bash
psql -h localhost -U midgard midgard -p 5432
```

For test DB use port 5433; the `pg2` instance is on port 6432. The password is `password`. To
avoid entering it over and over again, do:

```bash
echo '*:*:midgard:*:password' >> ~/.pgpass && chmod 0600 ~/.pgpass
```

Alternatively, you can use the psql from within the appropriate Docker container (no need to
install postgres-client on your machine):

```bash
docker exec -it midgard_pg_1 psql -h localhost -U midgard midgard
```

## Trimming the database

Regenerating the database from height 1 can be time consuming. If there is a bug in a later point
it's possible to trim back all database tables to just before the problematic point. This is
useful to apply a bugfix quickly.

```bash
go run ./cmd/trimdb config/config.json HEIGHTORTIMESTAMP
```

## Saving & copying the database

If you'd like to do some (potentially destructive) experiments with the database, it's probably
a good idea to make a backup of it first, so you don't have to resync in case things don't go as
expected.

Provided that the directory where you checked out Midgard code is named `midgard` the standard
location of the `pg` database instance will be under `/var/lib/docker/volumes/midgard_pg/_data`.
But you can check this with `docker inspect` on the appropriate docker container. Like this:

```bash
docker inspect midgard_pg_1 | jq -r '.[].Mounts | .[].Source'
```

Consider treating unset parameters as an error when substituting.

```bash
set -u
```

Creating a backup of the `pg` instance:

```bash
# choose where to put the backup:
backup_dir=/tmp/pgbackup
# query the location of the docker volume:
pg_volume=/var/lib/docker/volumes/midgard_pg/_data

# stop, backup, restart:
docker stop midgard_pg_1
sudo cp -a $pg_volume/ $backup_dir/
docker start midgard_pg_1
```

Restoring the DB from the backup:

```bash
docker stop midgard_pg_1
sudo rsync -ac --del $backup_dir/ $pg_volume/
docker start midgard_pg_1
```

Of course, you can do this with the `pg2` or `pgtest` instances too.

### DB Compression

One option to save space is to use a storage volume with zfs file system configured with lz4 compression.
The example below gives roughly a compression ratio of 4:1.

```bash
# content of base folder will be replaced with a compressed zfs volume, save content for later use
sudo mv /var/lib/docker/volumes/midgard_pg/_data/base /tmp
sudo apt install zfsutils-linux
# check zfs installation
whereis zfs
# create empty 24G file, will be used as loopback device to store the filesystem
dd if=/dev/zero of=/tmp/zfs_image bs=24576 count=1048576
# create loopback device from file
sudo losetup -fP /tmp/zfs_image
# find assigned devicename
losetup -a
# create a storage pool named "pg"
sudo zpool create -o autoexpand=on pg /dev/loop12
sudo zpool set feature@lz4_compress=enabled pg
# create inside pg pool a storage volume accessible from the host via the mountpoint folder
sudo zfs create -o recordsize=32k -o redundant_metadata=most -o primarycache=metadata -o logbias=throughput pg/base -o mountpoint=/var/lib/docker/volumes/midgard_pg/_data/base
sudo zfs set compression=lz4 pg/base
# copy existing postgresql content back into the mountpoint, this will be compressed now
sudo cp -r /tmp/base /var/lib/docker/volumes/midgard_pg/_data
# check compression ratio
sudo zfs get all | grep compress
```

The zfs block size in the example is zfsBlockSize=32K, which is above the default postgresql block size, postgreSqlBlockSize=8K.
Such zfsBlockSize gives a better compression, but creates write amplification, since zfs is a copy on write filesystem
(each update to a block creates a new block copy+update, and the unit of copy is the zfsBlockSize).
Ideally the two block sizes, zfsBlockSize and postgreSqlBlockSize should match, but the postgresql block size can only be defined when creating a new postgresql compilation.
Additionally the limit on the postgreSqlBlockSize is 32K.

### Monitoring more than one chain

It is possible to rune more than one Midgard instance against different chains (e.g. main/testnet).
Create two config files (e.g. mainnet.json, testnet.json):
* set listen_port to 8080 and 8081
* edit thornode and tendermint urls
* set timescale/port to 5432 and 6432

```sh
docker-compose up -d pg
docker-compose up -d pg2
go run ./cmd/midgard tmp/mainnet.json
go run ./cmd/midgard tmp/testnet.json
```

Then you can check depths separately for them:

```bash
go run ./cmd/statechecks tmp/mainnet.json
go run ./cmd/statechecks tmp/testnet.json
```


## Generated files

Some GraphQL or OpenApi files are generated.

You need to install a few things once.
* For redoc-cli do `npm install` which takes dependencies form `package.json`
* For oapi-codegen do `go get github.com/deepmap/oapi-codegen/cmd/oapi-codegen`
  which will provide `$GOPATH/bin/oapi-codegen`

Then from now you can regenerate files with:

```bash
make generated
```

## Generating blockstore hashes

Midgard can read blockstore to speed up fetching from ThorNode. Blockstore consists of compressed
files containing the raw Bloks in batches of 10K.
These batches (chunks) are stored in a remote location. Midgard will download them on startup, but
it accepts only if the hashes of the chunks match the predefined values.

To regenerate the hashes and store them in git do these two steps:


Fetch all blocks from thornode to have them locally:

```bash
# Stop midgard first.
go run ./cmd/blockstore/dump config
```

Save the hashes in the git repository:

```
(cd $blockstore_folder; sha256sum *) > resources/hashes/$chain_id
```

## Format, Lint

You can run these before submit to make sure the CI will pass:
```
gofmt -l -s -w ./
docker run --rm -v $(pwd):/app -w /app golangci/golangci-lint golangci-lint run -v
```

Please, use an editor that respects the project's `.editorconfig` settings. For example, for
Visual Studio Code you can install the official EditorConfig extension:
https://marketplace.visualstudio.com/items?itemName=EditorConfig.EditorConfig

## Managing dependencies

To update a Go library dependency (because a vulnerability was discovered or we want to use a
feature from a newer version), do the following:

```
go get github.com/team/repo@version
go mod tidy
```

The `version` can be `latest` if you want to upgrade to the latest version.

### Adding a binary-only dependency

Sometimes we need a go library dependency not because we use it in the code, but because it provides
some binary which we use in the build process, for example. You add this kind of dependency the
same way as a normal dependency:

```
go get github.com/deepmap/oapi-codegen@latest
```

But, to prevent `go mod tidy` from removing this dependency it should be added to a `.go` file.
In Midgard we use `cmd/deps/deps.go` for this purpose.

## Architecture

The `chain` package reads the blockchain in chronological order.
Blocks are parsed with `events` and persisted with `internal/timeseries`.
The RDBM is almost a one-to-one mapping of the *key-value entries* from the THORChain.
Aggregated values and tables are created separately in `aggregate.go`.

Package `internal/api` defines the HTTP interface. See `internal/graphql` for the query
facilities (provided by `internal/timeseries/stat`).

Blocks are “committed” with an entry in the `block_log` table, including a `block_timestamp`.
Queries give consistent [cachable] results when executed with a (time) `db.Window` within
`timeseries.LastBlock`.

## Bookmarks

Direct links:
* Testnet seed location: https://testnet.seed.thorchain.info/
* Thorchain pools at height: https://testnet.thornode.thorchain.info/thorchain/pools?height=20000
* Thorchain single pool at height: https://testnet.thornode.thorchain.info/thorchain/pool/ETH.ETH?height=20000
* Tendermint block: http://&lt;tendermint&gt;:26657/block_results?height=1000
  This is base64 wrapped, to find the readable version use `cmd/fetchblock`.

Documentation:
* Connecting to Thorchain: https://docs.thorchain.org/developers/connecting-to-thorchain
* Tendermint doc: https://docs.tendermint.com/master/rpc/#/
* Midgard doc: https://testnet.midgard.thorchain.info/v2/doc
