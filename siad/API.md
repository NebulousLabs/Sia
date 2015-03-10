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
struct {
	TotalStorage int
	MinFilesize  int
	MaxFilesize  int
	MinDuration  int
	MaxDuration  int
	WindowSize   int
	Price        int
	Collateral   int
}
```
TotalStorage (in bytes) is how much storage the host will rent to the network.

MinFilesize is the minimum allowed file size.

MaxFilesize is the maximum allowed file size.

MinDuration is the minimum amount of time a contract is allowed to last.

MaxDuration is the maximum amount of time a contract is allowed to last.

WindowSize is the number of blocks a host has to prove they are holding the file.

Price is the cost in Hastings per byte per block of hosting files on the network.

Collateral is the amount of collateral the host will offer in Hastings per byte
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
* /renter/status
* /renter/upload

#### /renter/download

Function: Starts a file download.

Parameters:
```
struct {
	Nickname    string
	Destination string
}
```
Nickname is the nickname of the file that has been uploaded to the network.

Destination is the filepath that the file should be downloaded to.

Response: standard

#### /renter/status

Function: Returns the status of the renter.

Parameters: none

Response:
```
struct {
	Files []string
}
```
Files is a list of all the files by their nickname.

#### /renter/upload

Function: Upload a file using a filepath.

Parameters:
```
struct {
	Source   string
	Nickname string
}
```
Source is a filename.

Nickname is the name the renter uses for the file.

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
struct {
	Amount      int
	Destination string
}
```
Amount is a volume of Hastings.

Destination is an address represented in hex.

Response: standard

#### /wallet/status

Function: Get the status of the wallet.

Parameters: none

Response:
```
struct {
	Balance      int
}
```
Balance is the spendable balance of the wallet.
