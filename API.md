Siad API
========

Unless otherwise specified, API calls return the JSON object { "Success": true }.
Errors are sent as plaintext, accompanied by an appropriate status code.

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
    TotalStorage"
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
