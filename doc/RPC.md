RPC
===

Sia peers communicate with each other via Remote Procedure Calls. An RPC consists of a unique ID and a pair of functions, one on the calling end and one on the receiving end. After the ID is written/received, both peers hand the connection off to their respective functions. Typically, the calling end writes an object to the receiver and then reads a response.

RPC IDs are always 8 bytes and contain a human-readable name for the RPC. If the name is shorter than 8 bytes, the remainder is padded with zeros. If the name is longer than 8 bytes, it is truncated.

### Call Listing

Unless otherwise specified, these calls follow a request/response pattern and use the [encoding](./Encoding.md) package to serialize data.

**All data received via RPC should be considered untrusted and potentially malicious.**

#### ShareNodes

ShareNodes requests peer addresses from a peer.
The gateway calls this RPC regularly to update its list of potential peers.

ID: `"ShareNod"`

Request: None

Response:

```go
[]modules.NetAddress
```

Recommendations:

+ Requesting peers should limit the request to 3000 bytes.
+ Responding peers should send no more than 10 peers, and should not send peers that are unlikely to be reachable.

#### SendBlocks

SendBlocks requests blocks from a peer. The blocks are added to the requesting peer's blockchain, and optionally rebroadcast to other peers. Unlike most RPCs, the SendBlocks call is a loop of requests and responses that continues until the responding peer has no more blocks to send.

ID: `"SendBloc"`

Request:

```go
// Exponentially-spaced IDs of most-recently-seen blocks,
// ordered from most recent to least recent.
// Less than 32 elements may be present, but the last element
// (index 31) is always the ID of the genesis block.
[32]types.BlockID
```

Response:

```go
struct {
   // sequential list of blocks, beginning with the first
   // block in the main chain not seen by the requesting peer.
   blocks []types.Block
   // true if the responding peer can send more blocks
   more bool
}
```

Recommendations:

+ Requesting peers should limit the request to 20MB.
+ Responding peers should identify the most recent BlockID that is in their blockchain, and send up to 10 blocks following that block.
+ Responding peers should set `more = true` if they have not sent the most recent block in their chain.

#### RelayHeader

RelayHeader sends a block header ID to a peer, with the expectation that the peer will relay the ID to its own peers.

ID: `"RelayHea"`

Request:

```go
types.BlockHeader
```

Response: None

Recommendation:

+ Requesting (sending) peers should call this RPC on all of their peers as soon as they mine or receive a block via `SendBlocks` or `SendBlk`.
+ Responding (receiving) peers should use the `SendBlk` RPC to download the actual block content. If the block is an orphan, `SendBlocks` should be used to discover the block's parent(s).
+ Responding peers should not rebroadcast the received ID until they have downloaded and verified the actual block.

#### SendBlk

SendBlk requests a block's contents from a peer, given the block's ID.

ID: `"SendBlk\0"`

Request:

```go
types.BlockID
```

Response:

```go
types.Block
```

+ Requesting peers should limit the received block to 2 MB (the maximum block size).
+ Requesting peers should broadcast the block's ID using `RelayHeader` once the received block has been verified.
+ Responding peers may simply close the connection if the block ID does not match a known block.

#### RelayTransactionSet

RelayTransactionSet sends a transaction set to a peer.

ID: `"RelayTra"`

Request:

```go
[]types.Transaction
```

Response: None

Recommendations:

+ Requesting peers should limit the request to 2 MB (the maximum block size).
+ Responding peers should broadcast the received transaction set once it has been verified.
