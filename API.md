Siad API
========

| Path              | Params                    | Response                     |
|-------------------|---------------------------|------------------------------|
| /wallet/address   |                           | `{ "Address" }`              |
| /wallet/send      | `amount`, `dest`          |                              |
| /wallet/status    |                           | See WalletInfo               |
| /miner/start      | `threads`                 |                              |
| /miner/stop       |                           |                              |
| /miner/status     |                           | See MinerInfo                |
| /host             | See HostInfo              |                              |
| /rent             | `filepath`, `nickname`    |                              |
| /download         | `nickname`, `destination` |                              |
| /stop             |                           |                              |
| /sync             |                           |                              |
| /peer/add         |                           |                              |
| /peer/remove      |                           |                              |
| /status           |                           | See CoreInfo                 |
| /update/check     |                           | `{ "Available", "Version" }` |
| /update/apply     | `version`                 |                              |

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

CoreInfo is a JSON object containing the following fields:
```
{
    "CurrentBlock"
    "Height"
    "Target"
    "Depth"
    "EarliestLegalTimestamp"
}
```