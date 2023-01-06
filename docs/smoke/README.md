# Smoke Test Documentation

Unfortunately running the smoke test is long and flaky process.
If you get an error don't give up, retry.

The steps for the smoke tests are the following

## Create a directory inside which you clone 3 repositories: Midgard, Thornode and Heimdall.

In this step we create a directory where we put Thornode, Midgard and Heimdall next to each other.
This is needed for the `smoke-setup.sh` script. If you already have these direcoties somewhere and
want to use them then modify the steps accordingly in `smoke-setup.sh`.

If you create a dirrectory, let's call it `$SMOKEPATH`. For example:

```shell
mkdir smoke
cd smoke
SMOKEPATH=`pwd`

git clone git@gitlab.com:thorchain/thornode.git
git clone git@gitlab.com:thorchain/heimdall.git

# If you don't have it yet clone midgard and go to the branch you want
git clone git@gitlab.com:delphidigital/midgard.git
```

## If needed change ports

If later you have problems that Heimdall can't connect to Midgard (probably windows or mac),
then come back and add `midgard` flag to `$SMOKEPATH/heimdall/Makefile`
(smoke target)[https://gitlab.com/thorchain/heimdall/-/blob/950d4b1eda144966c6bb68418e8b48ca1cee4ff2/Makefile#L37]

```
smoke:
  @docker run ${DOCKER_OPTS} ${IMAGE_NAME} python scripts/smoke.py --midgard=http://host.docker.internal:8080 --fast-fail=True
```

## Modify the midgard config

It's in `midgard/config/config.json`

```json
{
  "listen_port": 8080,
  "thorchain": {
    "url": "http://localhost:26657/websocket",
    "node_url": "http://localhost:1317/thorchain",
    "last_chain_backoff": "7s"
  },
  "timescale": {
    "host": "localhost",
    "port": 5433,
    "user_name": "midgard",
    "password": "password",
    "database": "midgard",
    "sslmode": "disable"
  }
}
```

## Prepare ThorChain components for the smoke tests

Inspect `smoke-setup.sh`.
You can use it as is or run commands one by one, adapting it later to your needs.
The steps the script does:

1. Removes all your existing docker containers!
    You might want to modify this step and only remove containers which will be started by 2 and pgtest.
1. Starts ThorChain dev environment, that includes several container jobs.
1. Stops Midgard and Database default instances from the ThorChain dev environment.
1. Start pgtest database with local changes.
1. Starts Midgard with local changes.

```shell
cd $SMOKEPATH
cp midgard/docs/smoke/smoke-setup.sh .
./smoke-setup.sh
```

## Start the smoke test

Start the smoke test in a separate terminal.
It has many steps. If the last finishes without errors then it passed.
It's flaky, you probably want to retry.

If the test failed due to midgard, it will throw an error that says `Bad Midgard Pool`
([source](https://gitlab.com/thorchain/heimdall/-/blob/f5ed4ad91d9f211d6600ba588d41c28d55fe6f81/scripts/health.py#L131)).

```shell
cd $SMOKEPATH/heimdall
make smoke
```

## Retry the smoke test

In the two separate terminals. First restart the jobs:
```shell
# pwd == $SMOKEPATH
./smoke-setup.sh
```

Then start the smoke test again.

```shell
# pwd == $SMOKEPATH/heimdall
make smoke
```
