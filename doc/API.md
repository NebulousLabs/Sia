Siad API
========

Sia uses semantic versioning and is backwards compatible to version v1.0.0.

API calls return either JSON or no content. Success is indicated by 2xx HTTP
status codes, while errors are indicated by 4xx and 5xx HTTP status codes. If
an endpoint does not specify its expected status code refer to
[#standard-responses](#standard-responses).

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Notes:
- Requests must set their User-Agent string to contain the substring "Sia-Agent".
- By default, siad listens on "localhost:9980". This can be changed using the
  `--api-addr` flag when running siad.
- **Do not bind or expose the API to a non-loopback address unless you are
  aware of the possible dangers.**

Example GET curl call:
```
curl -A "Sia-Agent" "localhost:9980/wallet/transactions?startheight=1&endheight=250"
```

Example POST curl call:
```
curl -A "Sia-Agent" --data "amount=123&destination=abcd" "localhost:9980/wallet/siacoins"
```

Standard responses
------------------

#### Success

The standard response indicating the request was successfully processed is HTTP
status code `204 No Content`. If the request was successfully processed and the
server responded with JSON the HTTP status code is `200 OK`. Specific endpoints
may specify other 2xx status codes on success.

#### Error

The standard error response indicating the request failed for any reason, is a
4xx or 5xx HTTP status code with an error JSON object describing the error.
```javascript
{
    "message": String

    // There may be additional fields depending on the specific error.
}
```

Authentication
--------------

API authentication can be enabled with the `--authenticate-api` siad flag.
Authentication is HTTP Basic Authentication as described in
[RFC 2617](https://tools.ietf.org/html/rfc2617), however, the username is the
empty string. The flag does not enforce authentication on all API endpoints.
Only endpoints that expose sensitive information or modify state require
authentication.

For example, if the API password is "foobar" the request header should include
```
Authorization: Basic OmZvb2Jhcg==
```

Units
-----

Unless otherwise specified, all parameters should be specified in their
smallest possible unit. For example, size should always be specified in bytes
and Siacoins should be specified in hastings. JSON values returned by the API
will also use the smallest possible unit, unless otherwise specified.

If a numbers is returned as a string in JSON, it should be treated as an
arbitrary-precision number (bignum), and it should be parsed with your
language's corresponding bignum library. Currency values are the most common
example where this is necessary.

Table of contents
-----------------

- [Daemon](#daemon)
- [Consensus](#consensus)
- [Gateway](#gateway)
- [Host](#host)
- [Host DB](#host-db)
- [Miner](#miner)
- [Renter](#renter)
- [Wallet](#wallet)

Daemon
------

| Route                                     | HTTP verb |
| ----------------------------------------- | --------- |
| [/daemon/constants](#daemonconstants-get) | GET       |
| [/daemon/stop](#daemonstop-get)           | GET       |
| [/daemon/version](#daemonversion-get)     | GET       |

For examples and detailed descriptions of request and response parameters,
refer to [Daemon.md](/doc/api/Daemon.md).

#### /daemon/constants [GET]

returns the set of constants in use.

###### JSON Response [(with comments)](/doc/api/Daemon.md#json-response)
```javascript
{
  "genesistimestamp":      1257894000, // Unix time
  "blocksizelimit":        2000000,    // bytes
  "blockfrequency":        600,        // seconds per block
  "targetwindow":          1000,       // blocks
  "mediantimestampwindow": 11,         // blocks
  "futurethreshold":       10800,      // seconds
  "siafundcount":          "10000",
  "siafundportion":        "39/1000",
  "maturitydelay":         144,        // blocks

  "initialcoinbase": 300000, // Siacoins (see note in Daemon.md)
  "minimumcoinbase": 30000,  // Siacoins (see note in Daemon.md)

  "roottarget": [0,0,0,0,32,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
  "rootdepth":  [255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255,255],

  "maxadjustmentup":   "5/2",
  "maxadjustmentdown": "2/5",

  "siacoinprecision": "1000000000000000000000000" // hastings per siacoin
}
```

#### /daemon/stop [GET]

cleanly shuts down the daemon. May take a few seconds.

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /daemon/version [GET]

returns the version of the Sia daemon currently running.

###### JSON Response [(with comments)](/doc/api/Daemon.md#json-response-1)
```javascript
{
  "version": "1.0.0"
}
```

Consensus
---------

| Route                                                                       | HTTP verb |
| --------------------------------------------------------------------------- | --------- |
| [/consensus](#consensus-get)                                                | GET       |
| [/consensus/validate/transactionset](#consensusvalidatetransactionset-post) | POST      |

For examples and detailed descriptions of request and response parameters,
refer to [Consensus.md](/doc/api/Consensus.md).

#### /consensus [GET]

returns information about the consensus set, such as the current block height.

###### JSON Response [(with comments)](/doc/api/Consensus.md#json-response)
```javascript
{
  "synced":       true,
  "height":       62248,
  "currentblock": "00000000000008a84884ba827bdc868a17ba9c14011de33ff763bd95779a9cf1",
  "target":       [0,0,0,0,0,0,11,48,125,79,116,89,136,74,42,27,5,14,10,31,23,53,226,238,202,219,5,204,38,32,59,165]
}
```

#### /consensus/validate/transactionset [POST]

validates a set of transactions using the current utxo set.

###### Request Body Bytes

Since transactions may be large, the transaction set is supplied in the POST
body, encoded in JSON format.

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

Gateway
-------

| Route                                                                              | HTTP verb |
| ---------------------------------------------------------------------------------- | --------- |
| [/gateway](#gateway-get-example)                                                   | GET       |
| [/gateway/connect/___:netaddress___](#gatewayconnectnetaddress-post-example)       | POST      |
| [/gateway/disconnect/___:netaddress___](#gatewaydisconnectnetaddress-post-example) | POST      |

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

#### /gateway/connect/___:netaddress___ [POST] [(example)](/doc/api/Gateway.md#connecting-to-a-peer)

connects the gateway to a peer. The peer is added to the node list if it is not
already present. The node list is the list of all nodes the gateway knows
about, but is not necessarily connected to.

###### Path Parameters [(with comments)](/doc/api/Gateway.md#path-parameters)
```
:netaddress
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /gateway/disconnect/___:netaddress___ [POST] [(example)](/doc/api/Gateway.md#disconnecting-from-a-peer)

disconnects the gateway from a peer. The peer remains in the node list.

###### Path Parameters [(with comments)](/doc/api/Gateway.md#path-parameters-1)
```
:netaddress
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

Host
----

| Route                                                                                 | HTTP verb |
| ------------------------------------------------------------------------------------- | --------- |
| [/host](#host-get)                                                                    | GET       |
| [/host](#host-post)                                                                   | POST      |
| [/host/announce](#hostannounce-post)                                                  | POST      |
| [/host/storage](#hoststorage-get)                                                     | GET       |
| [/host/storage/folders/add](#hoststoragefoldersadd-post)                              | POST      |
| [/host/storage/folders/remove](#hoststoragefoldersremove-post)                        | POST      |
| [/host/storage/folders/resize](#hoststoragefoldersresize-post)                        | POST      |
| [/host/storage/sectors/delete/___:merkleroot___](#hoststoragesectorsdeletemerkleroot) | POST      |

For examples and detailed descriptions of request and response parameters,
refer to [Host.md](/doc/api/Host.md).

#### /host [GET]

fetches status information about the host.

###### JSON Response [(with comments)](/doc/api/Host.md#json-response)
```javascript
{
  "externalsettings": {
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

    "collateral":    "57870370370",                     // hastings / byte / block
    "maxcollateral": "100000000000000000000000000000",  // hastings

    "contractprice":          "30000000000000000000000000", // hastings
    "downloadbandwidthprice": "250000000000000",            // hastings / byte
    "storageprice":           "231481481481",               // hastings / byte / block
    "uploadbandwidthprice":   "100000000000000",            // hastings / byte

    "revisionnumber": 0,
    "version":        "1.0.0"
  },

  "financialmetrics": {
    "contractcount":                 2,
    "contractcompensation":          "123", // hastings
    "potentialcontractcompensation": "123", // hastings

    "lockedstoragecollateral": "123", // hastings
    "lostrevenue":             "123", // hastings
    "loststoragecollateral":   "123", // hastings
    "potentialstoragerevenue": "123", // hastings
    "riskedstoragecollateral": "123", // hastings
    "storagerevenue":          "123", // hastings
    "transactionfeeexpenses":  "123", // hastings

    "downloadbandwidthrevenue":          "123", // hastings
    "potentialdownloadbandwidthrevenue": "123", // hastings
    "potentialuploadbandwidthrevenue":   "123", // hastings
    "uploadbandwidthrevenue":            "123"  // hastings
  },

  "internalsettings": {
    "acceptingcontracts":   true,
    "maxdownloadbatchsize": 17825792, // bytes
    "maxduration":          25920,    // blocks
    "maxrevisebatchsize":   17825792, // bytes
    "netaddress":           "123.456.789.0:9982",
    "windowsize":           144, // blocks

    "collateral":       "57870370370",                     // hastings / byte / block
    "collateralbudget": "2000000000000000000000000000000", // hastings
    "maxcollateral":    "100000000000000000000000000000",  // hastings

    "mincontractprice":          "30000000000000000000000000", // hastings
    "mindownloadbandwidthprice": "250000000000000",            // hastings / byte
    "minstorageprice":           "231481481481",               // hastings / byte / block
    "minuploadbandwidthprice":   "100000000000000"             // hastings / byte
  },

  "networkmetrics": {
    "downloadcalls":     0,
    "errorcalls":        1,
    "formcontractcalls": 2,
    "renewcalls":        3,
    "revisecalls":       4,
    "settingscalls":     5,
    "unrecognizedcalls": 6
  }
}
```

#### /host [POST]

configures hosting parameters. All parameters are optional; unspecified
parameters will be left unchanged.

###### Query String Parameters [(with comments)](/doc/api/Host.md#query-string-parameters)
```
acceptingcontracts   // Optional, true / false
maxdownloadbatchsize // Optional, bytes
maxduration          // Optional, blocks
maxrevisebatchsize   // Optional, bytes
netaddress           // Optional
windowsize           // Optional, blocks

collateral       // Optional, hastings / byte / block
collateralbudget // Optional, hastings
maxcollateral    // Optional, hastings

mincontractprice          // Optional, hastings
mindownloadbandwidthprice // Optional, hastings / byte
minstorageprice           // Optional, hastings / byte / block
minuploadbandwidthprice   // Optional, hastings / byte
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/announce [POST]

Announces the host to the network as a source of storage. Generally only needs
to be called once.

###### Query String Parameters [(with comments)](/doc/api/Host.md#query-string-parameters-1)
```
netaddress string // Optional
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/storage [GET]

gets a list of folders tracked by the host's storage manager.

###### JSON Response [(with comments)](/doc/api/Host.md#json-response-1)
```javascript
{
  "folders": [
    {
      "path":              "/home/foo/bar",
      "capacity":          50000000000,     // bytes
      "capacityremaining": 100000,          // bytes

      "failedreads":      0,
      "failedwrites":     1,
      "successfulreads":  2,
      "successfulwrites": 3
    }
  ]
}
```

#### /host/storage/folders/add [POST]

adds a storage folder to the manager. The manager may not check that there is
enough space available on-disk to support as much storage as requested

###### Query String Parameters [(with comments)](/doc/api/Host.md#query-string-parameters-2)
```
path // Required
size // bytes, Required
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/storage/folders/remove [POST]

remove a storage folder from the manager. All storage on the folder will be
moved to other storage folders, meaning that no data will be lost. If the
manager is unable to save data, an error will be returned and the operation
will be stopped.

###### Query String Parameters [(with comments)](/doc/api/Host.md#query-string-parameters-3)
```
path  // Required
force // bool, Optional, default is false
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/storage/folders/resize [POST]

grows or shrink a storage folder in the manager. The manager may not check that
there is enough space on-disk to support growing the storage folder, but should
gracefully handle running out of space unexpectedly. When shrinking a storage
folder, any data in the folder that needs to be moved will be placed into other
storage folders, meaning that no data will be lost. If the manager is unable to
migrate the data, an error will be returned and the operation will be stopped.

###### Query String Parameters [(with comments)](/doc/api/Host.md#query-string-parameters-4)
```
path    // Required
newsize // bytes, Required
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/storage/sectors/delete/___:merkleroot___ [POST]

deletes a sector, meaning that the manager will be unable to upload that sector
and be unable to provide a storage proof on that sector. This endpoint is for
removing the data entirely, and will remove instances of the sector appearing
at all heights. The primary purpose is to comply with legal requests to remove
data.

###### Path Parameters [(with comments)](/doc/api/Host.md#path-parameters)
```
:merkleroot
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).


Host DB
-------

| Request                                                 | HTTP Verb |
| ------------------------------------------------------- | --------- |
| [/hostdb/active](#hostdbactive-get-example)             | GET       |
| [/hostdb/all](#hostdball-get-example)                   | GET       |
| [/hostdb/hosts/___:pubkey___](#hostdbhosts-get-example) | GET       |

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
      "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
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
      "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    }
  ]
}
```

#### /hostdb/hosts/___:pubkey___ [GET] [(example)](/doc/api/HostDB.md#host-details)

fetches detailed information about a particular host, including metrics
regarding the score of the host within the database. It should be noted that
each renter uses different metrics for selecting hosts, and that a good score on
in one hostdb does not mean that the host will be successful on the network
overall.

###### Path Parameters [(with comments)](/doc/api/HostDB.md#path-parameters)
```
:pubkey
```

###### JSON Response [(with comments)](/doc/api/HostDB.md#json-response-2)
```javascript
{
  "entry": {
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
    "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
  },
  "scorebreakdown": {
    "ageadjustment":              0.1234,
    "burnadjustment":             0.1234,
    "collateraladjustment":       23.456,
    "priceadjustment":            0.1234,
    "storageremainingadjustment": 0.1234,
    "uptimeadjustment":           0.1234,
    "versionadjustment":          0.1234,
  }
}
```


Miner
-----

| Route                              | HTTP verb |
| ---------------------------------- | --------- |
| [/miner](#miner-get)               | GET       |
| [/miner/start](#minerstart-get)    | GET       |
| [/miner/stop](#minerstop-get)      | GET       |
| [/miner/header](#minerheader-get)  | GET       |
| [/miner/header](#minerheader-post) | POST      |

For examples and detailed descriptions of request and response parameters,
refer to [Miner.md](/doc/api/Miner.md).

#### /miner [GET]

returns the status of the miner.

###### JSON Response [(with comments)](/doc/api/Miner.md#json-response)
```javascript
{
  "blocksmined":      9001,
  "cpuhashrate":      1337,
  "cpumining":        false,
  "staleblocksmined": 0,
}
```

#### /miner/start [GET]

starts a single threaded cpu miner. Does nothing if the cpu miner is already
running.

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /miner/stop [GET]

stops the cpu miner. Does nothing if the cpu miner is not running.

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /miner/header [GET]

provides a block header that is ready to be grinded on for work.

###### Byte Response

For efficiency the header for work is returned as a raw byte encoding of the
header, rather than encoded to JSON. Refer to
[Miner.md#byte-response](/doc/api/Miner.md#byte-response) for a detailed
description of the byte encoding.

#### /miner/header [POST]

submits a header that has passed the POW.

###### Request Body Bytes

For efficiency headers are submitted as raw byte encodings of the header in the
body of the request, rather than as a query string parameter or path parameter.
The request body should contain only the 80 bytes of the encoded header. The
encoding is the same encoding used in `/miner/header [GET]` endpoint. Refer to
[Miner.md#byte-response](/doc/api/Miner.md#byte-response) for a detailed
description of the byte encoding.

Renter
------

| Route                                                                   | HTTP verb |
| ----------------------------------------------------------------------- | --------- |
| [/renter](#renter-get)                                                  | GET       |
| [/renter](#renter-post)                                                 | POST      |
| [/renter/contracts](#rentercontracts-get)                               | GET       |
| [/renter/downloads](#renterdownloads-get)                               | GET       |
| [/renter/prices](#renterprices-get)                                     | GET       |
| [/renter/files](#renterfiles-get)                                       | GET       |
| [/renter/delete/___*siapath___](#renterdeletesiapath-post)              | POST      |
| [/renter/download/___*siapath___](#renterdownloadsiapath-get)           | GET       |
| [/renter/downloadasync/___*siapath___](#renterdownloadasyncsiapath-get) | GET       |
| [/renter/rename/___*siapath___](#renterrenamesiapath-post)              | POST      |
| [/renter/upload/___*siapath___](#renteruploadsiapath-post)              | POST      |

For examples and detailed descriptions of request and response parameters,
refer to [Renter.md](/doc/api/Renter.md).

#### /renter [GET]

returns the current settings along with metrics on the renter's spending.

###### JSON Response [(with comments)](/doc/api/Renter.md#json-response)
```javascript
{
  "settings": {
    "allowance": {
      "funds":       "1234", // hastings
      "hosts":       24,
      "period":      6048, // blocks
      "renewwindow": 3024  // blocks
    }
  },
  "financialmetrics": {
    "contractspending": "1234", // hastings
    "downloadspending": "5678", // hastings
    "storagespending":  "1234", // hastings
    "uploadspending":   "5678", // hastings
    "unspent":          "1234"  // hastings
  }
}
```

#### /renter [POST]

modify settings that control the renter's behavior.

###### Query String Parameters [(with comments)](/doc/api/Renter.md#query-string-parameters)
```
funds // hastings
hosts
period      // block height
renewwindow // block height
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /renter/contracts [GET]

returns active contracts. Expired contracts are not included.

###### JSON Response [(with comments)](/doc/api/Renter.md#json-response-1)
```javascript
{
  "contracts": [
    {
      "endheight":       50000, // block height
      "id":              "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "lasttransaction": {}, // types.Transaction
      "netaddress":      "12.34.56.78:9",
      "renterfunds":     "1234", // hastings
      "size":            8192    // bytes
    }
  ]
}
```

#### /renter/downloads [GET]

lists all files in the download queue.

###### JSON Response [(with comments)](/doc/api/Renter.md#json-response-2)
```javascript
{
  "downloads": [
    {
      "siapath":     "foo/bar.txt",
      "destination": "/home/users/alice/bar.txt",
      "filesize":    8192,                  // bytes
      "received":    4096,                  // bytes
      "starttime":   "2009-11-10T23:00:00Z", // RFC 3339 time
      "error": ""
    }
  ]
}
```

#### /renter/files [GET]

lists the status of all files.

###### JSON Response [(with comments)](/doc/api/Renter.md#json-response-3)
```javascript
{
  "files": [
    {
      "siapath":        "foo/bar.txt",
      "filesize":       8192, // bytes
      "available":      true,
      "renewing":       true,
      "redundancy":     5,
      "uploadprogress": 100, // percent
      "expiration":     60000
    }
  ]
}
```

#### /renter/prices [GET]

lists the estimated prices of performing various storage and data operations.

###### JSON Response [(with comments)](/doc/api/Renter.md#json-response-4)
```javascript
{
  "downloadterabyte":      "1234", // hastings
  "formcontracts":         "1234", // hastings
  "storageterabytemonth":  "1234", // hastings
  "uploadterabyte":        "1234", // hastings
}
```


#### /renter/delete/___*siapath___ [POST]

deletes a renter file entry. Does not delete any downloads or original files,
only the entry in the renter.

###### Path Parameters [(with comments)](/doc/api/Renter.md#path-parameters)
```
*siapath
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /renter/download/___*siapath___ [GET]

downloads a file to the local filesystem. The call will block until the file
has been downloaded.

###### Path Parameters [(with comments)](/doc/api/Renter.md#path-parameters-1)
```
*siapath
```

###### Query String Parameters [(with comments)](/doc/api/Renter.md#query-string-parameters-1)
```
destination
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /renter/downloadasync/___*siapath___ [GET]

downloads a file to the local filesystem. The call will return immediately.

###### Path Parameters [(with comments)](/doc/api/Renter.md#path-parameters-2)
```
*siapath
```

###### Query String Parameters [(with comments)](/doc/api/Renter.md#query-string-parameters-2)
```
destination
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /renter/rename/___*siapath___ [POST]

renames a file. Does not rename any downloads or source files, only renames the
entry in the renter. An error is returned if `siapath` does not exist or
`newsiapath` already exists.

###### Path Parameters [(with comments)](/doc/api/Renter.md#path-parameters-3)
```
*siapath
```

###### Query String Parameters [(with comments)](/doc/api/Renter.md#query-string-parameters-3)
```
newsiapath
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /renter/upload/___*siapath___ [POST]

uploads a file to the network from the local filesystem.

###### Path Parameters [(with comments)](/doc/api/Renter.md#path-parameters-4)
```
*siapath
```

###### Query String Parameters [(with comments)](/doc/api/Renter.md#query-string-parameters-4)
```
datapieces   // int
paritypieces // int
source       // string - a filepath
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).


Wallet
------

| Route                                                           | HTTP verb |
| --------------------------------------------------------------- | --------- |
| [/wallet](#wallet-get)                                          | GET       |
| [/wallet/033x](#wallet033x-post)                                | POST      |
| [/wallet/address](#walletaddress-get)                           | GET       |
| [/wallet/addresses](#walletaddresses-get)                       | GET       |
| [/wallet/backup](#walletbackup-get)                             | GET       |
| [/wallet/init](#walletinit-post)                                | POST      |
| [/wallet/init/seed](#walletinitseed-post)                       | POST      |
| [/wallet/lock](#walletlock-post)                                | POST      |
| [/wallet/seed](#walletseed-post)                                | POST      |
| [/wallet/seeds](#walletseeds-get)                               | GET       |
| [/wallet/siacoins](#walletsiacoins-post)                        | POST      |
| [/wallet/siafunds](#walletsiafunds-post)                        | POST      |
| [/wallet/siagkey](#walletsiagkey-post)                          | POST      |
| [/wallet/sweep/seed](#walletsweepseed-post)                     | POST      |
| [/wallet/transaction/___:id___](#wallettransactionid-get)       | GET       |
| [/wallet/transactions](#wallettransactions-get)                 | GET       |
| [/wallet/transactions/___:addr___](#wallettransactionsaddr-get) | GET       |
| [/wallet/unlock](#walletunlock-post)                            | POST      |

For examples and detailed descriptions of request and response parameters,
refer to [Wallet.md](/doc/api/Wallet.md).

#### /wallet [GET]

returns basic information about the wallet, such as whether the wallet is
locked or unlocked.

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response)
```javascript
{
  "encrypted": true,
  "unlocked":  true,

  "confirmedsiacoinbalance":     "123456", // hastings, big int
  "unconfirmedoutgoingsiacoins": "0",      // hastings, big int
  "unconfirmedincomingsiacoins": "789",    // hastings, big int

  "siafundbalance":      "1",    // siafunds, big int
  "siacoinclaimbalance": "9001", // hastings, big int
}
```

#### /wallet/033x [POST]

loads a v0.3.3.x wallet into the current wallet, harvesting all of the secret
keys. All spendable addresses in the loaded wallet will become spendable from
the current wallet.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters)
```
source
encryptionpassword
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /wallet/address [GET]

gets a new address from the wallet generated by the primary seed. An error will
be returned if the wallet is locked.

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-1)
```javascript
{
  "address": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab"
}
```

#### /wallet/addresses [GET]

fetches the list of addresses from the wallet.

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-2)
```javascript
{
  "addresses": [
    "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  ]
}
```

#### /wallet/backup [GET]

creates a backup of the wallet settings file. Though this can easily be done
manually, the settings file is often in an unknown or difficult to find
location. The /wallet/backup call can spare users the trouble of needing to
find their wallet file.

###### Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-1)
```
destination
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /wallet/init [POST]

initializes the wallet. After the wallet has been initialized once, it does
not need to be initialized again, and future calls to /wallet/init will return
an error. The encryption password is provided by the api call. If the password
is blank, then the password will be set to the same as the seed.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-2)
```
encryptionpassword
dictionary // Optional, default is english.
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-3)
```javascript
{
  "primaryseed": "hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello"
}
```

#### /wallet/init/seed [POST]

initializes the wallet using a preexisting seed. After the wallet has been
initialized once, it does not need to be initialized again, and future calls
to /wallet/init/seed will return an error. The encryption password is provided
by the api call. If the password is blank, then the password will be set to
the same as the seed. Note that loading a preexisting seed requires scanning
the blockchain to determine how many keys have been generated from the seed.
For this reason, /wallet/init/seed can only be called if the blockchain is
synced.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-3)
```
encryptionpassword
dictionary // Optional, default is english.
seed
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /wallet/seed [POST]

gives the wallet a seed to track when looking for incoming transactions. The
wallet will be able to spend outputs related to addresses created by the seed.
The seed is added as an auxiliary seed, and does not replace the primary seed.
Only the primary seed will be used for generating new addresses.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-4)
```
encryptionpassword
dictionary
seed
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /wallet/seeds [GET]

returns the list of seeds in use by the wallet. The primary seed is the only
seed that gets used to generate new addresses. This call is unavailable when
the wallet is locked.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-5)
```
dictionary
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-4)
```javascript
{
  "primaryseed":        "hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello",
  "addressesremaining": 2500,
  "allseeds":           [
    "hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello",
    "foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo",
  ]
}
```

#### /wallet/siacoins [POST]

sends siacoins to an address. The outputs are arbitrarily selected from
addresses in the wallet.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-6)
```
amount      // hastings
destination // address
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-5)
```javascript
{
  "transactionids": [
    "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  ]
}
```

#### /wallet/siafunds [POST]

sends siafunds to an address. The outputs are arbitrarily selected from
addresses in the wallet. Any siacoins available in the siafunds being sent (as
well as the siacoins available in any siafunds that end up in a refund address)
will become available to the wallet as siacoins after 144 confirmations. To
access all of the siacoins in the siacoin claim balance, send all of the
siafunds to an address in your control (this will give you all the siacoins,
while still letting you control the siafunds).

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-7)
```
amount      // siafunds
destination // address
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-6)
```javascript
{
  "transactionids": [
    "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  ]
}
```

#### /wallet/siagkey [POST]

loads a key into the wallet that was generated by siag. Most siafunds are
currently in addresses created by siag.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-8)
```
encryptionpassword
keyfiles
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /wallet/sweep/seed [POST]

Function: Scan the blockchain for outputs belonging to a seed and send them to
an address owned by the wallet.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-9)
```
dictionary // Optional, default is english.
seed
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-7)
```javascript
{
  "coins": "123456", // hastings, big int
  "funds": "1",      // siafunds, big int
}
```

#### /wallet/lock [POST]

locks the wallet, wiping all secret keys. After being locked, the keys are
encrypted. Queries for the seed, to send siafunds, and related queries become
unavailable. Queries concerning transaction history and balance are still
available.

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /wallet/transaction/___:id___ [GET]

gets the transaction associated with a specific transaction id.

###### Path Parameters [(with comments)](/doc/api/Wallet.md#path-parameters)
```
:id
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-8)
```javascript
{
  "transaction": {
    "transaction": {
      // See types.Transaction in https://github.com/NebulousLabs/Sia/blob/master/types/transactions.go
    },
    "transactionid":         "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "confirmationheight":    50000,
    "confirmationtimestamp": 1257894000,
    "inputs": [
      {
        "fundtype":       "siacoin input",
        "walletaddress":  false,
        "relatedaddress": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
        "value":          "1234", // hastings or siafunds, depending on fundtype, big int
      }
    ],
    "outputs": [
      {
        "fundtype":       "siacoin output",
        "maturityheight": 50000,
        "walletaddress":  false,
        "relatedaddress": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "value":          "1234", // hastings or siafunds, depending on fundtype, big int
      }
    ]
  }
}
```

#### /wallet/transactions [GET]

returns a list of transactions related to the wallet in chronological order.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-10)
```
startheight // block height
endheight   // block height
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-9)
```javascript
{
  "confirmedtransactions": [
    {
      // See the documentation for '/wallet/transaction/:id' for more information.
    }
  ],
  "unconfirmedtransactions": [
    {
      // See the documentation for '/wallet/transaction/:id' for more information.
    }
  ]
}
```

#### /wallet/transactions/___:addr___ [GET]

returns all of the transactions related to a specific address.

###### Path Parameters [(with comments)](/doc/api/Wallet.md#path-parameters-1)
```
:addr
```

###### JSON Response [(with comments)](/doc/api/Wallet.md#json-response-10)
```javascript
{
  "transactions": [
    {
      // See the documentation for '/wallet/transaction/:id' for more information.
    }
  ]
}
```

#### /wallet/unlock [POST]

unlocks the wallet. The wallet is capable of knowing whether the correct
password was provided.

###### Query String Parameters [(with comments)](/doc/api/Wallet.md#query-string-parameters-11)
```
encryptionpassword
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).
