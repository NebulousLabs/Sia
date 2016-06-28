Siad API
========

The siad API is currently under construction. Under semantic versioning, the
minor version will be incremented whenever API-breaking changes are introduced.
Once siad hits v1.0.0, the major version will be incremented instead.

All API calls return JSON objects. If there is an error, the error is returned
in plaintext with an appropriate HTTP error code. The standard response is {
"Success": true }. In this document, the API responses are defined as Go
structs. The structs will be encoded to JSON before being sent; they are used
here to provide type information.

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Notes:
- Requests must set their User-Agent string to contain the substring "Sia-Agent".
- By default, siad listens on "localhost:9980". This can be changed using the
  '--api-addr' flag when running siad.
- The types.Currency object is an arbitrary-precision unsigned integer. In JSON,
  it is represented as a base-10 string. You must use a "bignum" library to handle
  these values, or you risk losing precision.

Example GET curl call:  `curl -A "Sia-Agent" /wallet/transactions?startheight=1&endheight=250`

Example POST curl call: `curl -A "Sia-Agent" --data "amount=123&destination=abcd" /wallet/siacoins

Standard responses
------------------

#### Success

The standard response indicating the request was successfully processed is HTTP
status code 204.

#### Error

The standard error response indicating the request failed for any reason, is a
4xx or 5xx HTTP status code with an error JSON object describing the error.
```javascript
{
    "message": String

    // There may be additional fields depending on the specific error.
}
```

Table of contents
-----------------

- [Daemon](#daemon)
- [Consensus](#consensus)
- [Explorer](#explorer)
- [Gateway](#gateway)
- [Host](#host)
- [Host DB](#host-db)
- [Miner](#miner)
- [Renter](#renter)
- [Wallet](#wallet)

Daemon
------

Queries:

* /daemon/constants [GET]
* /daemon/stop      [GET]
* /daemon/version   [GET]

#### /daemon/constants [GET]

Function: Returns the set of constants in use.

Parameters: none

Response:
```
struct {
	genesistimestamp      types.Timestamp (uint64)
	blocksizelimit        uint64
	blockfrequency        types.BlockHeight (uint64)
	targetwindow          types.BlockHeight (uint64)
	mediantimestampwindow uint64
	futurethreshold       types.Timestamp   (uint64)
	siafundcount          types.Currency    (string)
	siafundportion        *big.Rat          (string)
	maturitydelay         types.BlockHeight (uint64)

	initialcoinbase uint64
	minimumcoinbase uint64

	roottarget types.Target (byte array)
	rootdepth  types.Target (byte array)

	maxadjustmentup   *big.Rat (string)
	maxadjustmentdown *big.Rat (string)

	siacoinprecision types.Currency (string)
}
```
'genesistimestamp' is the timestamp of the genesis block.

'blocksizelimit' is the maximum size a block can be without being rejected.

'blockfrequency' is the target for how frequently new blocks should be mined.

'targetwindow' is the height of the window used to adjust the difficulty.

'mediantimestampwindow' is the duration of the window used to adjust the
difficulty.

'futurethreshold' is how far in the future a block can be without being
rejected.

'siafundcount' is the total number of siafunds.

'siafundportion' is the percentage of each file contract payout given to
siafund holders.

'maturitydelay' is the number of children a block must have before it is
considered "mature."

'initialcoinbase' is the number of coins given to the miner of the first
block.

'minimumcoinbase' is the minimum number of coins paid out to the miner of a
block (the coinbase decreases with each block).

'roottarget' is the initial target.

'rootdepth' is the initial depth.

'maxadjustmentup' is the largest allowed ratio between the old difficulty and
the new difficulty.

'maxadjustmentdown' is the smallest allowed ratio between the old difficulty
and the new difficulty.

'siacoinprecision' is the number of Hastings in one siacoin.

#### /daemon/stop [GET]

Function: Cleanly shuts down the daemon. May take a few seconds.

Parameters: none

Response: standard

#### /daemon/version [GET]

Function: Returns the version of Sia currently running.

Parameters: none

Response:
```
struct {
	version   string
}
```
'version' is the version of the responding Sia daemon.

Consensus
---------

Queries:

* /consensus                 [GET]

#### /consensus [GET]

Function: Returns information about the consensus set, such as the current
block height.

Parameters: none

Response:
```
struct {
	synced       types.BlockHeight (bool)
	height       types.BlockHeight (uint64)
	currentblock types.BlockID     (string)
	target       types.Target      (byte array)
}
```
'synced' is a bool that indicates if the consensus set is synced with the
network. Will be false during initial blockchain download and true after.

'height' is the number of blocks in the blockchain.

'currentblock' is the hash of the current block.

'target' is the hash that needs to be met by a block for the block to be valid.
The target is inversely proportional to the difficulty.

Explorer
--------

Queries:

* /explorer                 [GET]
* /explorer/blocks/{height} [GET]
* /explorer/hashes/{hash}   [GET]

#### /explorer [GET]

Function: Returns the status of the blockchain and some
statistics. All Siacoin amounts are given in Hastings

Parameters: None

Response:
```
struct {
	height            types.BlockHeight (uint64)
	block             types.Block
	target            types.Target    (byte array)
	difficulty        types.Currency  (string)
	maturitytimestamp types.Timestamp (uint64)
	circulation       types.Currency  (string)

