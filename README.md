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
* `MIDGARD_POOLS_DECIMAL="A.A:8,B.B:18"` will override the pools decimals

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

You can find an example Docker Compose configuration for running a full node for ThorChain mainnet
or testnet locally in the `docs/fullnode` directory.

The image versions in that example config might be out of date, so check first on the
`#thornode-mainnet` channel on Discord (or `#thornode-testnet` for testnet) and update it
accordingly. Then

```sh
cd docs/fullnode
docker-compose up -d mainnet
```

For testnet start the `testnet` service, of course. Note, the API of the testnet fullnode will
be on port 1318, instead of the usual 1317.

For Midgard config use `config/ex/net-main-local.json` or `config/ex/net-test-local.json`
correspondingly.

Syncing up a mainnet thornode take a really long time, you might want to use a NineRealms snapshot
to speed the process up. Use the ideas from the
https://gitlab.com/thorchain/devops/node-launcher/-/blob/master/scripts/recover-ninerealms.sh
script. (There is a `recovery` service defined in the above `docker-compose.yml` file to help
you get started.)

### Upgrading local ThorNode

When the network switches to a newer version your local thornode will stop working:
the docker container will be in a crash loop. To upgrade, update the image in
`docs/fullnode/docker-compose.yml` to the new version and restart with `docker-compose up -d ...`

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

## Monitoring more than one chain

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

Some OpenApi files are generated.

You need to install a few things once.
* For redoc-cli do `npm install` which takes dependencies form `package.json`
* For oapi-codegen do `go get github.com/deepmap/oapi-codegen/cmd/oapi-codegen`
  which will provide `$GOPATH/bin/oapi-codegen`

Then from now you can regenerate files with:

```bash
make generated
```

## Generate blockstore hashes

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

## Refresh native decimals

Midgard is a source of truth for the native decimal values of different coins. To regenerate the
constant table run:

```
go run ./cmd/decimal
```

Then commit the new generated file
(`internal/decimal/decimals.json`).

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

Package `internal/api` defines the HTTP interface.

Blocks are “committed” with an entry in the `block_log` table, including a `block_timestamp`.
Queries give consistent [cachable] results when executed with a (time) `db.Window` within
`timeseries.Latest`.

## Cutting a release


- You'll need to do an MR with 1-3 commits, make sure you refresh develop first
- Optional commit: [Generate blockstore hashes](#generate-blockstore-hashes)
- Optional commit: [Refresh native decimals](#refresh-native-decimals)
- Check the commits since the last release and write the release notes with the list of changes.
  * It doesn't have to be very detailed, it should list the important changes for clients.
  * Also note the commits which might be quickly testable as a last check
    (e.g. new endpoint, changes in a specific field of an endpoint, etc.)
  * Do note if there were DDL changes which will cause DB resync
- Start up Midgard with mainnet and do a quick check that everything looks sane
- If there were changes to endpoints/fields do eyeball that results seems sane,
  no 404, no 0 values, etc.
- Bump the version in a commit:
  * Edit openapi.yaml, bump the version by semantic versioning rules:
    + increase the patch number (last number) if there were no relevant changes for the clients, only bugfixes, speed upgrades, ...
    + increase the minor version (middle number) if there is a API change (e.g. new field)
  * On shell do: `make` which will regenerate oapigen go/html
  * If you have problems with openapi generation check [Generated files](#generated-files)
  * The commit message should be "Bump version to 2.x.x"
- Create an MR. If people are expecting that you'll do a release then merge right away,
  you don't need to wait for additional approval.
  [Example MR](https://gitlab.com/thorchain/midgard/-/merge_requests/451)
- You don't necesarrily need to wait until smoke test is passing, but the image will not build
  until it passes, you might need to sync with other teams to get it passing.
- Once merged create the release (example https://gitlab.com/thorchain/midgard/-/releases/2.9.6 ):
  * In gitlab click: `Deployments > Releases > New release` .
  * Tagname: type in the version number (`2.x.x`)  and click create tag
  * Create from: develop
  * Release title: append a `v` to the version number (`v2.x.x`)
  * Milestone: leave empty
  * Release date: leave default
  * Release notes: copy the release notes you gathered when looking through the commits
- Announce on Midgard Discord channel the release, tag ursa, copy the release notes there too.
  [Example](https://discord.com/channels/838986635756044328/839002340354424842/1033018380728406026)

## Bookmarks

Direct links:
* Testnet seed location: https://testnet.seed.thorchain.info/
* Thorchain pools at height: https://testnet.thornode.thorchain.info/thorchain/pools?height=20000
* Thorchain single pool at height: https://testnet.thornode.thorchain.info/thorchain/pool/ETH.ETH?height=20000
* Tendermint block: http://&lt;tendermint&gt;:26657/block_results?height=1000
  This is base64 wrapped, to find the readable version use `cmd/fetchblock`.

Documentation:
* Thorchain Docs: https://docs.thorchain.org/
* Tendermint doc: https://docs.tendermint.com/master/rpc/#/
* Midgard doc: https://testnet.midgard.thorchain.info/v2/doc
