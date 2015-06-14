Siad API
========

All API calls return JSON objects. If there is an error, the error is returned
in plaintext with an appropriate HTTP error code. The standard response is {
"Success": true }. In this document, the API responses are defined as Go
structs. The structs will be encoded to JSON before being sent; they are used
here to provide type information.

At version 0.4, the API will be locked into forwards compatibility. This means
that we will not add new required parameters or remove response fields. We
may, however, add additional fields and optional parameters, and we may
disable parameters.

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

The default port for the API is "localhost:9980". This can be changed using the
'-a' flag when running siad.

Consensus
---------

Queries:

* /consensus/status
* /consensus/synchronize

#### /consensus/status

Function: Returns information about the consensus set, such as the current
block height.

Parameters: none

Response:
```
struct {
	Height       int
	CurrentBlock [32]byte
	Target       [32]byte
}
```

#### /consensus/synchronize

Function: Will force synchronization of the local node and the rest of the
network. May take a while. Should only be necessary for debugging.

Parameters: none

Reponse: standard

Daemon
------

Queries:

* /daemon/stop
* /daemon/updates/apply
* /daemon/updates/check

#### /daemon/stop

Function: Cleanly shuts down the daemon. May take a while.

Parameters: none

Response: standard

#### /daemon/updates/apply:

Function: Applies the update specified by `version`.

Parameters:
```
version string
```

Response: standard

#### /daemon/updates/check:

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

Gateway
-------

Queries:

* /gateway/status
* /gateway/peers/add
* /gateway/peers/remove

#### /gateway/status

Function: Returns information about the gateway, including the list of peers.

Parameters: none

Response:
```
struct {
	Address NetAddress
	Peers   []string
}
```

#### /gateway/peers/add

Function: Will add a peer to the gateway.

Parameters:
```
address string
```
`address` should be a reachable hostname + port number, typically of the form
"a.b.c.d:xxxx".

Response: standard

#### /gateway/peers/remove

Function: Will remove a peer from the gateway.

Parameters:
```
address string
```
`address` should be a reachable hostname + port number, typically of the form
"a.b.c.d:xxxx".

Response: standard

Host
----

Queries:

* /host/announce
* /host/configure
* /host/status

#### /host/announce

Function: The host will announce itself to the network as a source of storage.
Generally only needs to be called once.

Parameters: none

Response: standard

#### /host/configure

Function: Configures hosting parameters. All parameters are optional;
unspecified parameters will be left unchanged.

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
`totalStorage` is how much storage (in bytes) the host will rent to the
network.

`minFilesize` is the minimum allowed file size.

`maxFilesize` is the maximum allowed file size.

`minDuration` is the minimum amount of time a contract is allowed to last.

`maxDuration` is the maximum amount of time a contract is allowed to last.

`windowSize` is the number of blocks a host has to prove they are holding the
file.

`price` is the cost (in Hastings per byte) of data stored.

`collateral` is the amount of collateral the host will offer (in Hastings per
byte per block) for losing files on the network.

Response: standard

#### /host/status

Function: Queries the host for its configuration values, as well as the amount
of storage remaining and the number of contracts formed.

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

Queries:

* /hostdb/hosts/active

#### /hostdb/hosts/active

Function: Lists all of the active hosts in the hostdb.

Parameters: none

Response:
```
struct {
	Hosts []HostSettings
}
```

Miner
-----

Queries:

* /miner/blockforwork
* /miner/start
* /miner/status
* /miner/stop
* /miner/submitblock

#### /miner/blockforwork

Function: Provides a block that is ready for the blockchain except for having
an invalid target. An external miner is expected to try nonces until a block
with a valid target is found. Every block returned by this call will have a
unique hash - miners can start at nonce 0 without worrying about trying the
same block twice.

Parameters: none

Response:
```
[]byte
```
The response is a byte array containing a targe followed by a block header
followed by a block. The target is the first 32 bytes. The block header is the
following 80 bytes, and the nonce is bytes 32-39 (inclusive) of the header
(bytes 64-71 of the whole array). The remaining bytes are the block. Hashing
the header will result in the block id, which must be lower than the target.
When submitting a solved block, you need to update the nonce in the block. The
nonce is at bytes 32-39 (inclusive) of the block, or bytes 144-151 of the whole
field. When submitting the block to /miner/submitblock, only submit the block
and not the target or header.

