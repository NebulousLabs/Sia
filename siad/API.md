Siad API
========

All API calls return JSON objects. If there is an error, the error is returned
in plaintext. The standard response is { "Success": true}. In this document,
the API responses are defined as Go structs. The Go structs will get encoded to
JSON before being sent. Go structs are used here to provide type information.

At version 0.4, the API will be locked into forwards compatibility. This means
that we will not remove elements from the responses (but we may add additional
elements), and we will not add any required parameters (though we may add
optional parameters, and we may disable parameters).

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Daemon
------

Queries:

* /daemon/stop
* /daemon/update/apply
* /daemon/update/check

#### /daemon/stop

Function: Cleanly shuts down the daemon. May take a while.

Parameters: none

Response: standard

#### /daemon/update/apply:

Function: Applies any updates that are available.

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
address string
```
address should be a reachable hostname + port number. typically of the form
"a.b.c.d:xxxx".

Response: standard

#### /gateway/peer/remove

Function: Will remove a peer from the gateway.

Parameters:
```
address string
```
address should be a reachable hostname + port number. typically of the form
"a.b.c.d:xxxx".

Response: standard

Host
----

Queries:

* /host/announce
* /host/config
* /host/status

#### /host/announce

Function: The host will announce itself to the network as a source of storage.
Generally only needs to be called once.

Parameters: none

Response: standard

#### /host/config

Function: Sets the configuration of the host.

Parameters:
```
totalStorage int
minFilesize  int
maxFilesize  int
minDuration  int
maxDuration  int
windowSize   int
price        int
collateral   int
```
totalStorage (in bytes) is how much storage the host will rent to the network.

minFilesize is the minimum allowed file size.

maxFilesize is the maximum allowed file size.

minDuration is the minimum amount of time a contract is allowed to last.

maxDuration is the maximum amount of time a contract is allowed to last.

windowSize is the number of blocks a host has to prove they are holding the file.

price is the cost in Hastings per byte per block of hosting files on the network.

collateral is the amount of collateral the host will offer in Hastings per byte
per block for losing files on the network.

Response: standard

#### /host/status

Function: Queries the host for general information.

Parameters: none

Response:
```
struct {
	TotalStorage     int
	MinFilesize      int
	MaxFilesize      int
	MinDuration      int
	MaxDuration      int
	WindowSize       int
	Price            int
	Collateral       int
	StorageRemaining int
	NumContracts     int
}
```

HostDB
------

Queries: none

Miner
-----

Queries:

* /miner/start
* /miner/status
* /miner/stop

#### /miner/start

Function: Starts the miner.

Parameters: none

Response: standard

#### /miner/status

Parameters: none

Response:
```
struct {
	Mining bool
}
```
If the Mining flag is set, the miner is currently mining. Otherwise it is not.

#### /miner/stop

Function: Stops the miner.

Parameters: none

Response: standard

Renter
------

Queries:

* /renter/download
* /renter/files
* /renter/upload

#### /renter/download

Function: Starts a file download.

Parameters:
```
nickname    string
destination string
```
nickname is the nickname of the file that has been uploaded to the network.

destination is the filepath that the file should be downloaded to.

Response: standard

#### /renter/files

Function: Lists the status of all files.

Parameters: none

Response:
```
[]struct {
	Available bool
	Nickname  string
	Repairing bool
	TimeRemaining int
}
```
The above is an array of objects, where each object represents a singe file.

Available indicates whether or not the file can be downloaded immediately.

Files is a type.

Nickname is the name the renter uses for the file.

Repairing indicates whether the file is currently being repaired. It is
typically best not to shut down siad until files are no longer being repaired.

TimeRemaining indicates how many blocks the file will be available for.

#### /renter/upload

Function: Upload a file using a filepath.

Parameters:
```
source   string
nickname string
```
source is a filename.

nickname is the name the renter uses for the file.

Response: standard.

Wallet
------

Queries:

* /wallet/address
* /wallet/send
* /wallet/status

#### /wallet/address

Function: Returns an address that is spendable by the wallet.

Parameters: none

Response:
```
struct {
	Address string
}
```
Address is a hex representation of a wallet address.

#### /wallet/send

Function: Sends coins to a destination address.

Parameters:
```
amount      int
destination string
```
amount is a volume of Hastings.

destination is an address represented in hex.

Response: standard

#### /wallet/status

Function: Get the status of the wallet.

Parameters: none

Response:
```
struct {
	Balance int
}
```
Balance is the spendable balance of the wallet.
