# BlockStore

## Summary

We need a simple format for a permanent storage of thornode blocks. It would be used for:

1. Speeding up Midgard sync and removing the corresponding load from Thornodes

2. Preserving historical blocks which can be used for syncing Midgard and debugging even after
   a Thornode fork that trims history.

### Goals

1. Plain files in a directory which can be easily rsynced, wgeted, sha256summed, etc.

2. Preferably, format that can be manipulated and processed with standard command line tools,
   like `wc`, `head`, `tail`, `jq`, etc.

3. Deterministic: dumps done by different users should be bit-for-bit equal.

4. Complete: should contain all of the info returned by the Thornode's `BlockResults` API call,
   not just the parts that are useful for Midgard. (So it can be used later for any purpose.)

5. Preferably a format that allows for (relatively) fast lookup of an individual block. (To be
   served by Midgard on the `/v2/debug/block/NNNNN` endpoint.)

Non-goals:

1. Format that allows for real-time querying and processing. This is only intended for archival and
   fast sequential reading purposes (and limited single-block lookup as described above.)

## Challenges

Before describing the proposed solutions we describe the biggest technical challenges we found:

### JSON is slow

JSON would be an obvious choice for the basis of the BlockStore format, since it's human readable,
can be processed by standard tools, and is the format that we get the data in from the Thornode
API to begin with.

Unfortunately, JSON parsing implementations in Go are very slow. One `BlockResults` from Chaosnet
at heights around 3'000'000 is a JSON object of average 22 kB size. Parsing it into a Go struct
takes around 4-5ms. We have tried the following JSON implementations, and none of them were
faster than 4 ms/block on the test machine:

- standard `encodings/json`
- `github.com/json-iterator/go`
- `github.com/segmentio/encoding/json`
- `github.com/goccy/go-json`
- `github.com/mailru/easyjson`

As a comparison, all of the binary serialization libraries that we've tried are at least an order
of magnitude faster (measured with a layer of `base64` encoding and `zsdt` compression):

- `encoding/gob`: 0.43 ms/block
- `github.com/fxamacker/cbor/v2`: 0.36 ms/block
- `github.com/gogo/protobuf/proto`: 0.16 ms/block

### Serializing/deserializing interface fields

One of the fields of the `chain.Block` data structure, in particular
`Results.ValidatorUpdates.PubKey.Sum` is of a (private) interface type in the
`github.com/tendermint/tendermint/proto/tendermint/crypto` package. This presents a problem for
serializing/deserializing it.

It can be natively handled with the tendermint's json package
`github.com/tendermint/tendermint/libs/json`, as it's aware of tendermint's types and uses
reflection to handle it properly. It can also be handled by the protobuf library; it's actually
defined as a `one_of` protobuf field, and Go's protobuf implementation translates it into an
interface type.

None of the other tested libraries can handle this field: different JSON implementation, Go's
native `gob` binary serialization, or `github.com/fxamacker/cbor/v2` CBOR implementation.

### Defining and compiling proto definitions is a hassle