	transactioncount          uint64
	siacoininputcount         uint64
	siacoinoutputcount        uint64
	filecontractcount         uint64
	filecontractrevisioncount uint64
	storageproofcount         uint64
	siafundinputcount         uint64
	siafundoutputcount        uint64
	minerfeecount             uint64
	arbitrarydatacount        uint64
	transactionsignaturecount uint64

	activecontractcount uint64
	activecontractcost  types.Currency (string)
	activecontractsize  types.Currency (string)
	totalcontractcost   types.Currency (string)
	totalcontractsize   types.Currency (string)
}
```

#### /explorer/blocks/{height} [GET]

Function: Returns a block at a given height.

Parameters:
```
height types.BlockHeight (uint64)
```
'height' is the height of the block that is being requested. The genesis block
is at height 0, its child is at height 1, etc.

Response:
```
struct {
	block api.ExplorerBlock
}
```

#### /explorer/hashes/{hash} [GET]

Function: Returns information about an unknown hash.

Parameters:
```
hash crypto.Hash (string)
```
'hash' can be an unlock hash, a wallet address, a block ID, a transaction
ID, siacoin output ID, file contract ID, siafund output ID, or any of the
derivatives of siacoin output IDs (such as miner payout IDs and file contract
payout IDs).

Response:
```
struct {
	 hashtype     string
	 block        api.ExplorerBlock
	 blocks       []api.ExplorerBlock
	 transaction  api.ExplorerTransaction
	 transactions []api.ExplorerTransaction
}
```
'hashtype' indicates what type of hash was supplied. The options are 'blockid',
'transactionid', 'unlockhash', 'siacoinoutputid', 'filecontractid',
'siafundoutputid'. If the object is a block, only the 'block' field will be
filled out. If the object is a transaction, only the 'transaction' field will
be filled out. For all other types, the 'blocks' and 'transactions' fields will
be filled out, returning all of the blocks and transactions that feature the
provided hash.


Gateway
-------

| Route                                                                         | HTTP verb |
| ----------------------------------------------------------------------------- | --------- |
| [/gateway](#gateway-get-example)                                              | GET       |
| [/gateway/connect/{netaddress}](#gatewayconnectnetaddress-post-example)       | POST      |
| [/gateway/disconnect/{netaddress}](#gatewaydisconnectnetaddress-post-example) | POST      |

For examples and detailed descriptions of request and response parameters,
refer to [Gateway.md](/doc/api/Gateway.md).

#### /gateway [GET] [(example)](/doc/api/Gateway.md#gateway-info)

returns information about the gateway, including the list of connected peers.

###### JSON Response [(with comments)](/doc/api/Gateway.md#json-response)
```javascript
{
    "netaddress": String,
    "peers":      []{
        "netaddress": String,
        "version":    String,
        "inbound":    Boolean
    }
}
```

#### /gateway/connect/{netaddress} [POST] [(example)](/doc/api/Gateway.md#connecting-to-a-peer)

connects the gateway to a peer. The peer is added to the node list if it is not
already present. The node list is the list of all nodes the gateway knows
about, but is not necessarily connected to.

###### Path Parameters [(with comments)](/doc/api/Gateway.md#path-parameters)
```
{netaddress}
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /gateway/disconnect/{netaddress} [POST] [(example)](/doc/api/Gateway.md#disconnecting-from-a-peer)

disconnects the gateway from a peer. The peer remains in the node list.

###### Path Parameters [(with comments)](/doc/api/Gateway.md#path-parameters-1)
```
{netaddress}
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

Host
----

Queries:

* /host                                     [GET]
* /host                                     [POST]
* /host/announce                            [POST]
* /host/delete/{filecontractid}             [POST]
* /host/storage                             [GET]
* /host/storage/folders/add                 [POST]
* /host/storage/folders/remove              [POST]
* /host/storage/folders/resize              [POST]
* /host/storage/sectors/delete/{merkleroot} [POST]

[Full Description](api/Host.md)

#### /host [GET]

Function: Fetches status information about the host.

Parameters: none

Response:
```go
struct {
	externalsettings {
		acceptingcontracts   bool
		maxdownloadbatchsize uint64
		maxduration          types.BlockHeight (uint64)
		maxrevisebatchsize   uint64
		netaddress           modules.NetAddress (string)
		remainingstorage     uint64
		sectorsize           uint64
		totalstorage         uint64
		unlockhash           types.UnlockHash (string)
		windowsize           types.BlockHeight (uint64)

		collateral    types.Currency (string)
		maxcollateral types.Currency (string)

		contractprice          types.Currency (string)
		downloadbandwidthprice types.Currency (string)
		storageprice           types.Currency (string)
		uploadbandwidthprice   types.Currency (string)

		revisionnumber uint64
		version        string
	}

	financialmetrics {
		contractcompensation          types.Currency (string)
		potentialcontractcompensation types.Currency (string)

		lockedstoragecollateral types.Currency (string)
		lostrevenue             types.Currency (string)
		loststoragecollateral   types.Currency (string)
		potentialstoragerevenue types.Currency (string)
		riskedstoragecollateral types.Currency (string)
		storagerevenue          types.Currency (string)
		transactionfeeexpenses  types.Currency (string)

		downloadbandwidthrevenue          types.Currency (string)
		potentialdownloadbandwidthrevenue types.Currency (string)
		potentialuploadbandwidthrevenue   types.Currency (string)
		uploadbandwidthrevenue            types.Currency (string)
	}

	internalsettings {
		acceptingcontracts   bool
		maxdownloadbatchsize uint64
		maxduration          types.BlockHeight (uint64)
		maxrevisebatchsize   uint64
		netaddress           modules.NetAddress (string)
		windowsize           types.BlockHeight (uint64)

		collateral       types.Currency (string)
		collateralbudget types.Currency (string)
		maxcollateral    types.Currency (string)

		mincontractprice          types.Currency (string)
		mindownloadbandwidthprice types.Currency (string)
		minstorageprice           types.Currency (string)
		minuploadbandwidthprice   types.Currency (string)
	}

	// Information about the network, specifically various ways in which
	// renters have contacted the host.
	networkmetrics {
		downloadcalls     uint64
		errorcalls        uint64
		formcontractcalls uint64
		renewcalls        uint64
		revisecalls       uint64
		settingscalls     uint64
		unrecognizedcalls uint64
	}
}
```

#### /host [POST]

Function: Configures hosting parameters. All parameters are optional;
unspecified parameters will be left unchanged.

Parameters:
```
acceptingcontracts   bool                        // Optional
maxdownloadbatchsize uint64                      // Optional
maxduration          types.BlockHeight (uint64)  // Optional
maxrevisebatchsize   uint64                      // Optional
netaddress           modules.NetAddress (string) // Optional
windowsize           types.BlockHeight (uint64)  // Optional

collateral       types.Currency (string) // Optional
collateralbudget types.Currency (string) // Optional
maxcollateral    types.Currency (string) // Optional

mincontractprice          types.Currency (string) // Optional
mindownloadbandwidthprice types.Currency (string) // Optional
minstorageprice           types.Currency (string) // Optional
minuploadbandwidthprice   types.Currency (string) // Optional
```

Response: standard

#### /host/announce [POST]

Function: The host will announce itself to the network as a source of storage.
Generally only needs to be called once.

Parameters:
```
netaddress string // Optional
```

Response: standard

#### /host/storage [GET]

Function: Get a list of folders tracked by the host's storage manager.

Parameters: none

Response:
```javascript
{
  "folders": [
    {
      "path":              "/home/foo/bar",
      "capacity":          50000000000,     // bytes
      "capacityremaining": 100000,          // bytes

      "failedreads": 0,
      "failedwrites": 1,
      "successfulreads": 2,
      "successfulwrites": 3
    }
  ]
}
```

#### /host/storage/folders/add [POST]

Function: Add a storage folder to the manager. The manager may not check that
there is enough space available on-disk to support as much storage as requested

Parameters:
```
path // Required
size // bytes, Required
```

Response: standard

#### /host/storage/folders/remove [POST]

Function: Remove a storage folder from the manager. All storage on the folder
will be moved to other storage folders, meaning that no data will be lost. If
the manager is unable to save data, an error will be returned and the operation
will be stopped.

Parameters:
```
path  // Required
force // bool, Optional, default is false
```

Response: standard

#### /host/storage/folders/resize [POST]

Function: Grow or shrink a storage folder in the manager. The manager may not
check that there is enough space on-disk to support growing the storage folder,
but should gracefully handle running out of space unexpectedly. When shrinking
a storage folder, any data in the folder that needs to be moved will be placed
into other storage folders, meaning that no data will be lost. If the manager
is unable to migrate the data, an error will be returned and the operation will
be stopped.

Parameters:
```
path    // Required
newsize // bytes, Required
```

Response: standard

#### /host/storage/sectors/delete/{merkleroot} [POST]

Function: Deletes a sector, meaning that the manager will be unable to upload
that sector and be unable to provide a storage proof on that sector.
DeleteSector is for removing the data entirely, and will remove instances of
the sector appearing at all heights. The primary purpose of DeleteSector is to
comply with legal requests to remove data.

Path Parameters
```
{merkleroot} // Required
```

Response: standard


Host DB
-------

| Request                                     | HTTP Verb |
| ------------------------------------------- | --------- |
| [/hostdb/active](#hostdbactive-get-example) | GET       |
| [/hostdb/all](#hostdball-get-example)       | GET       |

For examples and detailed descriptions of request and response parameters,
refer to [HostDB.md](/doc/api/HostDB.md).

#### /hostdb/active [GET] [(example)](/doc/api/HostDB.md#active-hosts)

lists all of the active hosts known to the renter, sorted by preference.

###### Query String Parameters [(with comments)](/doc/api/HostDB.md#query-string-parameters)
```
numhosts // Optional
```

###### JSON Response [(with comments)](/doc/api/HostDB.md#json-response)
```javascript
{
  "hosts": [
    {
      "acceptingcontracts":   true,
      "maxdownloadbatchsize": 17825792, // bytes
      "maxduration":          25920,    // blocks
      "maxrevisebatchsize":   17825792, // bytes
      "netaddress":           "123.456.789.2:9982",
      "remainingstorage":     35000000000, // bytes
      "sectorsize":           4194304,     // bytes
      "totalstorage":         35000000000, // bytes
      "unlockhash":           "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
      "windowsize":           144, // blocks
      "publickey": {
        "algorithm": "ed25519",
        "key":        "RW50cm9weSBpc24ndCB3aGF0IGl0IHVzZWQgdG8gYmU="
      }
    }
  ]
}
```

#### /hostdb/all [GET] [(example)](/doc/api/HostDB.md#all-hosts)

lists all of the hosts known to the renter. Hosts are not guaranteed to be in
any particular order, and the order may change in subsequent calls.

###### JSON Response [(with comments)](/doc/api/HostDB.md#json-response-1)
```javascript
{
  "hosts": [
    {
      "acceptingcontracts":   true,
      "maxdownloadbatchsize": 17825792, // bytes
      "maxduration":          25920,    // blocks
      "maxrevisebatchsize":   17825792, // bytes
      "netaddress":           "123.456.789.0:9982",
      "remainingstorage":     35000000000, // bytes
      "sectorsize":           4194304,     // bytes
      "totalstorage":         35000000000, // bytes
      "unlockhash":           "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
      "windowsize":           144, // blocks
      "publickey": {
        "algorithm": "ed25519",
        "key":       "RW50cm9weSBpc24ndCB3aGF0IGl0IHVzZWQgdG8gYmU="
      }
    }
  ]
}
```

Miner
-----

Queries:

* /miner        [GET]
* /miner/start  [GET]
* /miner/stop   [GET]
* /miner/header [GET]
* /miner/header [POST]

#### /miner [GET]

Function: Return the status of the miner.

Parameters: none

Response:
```
struct {
	blocksmined      int
	cpuhashrate      int
	cpumining        bool
	staleblocksmined int
}
```
'cpumining' indicates whether the cpu miner is active or not.

'cpuhashrate' indicates how fast the cpu is hashing, in hashes per second.

'blocksmined' indicates how many blocks have been mined, this value is remembered after restarting.

'staleblocksmined' indicates how many stale blocks have been mined, this value is remembered after restarting.

#### /miner/start [GET]

Function: Starts a single threaded cpu miner. Does nothing if the cpu miner is
already running.

Parameters: none

Response: standard

#### /miner/stop [GET]

Function: Stops the cpu miner. Does nothing if the cpu miner is not running.

Parameters: none

Response: standard

#### /miner/header [GET]

Function: Provide a block header that is ready to be grinded on for work.

Parameters: none

Response:
```
[]byte
```
The response is a byte array containing a target followed by a block header
followed by a block. The target is the first 32 bytes. The block header is the
following 80 bytes, and the nonce is bytes 32-39 (inclusive) of the header
(bytes 64-71 of the whole array).

Layout:

0-31: target

32-111: header

#### /miner/header [POST]

Function: Submit a header that has passed the POW.

Parameters:
```
input []byte
```
The input byte array should be 80 bytes that form the solved block header. *Unlike most API calls, it should be written directly to the request body, not as a query parameter.*

Renter
------

Queries:

* /renter/allowance          [GET]
* /renter/allowance          [POST]
* /renter/downloads          [GET]
* /renter/files              [GET]
* /renter/load               [POST]
* /renter/loadascii          [POST]
* /renter/share              [GET]
* /renter/shareascii         [GET]
* /renter/delete/{siapath}   [POST]
* /renter/download/{siapath} [GET]
* /renter/rename/{siapath}   [POST]
* /renter/upload/{siapath}   [POST]

#### /renter/allowance [GET]

Function: Returns the current contract allowance.

Parameters: none

Response:
```
struct {
	funds  types.Currency    (string)
	hosts  uint64
	period types.BlockHeight (uint64)
}
```
'funds' is the number of hastings allocated for file contracts in the given
period.

'hosts' is the number of hosts that contracts will be formed with.

'period' is the duration of contracts formed.

#### /renter/allowance [POST]

Function: Sets the contract allowance.

Parameters: none
```
funds  types.Currency    (string)
hosts  uint64
period types.BlockHeight (uint64)
```
'funds' is the number of hastings allocated for file contracts in the given
period.

'hosts' is the number of hosts that contracts will be formed with.

'period' is the duration of contracts formed.

Response: standard

#### /renter/downloads [GET]

Function: Lists all files in the download queue.

Parameters: none

Response:
```
struct {
	downloads []struct {
		siapath     string
		destination string
		filesize    uint64
		received    uint64
		starttime   Time (string)
	}
}
```
'siapath' is the siapath given to the file when it was uploaded.

'destination' is the path that the file will be downloaded to.

'filesize' is the size of the file being downloaded.

'received' is the number of bytes downloaded thus far.

'starttime' is the time at which the download was initiated.

#### /renter/files

Function: Lists the status of all files.

Parameters: none

Response:
```
struct {
	files []struct {
		siapath        string
		filesize       uint64
		available      bool
		renewing       bool
		uploadprogress float64
		expiration     types.BlockHeight (uint64)
	}
}
```
'siapath' is the location of the file in the renter.

'filesize' is the size of the file in bytes.

'available' indicates whether or not the file can be downloaded immediately.

'renewing' indicates whether or not the file's contracts will be renewed
automatically by the renter.

'uploadprogress' is the current upload percentage of the file, including
redundancy. In general, files will be available for download before
uploadprogress == 100.

'expiration' is the block height at which the file ceases availability.

#### /renter/load [POST]

Function: Load a .sia file into the renter.

Parameters:
```
source string
```
'source' is the location on disk of the .sia file being loaded.

Response:
```
struct {
	filesadded []string
}
```
'filesadded' is an array of renter locations of the files contained in the
.sia file.


#### /renter/loadascii [POST]

Function: Load a .sia file into the renter.

Parameters:
```
asciisia string
```
'asciisia' is the ASCII-encoded .sia file that is being loaded.

Response:
```
struct {
	filesadded []string
}
```
See /renter/load for a description of 'filesadded'

#### /renter/share [GET]

Function: Create a .sia file that can be shared with other people.

Parameters:
```
siapaths    []string
destination string
```
'siapaths' is an array of the renter paths to be shared. It is comma-delimited.

'destination' is the path of the .sia file to be created. It must end in
'.sia'.

Response: standard.

#### /renter/shareascii [GET]

Function: Create an ASCII .sia file that can be shared with other people.

Parameters:
```
siapaths []string
```
'siapaths' is an array of the nicknames to be shared. It is comma-delimited.

Response:
```
struct {
	asciisia string
}
```
'asciisia' is the ASCII-encoded .sia file.

#### /renter/delete/{siapath} [POST]

Function: Deletes a renter file entry. Does not delete any downloads or
original files, only the entry in the renter.

Parameters:
```
siapath string
```
'siapath' is the location of the file in the renter.

Response: standard

#### /renter/download/{siapath} [GET]

Function: Downloads a file. The call will block until the download completes.

Parameters:
```
siapath     string
destination string
```
'siapath' is the location of the file in the renter.

'destination' is the location on disk that the file will be downloaded to.

Response: standard

#### /renter/rename/{siapath} [POST]

Function: Rename a file. Does not rename any downloads or source files, only
renames the entry in the renter.

Parameters:
```
siapath     string
newsiapath  string
```
'siapath' is the current location of the file in the renter.

'newsiapath' is the new location of the file in the renter.

Response: standard.

#### /renter/upload/{siapath} [POST]

Function: Uploads a file.

Parameters:
```
siapath  string
source   string
```
'siapath' is the location where the file will reside in the renter.

'source' is the location on disk of the file being uploaded.

Response: standard.


Wallet
------

Queries:

* /wallet                      [GET]
* /wallet/033x                 [POST]
* /wallet/address              [GET]
* /wallet/addresses            [GET]
* /wallet/backup               [GET]
* /wallet/init                 [POST]
* /wallet/lock                 [POST]
* /wallet/seed                 [POST]
* /wallet/seeds                [GET]
* /wallet/siacoins             [POST]
* /wallet/siafunds             [POST]
* /wallet/siagkey              [POST]
* /wallet/transaction/{id}     [GET]
* /wallet/transactions         [GET]
* /wallet/transactions/{addr}  [GET]
* /wallet/unlock               [POST]

The first time that the wallet is ever created, the wallet will be unencrypted
and locked. The wallet must be initialized and encrypted using a call to 
/wallet/init. After encrypting the wallet, the wallet must be unlocked. From 
that point forward (including restarting siad), the wallet will be encrypted,
and only the call to /wallet/unlock will be needed.

#### /wallet [GET]

Function: Returns basic information about the wallet, such as whether the
wallet is locked or unlocked.

Parameters: none

Response:
```
struct {
	encrypted bool
	unlocked  bool

	confirmedsiacoinbalance     types.Currency (string)
	unconfirmedoutgoingsiacoins types.Currency (string)
	unconfirmedincomingsiacoins types.Currency (string)

	siafundbalance      types.Currency (string)
	siacoinclaimbalance types.Currency (string)
}
```
'encrypted' indicates whether the wallet has been encrypted or not. If the
wallet has not been encrypted, then no data has been generated at all, and the
first time the wallet is unlocked, the password given will be used as the
password for encrypting all of the data. 'encrypted' will only be set to false
if the wallet has never been unlocked before (the unlocked wallet is still
encryped - but the encryption key is in memory).

'unlocked' indicates whether the wallet is currently locked or unlocked. Some
calls become unavailable when the wallet is locked.

'confirmedsiacoinbalance' is the number of siacoins available to the wallet as
of the most recent block in the blockchain.

'unconfirmedoutgoingsiacoins' is the number of siacoins that are leaving the
wallet according to the set of unconfirmed transactions. Often this number
appears inflated, because outputs are frequently larger than the number of
coins being sent, and there is a refund. These coins are counted as outgoing,
and the refund is counted as incoming. The difference in balance can be
calculated using 'unconfirmedincomingsiacoins' - 'unconfirmedoutgoingsiacoins'

'unconfirmedincomingsiacoins' is the number of siacoins are entering the wallet
according to the set of unconfirmed transactions. This number is often inflated
by outgoing siacoins, because outputs are frequently larger than the amount
being sent. The refund will be included in the unconfirmed incoming siacoins
balance.

'siafundbalance' is the number of siafunds available to the wallet as
of the most recent block in the blockchain.

'siacoinclaimbalance' is the number of siacoins that can be claimed from the
siafunds as of the most recent block. Because the claim balance increases every
time a file contract is created, it is possible that the balance will increase
before any claim transaction is confirmed.

#### /wallet/033x [POST]

Function: Load a v0.3.3.x wallet into the current wallet, harvesting all of the
secret keys. All spendable addresses in the loaded wallet will become spendable
from the current wallet.

Parameters:
```
source             string
encryptionpassword string
```
'source' is the location on disk of the v0.3.3.x wallet that is being loaded.

'encryptionpassword' is the encryption key of the wallet. An error will be
returned if the wrong key is provided.

Response: standard.

#### /wallet/address [GET]

Function: Get a new address from the wallet generated by the primary seed. An
error will be returned if the wallet is locked.

Parameters: none

Response:
```
struct {
	address types.UnlockHash (string)
}
```
'address' is a wallet address that can receive siacoins or siafunds.

#### /wallet/addresses [GET]

Function: Fetch the list of addresses from the wallet.

Parameters: none

Response:
```
struct {
	addresses []types.UnlockHash (string)
}
```
'addresses' is an array of wallet addresses.

#### /wallet/backup [GET]

Function: Create a backup of the wallet settings file. Though this can easily
be done manually, the settings file is often in an unknown or difficult to find
location. The /wallet/backup call can spare users the trouble of needing to
find their wallet file.

Parameters:
```
destination string
```
'destination' is the location on disk where the file will be saved.

Response: standard

#### /wallet/init [POST]

Function: Initialize the wallet. After the wallet has been initialized once, it
does not need to be initialized again, and future calls to /wallet/init will
return an error. The encryption password is provided by the api call. If the
password is blank, then the password will be set to the same as the seed.

Parameters:
```
encryptionpassword string
dictionary string
```
'encryptionpassword' is the password that will be used to encrypt the wallet.
All subsequent calls should use this password. If left blank, the seed that
gets returned will also be the encryption password.

'dictionary' is the name of the dictionary that should be used when encoding
the seed. 'english' is the most common choice when picking a dictionary.

Response:
```
struct {
	primaryseed string
}
```
'primaryseed' is the dictionary encoded seed that is used to generate addresses
that the wallet is able to spend.

#### /wallet/seed [POST]

Function: Give the wallet a seed to track when looking for incoming
transactions. The wallet will be able to spend outputs related to addresses
created by the seed. The seed is added as an auxiliary seed, and does not
replace the primary seed. Only the primary seed will be used for generating new
addresses.

Parameters:
```
encryptionpassword string
dictionary         string
seed               string
```
'encryptionpassword' is the key that is used to encrypt the new seed when it is
saved to disk.

'dictionary' is the name of the dictionary that should be used when encoding
the seed. 'english' is the most common choice when picking a dictionary.

'seed' is the dictionary-encoded phrase that corresponds to the seed being
added to the wallet.

Response: standard

#### /wallet/seeds [GET]

Function: Return a list of seeds in use by the wallet. The primary seed is the
only seed that gets used to generate new addresses. This call is unavailable
when the wallet is locked.

Parameters:
```
dictionary string
```
'dictionary' is the name of the dictionary that should be used when encoding
the seed. 'english' is the most common choice when picking a dictionary.

Response:
```
struct {
	primaryseed        mnemonics.Phrase   (string)
	addressesremaining int
	allseeds           []mnemonics.Phrase ([]string)
}
```
'primaryseed' is the seed that is actively being used to generate new addresses
for the wallet.

'addressesremaining' is the number of addresses that remain in the primary seed
until exhaustion has been reached. Once exhaustion has been reached, new
addresses will continue to be generated but they will be more difficult to
recover in the event of a lost wallet file or encryption password.

'allseeds' is an array of all seeds that the wallet references when scanning the
blockchain for outputs. The wallet is able to spend any output generated by any
of the seeds, however only the primary seed is being used to generate new
addresses.

A seed is an encoded version of a 128 bit random seed. The output is 15 words
chosen from a small dictionary as indicated by the input. The most common
choice for the dictionary is going to be 'english'. The underlying seed is the
same no matter what dictionary is used for the encoding. The encoding also
contains a small checksum of the seed, to help catch simple mistakes when
copying. The library
[entropy-mnemonics](https://github.com/NebulousLabs/entropy-mnemonics) is used
when encoding.

#### /wallet/siacoins [POST]

Function: Send siacoins to an address. The outputs are arbitrarily selected
from addresses in the wallet.

Parameters:
```
amount      int
destination types.UnlockHash (string)
```
'amount' is the number of hastings being sent. A hasting is the smallest unit
in Sia. There are 10^24 hastings in a siacoin.

'destination' is the address that is receiving the coins.

Response:
```
struct {
	transactionids []types.TransactionID ([]string)
}
```
'transactionids' are the ids of the transactions that were created when sending
the coins. The last transaction contains the output headed to the
'destination'.

#### /wallet/siafunds [POST]

Function: Send siafunds to an address. The outputs are arbitrarily selected
from addresses in the wallet. Any siacoins available in the siafunds being sent
(as well as the siacoins available in any siafunds that end up in a refund
address) will become available to the wallet as siacoins after 144
confirmations. To access all of the siacoins in the siacoin claim balance, send
all of the siafunds to an address in your control (this will give you all the
siacoins, while still letting you control the siafunds).

Parameters:
```
amount      int
destination string
```
'amount' is the number of siafunds being sent.

'destination' is the address that is receiving the funds.

Response:
```
struct {
	transactionids []types.TransactionID ([]string)
}
```
'transactionids' are the ids of the transactions that were created when sending
the coins. The last transaction contains the output headed to the
'destination'.

#### /wallet/siagkey [POST]

Function: Load a key into the wallet that was generated by siag. Most siafunds
are currently in addresses created by siag.

Parameters:
```
encryptionpassword string
keyfiles           string
```
'encryptionpassword' is the key that is used to encrypt the siag key when it is
imported to the wallet.

'keyfiles' is a list of filepaths that point to the keyfiles that make up the
siag key. There should be at least one keyfile per required signature. The
filenames need to be commna separated (no spaces), which means filepaths that
contain a comma are not allowed.

#### /wallet/lock [POST]

Function: Locks the wallet, wiping all secret keys. After being locked, the
keys are encrypted. Queries for the seed, to send siafunds, and related queries
become unavailable. Queries concerning transaction history and balance are
still available.

Parameters: none

Response: standard.

#### /wallet/transaction/{id} [GET]

Function: Get the transaction associated with a specific transaction id.

Parameters:
```
id string
```
'id' is the ID of the transaction being requested.

Response:
```
struct {
	transaction modules.ProcessedTransaction
}
```

Processed transactions are transactions that have been processed by the wallet
and given more information, such as a confirmation height and a timestamp.
Processed transactions will always be returned in chronological order.

A processed transaction takes the following form:
```
struct modules.ProcessedTransaction {
	transaction           types.Transaction
	transactionid         types.TransactionID (string)
	confirmationheight    types.BlockHeight   (int)
	confirmationtimestamp types.Timestamp     (uint64)

	inputs  []modules.ProcessedInput
	outputs []modules.ProcessedOutput
}
```
'transaction' is a types.Transaction, and is defined in types/transactions.go

'transactionid' is the id of the transaction from which the wallet transaction
was derived.

'confirmationheight' is the height at which the transaction was confirmed. The
height will be set to 'uint64max' if the transaction has not been confirmed.

'confirmationtimestamp' is the time at which a transaction was confirmed. The
timestamp is an unsigned 64bit unix timestamp, and will be set to 'uint64max'
if the transaction is unconfirmed.

'inputs' is an array of processed inputs detailing the inputs to the
transaction. More information below.

'outputs' is an array of processed outputs detailing the outputs of
the transaction. Outputs related to file contracts are excluded.

A modules.ProcessedInput takes the following form:
```
struct modules.ProcessedInput {
	fundtype       types.Specifier  (string)
	walletaddress  bool
	relatedaddress types.UnlockHash (string)
	value          types.Currency   (string)
}
```

'fundtype' indicates what type of fund is represented by the input. Inputs can
be of type 'siacoin input', and 'siafund input'.

'walletaddress' indicates whether the address is owned by the wallet.
 
'relatedaddress' is the address that is affected. For inputs (outgoing money),
the related address is usually not important because the wallet arbitrarily
selects which addresses will fund a transaction. For outputs (incoming money),
the related address field can be used to determine who has sent money to the
wallet.

'value' indicates how much money has been moved in the input or output.

A modules.ProcessedOutput takes the following form:
```
struct modules.ProcessedOutput {
	fundtype       types.Specifier   (string)
	maturityheight types.BlockHeight (int)
	walletaddress  bool
	relatedaddress types.UnlockHash  (string)
	value          types.Currency    (string)
}
```

'fundtype' indicates what type of fund is represented by the output. Outputs
can be of type 'siacoin output', 'siafund output', and 'claim output'. Siacoin
outputs and claim outputs both relate to siacoins. Siafund outputs relate to
siafunds. Another output type, 'miner payout', points to siacoins that have been
spent on a miner payout. Because the destination of the miner payout is determined by
the block and not the transaction, the data 'maturityheight', 'walletaddress',
and 'relatedaddress' are left blank.

'maturityheight' indicates what height the output becomes available to be
spent. Siacoin outputs and siafund outputs mature immediately - their maturity
height will always be the confirmation height of the transaction. Claim outputs
cannot be spent until they have had 144 confirmations, thus the maturity height
of a claim output will always be 144 larger than the confirmation height of the
transaction.

'walletaddress' indicates whether the address is owned by the wallet.
 
'relatedaddress' is the address that is affected.

'value' indicates how much money has been moved in the input or output.

#### /wallet/transactions [GET]

Function: Return a list of transactions related to the wallet.

Parameters:
```
startheight types.BlockHeight (uint64)
endheight   types.BlockHeight (uint64)
```
'startheight' refers to the height of the block where transaction history
should begin.

'endheight' refers to the height of of the block where the transaction history
should end. If 'endheight' is greater than the current height, all transactions
up to and including the most recent block will be provided.

Response:
```
struct {
	confirmedtransactions   []modules.ProcessedTransaction
	unconfirmedtransactions []modules.ProcessedTransaction
}
```
'confirmedtransactions' lists all of the confirmed transactions appearing between
height 'startheight' and height 'endheight' (inclusive).

'unconfirmedtransactions' lists all of the unconfirmed transactions.

#### /wallet/transactions/{addr} [GET]

Function: Return all of the transaction related to a specific address.

Parameters:
```
addr types.UnlockHash
```
'addr' is the unlock hash (i.e. wallet address) whose transactions are being
requested.

Response:
```
struct {
	transactions []modules.ProcessedTransaction.
}
```
'transactions' is a list of processed transactions that relate to the supplied
address.  See the documentation for '/wallet/transaction' for more information.

#### /wallet/unlock [POST]

Function: Unlock the wallet. The wallet is capable of knowing whether the
correct password was provided.

Parameters:
```
encryptionpassword string
```
'encryptionpassword' is the password that gets used to decrypt the file. Most
frequently, the encryption password is the same as the primary wallet seed.

Response: standard
