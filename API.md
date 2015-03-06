Siad API
========

All API calls return JSON objects. If there is an error, the error is returned
in plaintext. The standard response is { "Success": true}. In this document,
the API responses are defined as go structs. The go structs will get encoded to
JSON before being sent. Go structs are used here to provide type information.

At version 0.4, the API will be locked into forwards compatibility. This means
that we will not remove elements from the responses (but we may add additional
elements), and we will not add any required parameters (though we may add
optional parameters, and we may disable parameters).

Daemon
------

Queries:

* /daemon/stop
* /daemon/update/check
* /daemon/update/apply

#### /daemon/stop

Function: Cleanly shuts down the daemon. May take a while.

Parameters: none

Response: standard

#### /daemon/update/check:

Function: Checks for an update, returning a bool indicating whether
there is an update and a version indicating the version of the update.

Parameters: none

Response:
```
struct {
	Available bool
	Version   string
}
```

#### /daemon/update/apply:

Function: Applies any updates that are available.

Parameters: none

Response: standard

Consensus
---------

Queries:

* /consensus/status

#### /consensus/status

Function: Returns information about the consensus set, such as the current
block height.

Parameters: none

Response:
```
struct {
	Height       int
	CurrentBlock string
	Target       string
}
```

Gateway
-------

Queries:

* /gateway/status
* /gateway/synchronize
* /gateway/peer/add
* /gateway/peer/remove

#### /gateway/status

Function: Returns information about the gateway, including the list of peers.

Parameters: none

Response:
```
struct {
	Peers []string
}
```

#### /gateway/synchronize

Function: Will synchronize the daemon + consensus to the rest of the network.
Effects may take a few minutes to set in. Should only be necessary for
debugging.

Parameters: none

Reponse: none

#### /gateway/peer/add

Function: Will add a peer to the gateway.

Parameters:
```
struct {
	Address string
}
```
Address should be an IP address of the form: "a.b.c.d:port"

Response: standard

#### /gateway/peer/remove

Function: Will remove a peer from the gateway.

Parameters:
```
struct {
	Address string
}
```
Address should be an IP address of the form: "a.b.c.d:port"

Response: standard

-----

| Path              | Parameters                           | Response                     |
|:------------------|:---------------------------------|:-----------------------------|
| /host/config      |                                  | See HostInfo                 |
| /host/setconfig   | See HostSettings                 |                              |
| /miner/start      | `threads`                        |                              |
| /miner/status     |                                  | See MinerInfo                |
| /miner/stop       |                                  |                              |
| /wallet/address   |                                  | `{ "Address" }`              |
| /wallet/send      | `amount`, `dest`                 |                              |
| /wallet/status    |                                  | See WalletInfo               |
| /file/upload      | `file`, `nickname`, `pieces`     |                              |
| /file/uploadpath  | `filename`, `nickname`, `pieces` |                              |
| /file/download    | `nickname`, `filename`           |                              |
| /file/status      |                                  | `[ "File" ]`                 |
| /peer/add         | `addr`                           |                              |
| /peer/remove      | `addr`                           |                              |
| /peer/status      |                                  | `[ "Address" ]`              |
| /update/check     |                                  | `{ "Available", "Version" }` |
| /update/apply     | `version`                        |                              |
| /status           |                                  | See StateInfo                |
| /stop             |                                  |                              |
| /sync             |                                  |                              |

HostInfo is a JSON object containing the following values:
```
    "TotalStorage"
    "Minfilesize"
    "Maxfilesize"
    "Minwindow"
    "Minduration"
    "Maxduration"
    "Price"
    "Collateral"
    "StorageRemaining"
    "NumContracts"
```

HostSettings: the following parameters can be set via `/host/setconfig`:
```
totalstorage
minfilesize
maxfilesize
minwindow
minduration
maxduration
price
collateral
```
Only the values specified in the call will be updated.

WalletInfo is a JSON object containing the following fields:
```
{
    "Balance"
    "FullBalance"
    "NumAddresses"
}
```

MinerInfo is a JSON object containing the following fields:
```
{
    "State"
    "Threads"
    "RunningThreads"
    "Address"
}
```

StateInfo is a JSON object containing the following fields:
```
{
    "CurrentBlock"
    "Height"
    "Target"
    "Depth"
    "EarliestLegalTimestamp"
}
```