Layout:

0-31: target

32-111: header

112+: block

32-39 of header: header nonce

32-39 of block: block nonce

When submitting the block, update the nonce and submit only the block (bytes
112+).

#### /miner/start

Function: Tells the miner to begin mining on `threads` threads.

Parameters:
```
threads int
```

Response: standard

#### /miner/status

Parameters: none

Response:
```
struct {
	Mining         bool
	State          string
	Threads        int
	RunningThreads int
	Address        [32]byte
}
```
If the `Mining` flag is set, the miner is currently mining. Otherwise it is
not.

`State` gives a more nuanced description of the miner, including
transitional states.

`Threads` indicates the number of desired threads, while
`RunningThreads` is the number of currently active threads. If the miner finds
a block,

`Address` is the address that will receive the coinbase.

#### /miner/stop

Function: Stops the miner.

Parameters: none

Response: standard

#### /miner/submitblock

Function: Submits a block to the miner. The block is submitted as an encoded
byte slice. The block does not need to be modified from the block provided by
/miner/blockforwork except that the nonce must be updated. The nonce can be
found at bytes 32-39 (inclusive) of the block.

Response Body:
```
[]byte
```

Response: standard

Renter
------

Queries:

* /renter/downloadqueue
* /renter/files/delete
* /renter/files/download
* /renter/files/list
* /renter/files/load
* /renter/files/loadascii
* /renter/files/rename
* /renter/files/share
* /renter/files/shareascii
* /renter/files/upload

#### /renter/downloadqueue

Function: Lists all files in the download queue.

Parameters: none

Response:
```
[]struct{
	Complete    bool
	Filesize    uint64
	Received    uint64
	Destination string
	Nickname    string
}
```
Each file in the queue is represented by the above struct.

`Complete` indicates whether the file is ready to be used. Note that `Received
== Filesize` does not imply `Complete`, because the file may require
additional processing (e.g. decryption) after all of the raw bytes have been
downloaded.

`Filesize` is the size of the file being download.

`Received` is the number of bytes downloaded thus far.

`Destination` is the path that the file was downloaded to.

`Nickname` is the nickname given to the file when it was uploaded.

#### /renter/files/delete

Function: Deletes a renter file entry. Does not delete any downloads or
original files, only the entry in the renter.

Parameters:
```
nickname string
```
`nickname` is the nickname of the file that has been uploaded to the network.

Response: standard

#### /renter/files/download

Function: Starts a file download.

Parameters:
```
nickname    string
destination string
```
`nickname` is the nickname of the file that has been uploaded to the network.

`destination` is the path that the file will be downloaded to.

Response: standard

#### /renter/files/list

Function: Lists the status of all files.

Parameters: none

Response:
```
[]struct {
	Available     bool
	Nickname      string
	Repairing     bool
	TimeRemaining int
}
```
Each uploaded file is represented by the above struct.

`Available` indicates whether or not the file can be downloaded immediately.

`Nickname` is the nickname given to the file when it was uploaded.

`Repairing` indicates whether the file is currently being repaired. It is
typically best not to shut down siad until files are no longer being repaired.

`TimeRemaining` indicates how many blocks the file will be available for.

#### /renter/files/load

Function: Load a '.sia' into the renter.

Parameters:
```
filename string
```
`filename` is the filepath of the '.sia' that is being loaded.

Response:
```
struct {
	FilesAdded []string
}
```

#### /renter/files/loadascii

Function: Load a '.sia' into the renter.

Parameters:
```
file string
```
`file` is the ASCII representation of the '.sia' file being loaded into the
renter.

Response:
```
struct {
	FilesAdded []string
}
```

#### /renter/files/rename

Function: Rename a file. Does not rename any downloads or source files, only
renames the entry in the renter.

Parameters:
```
nickname string
newname  string
```
`nickname` is the current name of the file entry.

`newname` is the new name for the file entry.

Response: standard.

#### /renter/files/share

Function: Create a '.sia' that can be shared with other people.

Parameters:
```
nickname string
filepath string
```
`nickname` is the nickname of the file that will be shared.

