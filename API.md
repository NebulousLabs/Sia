Siad API
========

Unless otherwise specified, API calls return the JSON object { "Success": true }.
Errors are sent as plaintext, accompanied by an appropriate status code.

| Path              | Params                       | Response                     |
|:------------------|:-----------------------------|:-----------------------------|
| /host/config      |                              | See HostInfo                 |
| /host/setconfig   | See HostInfo                 |                              |
| /miner/start      | `threads`                    |                              |
| /miner/status     |                              | See MinerInfo                |
| /miner/stop       |                              |                              |
| /wallet/address   |                              | `{ "Address" }`              |
| /wallet/send      | `amount`, `dest`             |                              |
| /wallet/status    |                              | See WalletInfo               |
| /file/upload      | `pieces`, `file`, `nickname` |                              |
| /file/download    | `nickname`, `filename`       |                              |
| /file/status      |                              | `[ "File" ]`                 |
| /peer/add         | `addr`                       |                              |
| /peer/remove      | `addr`                       |                              |
| /peer/status      |                              | `[ "Address" ]`              |
| /update/check     |                              | `{ "Available", "Version" }` |
| /update/apply     | `version` (optional)         |                              |
| /status           |                              | See StateInfo                |
| /stop             |                              |                              |
| /sync             |                              |                              |

HostInfo comprises the following values:
```
totalstorage 
minfile 
maxfile 
mintolerance 
minduration 
maxduration 
minwin 
maxwin 
freezeduration 
price 
penalty 
freezevolume
```

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