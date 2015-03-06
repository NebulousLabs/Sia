Siad API
========

All API calls return JSON objects. If there is an error, the error is returned
in plaintext. The standard response is { "Success": true}. In this document,
the API responses are defined as go structs. The go structs will get encoded to
JSON before being sent. Go structs are used here to provide type information.

Daemon
------

Queries:

* /daemon/stop
* /daemon/update/check
* /daemon/update/apply

#### /daemon/stop

Function: Cleanly shuts down the daemon. May take a while.

Params: none

Response: standard

#### /daemon/update/check:

Function: Checks for an update, returning a bool indicating whether
there is an update and a version indicating the version of the update.

Params: none

Response:
```
struct {
	Available bool
	Version string
}
```

#### /daemon/update/apply:

Function: Applies any updates that are available.

Params: none

Response: standard

-----

| Path              | Params                           | Response                     |
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