`filepath` is the filepath of the '.sia' that will be created to share the
file. `filepath` must have the suffix '.sia'.

Response: standard.

#### /renter/files/shareascii

Function: Create a '.sia' that can be shared with other people.

Parameters:
```
nickname string
```
`nickname` is the nickname of the file that will be shared.

Response:
```
File string
```
`file` is the ASCII representation of the '.sia' that would have been created.

#### /renter/files/upload

Function: Upload a file.

Parameters:
```
source   string
nickname string
```
`source` is the path to the file to be uploaded.

`nickname` is the name that will be used to reference the file.

Response: standard.

Transaction Pool
----------------

Queries:

* /transactionpool/transactions

#### /transactionpool/transactions

Function: Returns all of the transactions in the transaction pool.

Parameters: none

Response:
```
struct {
	Transactions []consensus.Transaction
}
```
Please see consensus/types/transactions.go for a more detailed explanation on
what a transaction looks like. There are many fields.

Wallet
------

Queries:

* /wallet/address
* /wallet/send
* /wallet/siafunds/balance
* /wallet/siafunds/spend
* /wallet/siafunds/watchsiagaddress
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
`Address` is the hex representation of a wallet address.

#### /wallet/send

Function: Sends coins to a destination address.

Parameters:
```
amount      int
destination string
```
`amount` is a volume of coins to send, in Hastings.

`destination` is the hex representation of the recipient address.

Response: standard

#### /wallet/siafunds/balance

Function: Display the balance of siafunds tracked by the wallet.

Parameters: none

Response:
```
struct {
	SiafundBalance      int
	SiacoinClaimBalance int
}
```

#### /wallet/siafunds/spend

Function: Spend siafunds tracked by the wallet. The siacoins stored in the
siafunds are sent to the wallet.

Parameters:
```
amount      int
destination string
keyfiles    string
```
`amount` is a volume of coins to send, in Hastings.

`destination` is the hex representation of the recipient address.

`keyfiles` is a comma-separated list of files containing the keys used to
unlock the siafunds.

#### /wallet/siafunds/watchsiagaddress

Function: Track a new siafund address.

Parameters:
```
keyfile string
```
`keyfile` is a file containing a key corresponding to the address.


Response: standard

#### /wallet/status

Function: Get the status of the wallet.

Parameters: none

Response:
```
struct {
	Balance      int
	FullBalance  int
	NumAddresses int
}
```
`Balance` is the spendable balance of the wallet.

`FullBalance` is the balance of the wallet, including unconfirmed coins.

`NumAddresses` is the number of addresses controlled by the wallet.

Block Explorer
--------------

Queries:

* /blockexplorer/blockchain
* /blockexplorer/current
* /blockexplorer/siacoins
* /blockexplorer/filecontracts
* /blockexplorer/blockdata

### /blockexplorer/status

Function: Returns the status of the blockchain and some
statistics. All Siacoin amounts are given in Hastings

Parameters: None

Response:
```
struct {
       Height int
       Block  Types.Block
       Target []byte
       CurrencySent  int
       TotalCurrency int
       ActiveContractCount int
       ActiveContractcosts int
	   TotalContractCount int
       TotalContractcosts int
}
```
`Height` is the current height of the blockchain.

`Block` is the most recently mined valid block. See types/block.go for
a detailed specification of a block structure.

`Target` is the target at which the current block has been mined at.

`TotalCurrency` is the total amount of siacoins in ciculation

`ActiveContractCount` is the number of file contracts that are currently in place.

`ActiveContractCosts` is the amount of siacoins that are currently tied
up in file contracts.

`TotalContractCount` is the number of file contracts that have ever
took place

`TotalContractCosts` is the amount of siacoins that have ever been
spent on file contracts

### /blockexplorer/blockdata

Function: Return information about specific blocks

Parameters:
```
start  int
finish int
```
`start` is the starting block height.

`finish` is the finishing block height.

Response:
```
[]struct {
	 Timestamp int
	 Target    int
	 Size	   int
}

`Timestamp` is the timestamp registered on the block.

`Target` is the target at which the block was mined at.

`Size` is the size, in bytes of the marshalled block.
