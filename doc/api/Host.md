Host API
--------

This document contains detailed descriptions of the host's API routes. For an
overview of the host's API routes, see [API.md#host](/doc/API.md#host).  For an
overview of all API routes, see [API.md](/doc/API.md)

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Overview
--------

The host provides storage from local disks to the network. The host negotiates
file contracts with remote renters to earn money for storing other users'
files. The host's endpoints expose methods for viewing and modifying host
settings, announcing to the network, and managing how files are stored on disk.

Index
-----

| Route                                                                                      | HTTP verb |
| ------------------------------------------------------------------------------------------ | --------- |
| [/host](#host-get)                                                                         | GET       |
| [/host](#host-post)                                                                        | POST      |
| [/host/announce](#hostannounce-post)                                                       | POST      |
| [/host/contracts](#hostcontracts-get)                                                      | GET       |
| [/host/estimatescore](#hostestimatescore-get)                                              | GET       |
| [/host/storage](#hoststorage-get)                                                          | GET       |
| [/host/storage/folders/add](#hoststoragefoldersadd-post)                                   | POST      |
| [/host/storage/folders/remove](#hoststoragefoldersremove-post)                             | POST      |
| [/host/storage/folders/resize](#hoststoragefoldersresize-post)                             | POST      |
| [/host/storage/sectors/delete/:___merkleroot___](#hoststoragesectorsdeletemerkleroot-post) | POST      |


#### /host [GET]

fetches status information about the host.

###### JSON Response
```javascript
{
  // The settings that get displayed to untrusted nodes querying the host's
  // status.
  "externalsettings": {
    // Whether or not the host is accepting new contracts.
    "acceptingcontracts": true,

    // The maximum size of a single download request from a renter. Each
    // download request has multiple round trips of communication that
    // exchange money. Larger batch sizes mean fewer round trips, but more
    // financial risk for the host - the renter can get a free batch when
    // downloading by refusing to provide a signature.
    "maxdownloadbatchsize": 17825792, // bytes

    // The maximum duration that a host will allow for a file contract. The
    // host commits to keeping files for the full duration under the threat
    // of facing a large penalty for losing or dropping data before the
    // duration is complete. The storage proof window of an incoming file
    // contract must end before the current height + maxduration.
    "maxduration": 25920, // blocks

    // The maximum size of a single batch of file contract revisions. The
    // renter can perform DoS attacks on the host by uploading a batch of
    // data then refusing to provide a signature to pay for the data. The
    // host can reduce this exposure by limiting the batch size. Larger
    // batch sizes allow for higher throughput as there is significant
    // communication overhead associated with performing a batch upload.
    "maxrevisebatchsize": 17825792, // bytes

    // The IP address or hostname (including port) that the host should be
    // contacted at.
    "netaddress": "123.456.789.0:9982",

    // The amount of unused storage capacity on the host in bytes. It
    // should be noted that the host can lie.
    "remainingstorage": 35000000000, // bytes

    // The smallest amount of data in bytes that can be uploaded or
    // downloaded when performing calls to the host.
    "sectorsize": 4194304, // bytes

    // The total amount of storage capacity on the host. It should be noted
    // that the host can lie.
    "totalstorage": 35000000000, // bytes

    // The unlock hash is the address at which the host can be paid when
    // forming file contracts.
    "unlockhash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",

    // The storage proof window is the number of blocks that the host has
    // to get a storage proof onto the blockchain. The window size is the
    // minimum size of window that the host will accept in a file contract.
    "windowsize": 144, // blocks

    // The maximum amount of money that the host will put up as collateral
    // for storage that is contracted by the renter.
    "collateral": "57870370370", // hastings / byte / block

    // The maximum amount of collateral that the host will put into a
    // single file contract.
    "maxcollateral": "100000000000000000000000000000",  // hastings

    // The price that a renter has to pay to create a contract with the
    // host. The payment is intended to cover transaction fees
    // for the file contract revision and the storage proof that the host
    // will be submitting to the blockchain.
    "contractprice": "30000000000000000000000000", // hastings

    // The price that a renter has to pay when downloading data from the
    // host.
    "downloadbandwidthprice": "250000000000000", // hastings / byte

    // The price that a renter has to pay to store files with the host.
    "storageprice": "231481481481", // hastings / byte / block

    // The price that a renter has to pay when uploading data to the host.
    "uploadbandwidthprice": "100000000000000", // hastings / byte

    // The revision number indicates to the renter what iteration of
    // settings the host is currently at. Settings are generally signed.
    // If the renter has multiple conflicting copies of settings from the
    // host, the renter can expect the one with the higher revision number
    // to be more recent.
    "revisionnumber": 0,

    // The version of external settings being used. This field helps
    // coordinate updates while preserving compatibility with older nodes.
    "version": "1.0.0"
  },

  // The financial status of the host.
  "financialmetrics": {
    // Number of open file contracts.
    "contractcount": 2,

    // The amount of money that renters have given to the host to pay for
    // file contracts. The host is required to submit a file contract
    // revision and a storage proof for every file contract that gets created,
    // and the renter pays for the miner fees on these objects.
    "contractcompensation": "123", // hastings

    // The amount of money that renters have given to the host to pay for
    // file contracts which have not been confirmed yet. The potential
    // compensation becomes compensation after the storage proof is
    // submitted.
    "potentialcontractcompensation": "123", // hastings

    // The amount of storage collateral which the host has tied up in file
    // contracts. The host has to commit collateral to a file contract even
    // if there is no storage, but the locked collateral will be returned
    // even if the host does not submit a storage proof - the collateral is
    // not at risk, it is merely set aside so that it can be put at risk
    // later.
    "lockedstoragecollateral": "123", // hastings

    // The amount of revenue, including storage revenue and bandwidth
    // revenue, that has been lost due to failed file contracts and
    // failed storage proofs.
    "lostrevenue": "123", // hastings

    // The amount of collateral that was put up to protect data which has
    // been lost due to failed file contracts and missed storage proofs.
    "loststoragecollateral": "123", // hastings

    // The amount of revenue that the host stands to earn if all storage
    // proofs are submitted corectly and in time.
    "potentialstoragerevenue": "123", // hastings

    // The amount of money that the host has risked on file contracts. If
    // the host starts missing storage proofs, the host can forfeit up to
    // this many coins. In the event of a missed storage proof, locked
    // storage collateral gets returned, but risked storage collateral
    // does not get returned.
    "riskedstoragecollateral": "123", // hastings

    // The amount of money that the host has earned from storing data. This
    // money has been locked down by successful storage proofs.
    "storagerevenue": "123", // hastings

    // The amount of money that the host has spent on transaction fees when
    // submitting host announcements, file contract revisions, and storage
    // proofs.
    "transactionfeeexpenses": "123", // hastings

    // The amount of money that the host has made from renters downloading
    // their files. This money has been locked in by successsful storage
    // proofs.
    "downloadbandwidthrevenue": "123", // hastings

    // The amount of money that the host stands to make from renters that
    // downloaded their files. The host will only realize this revenue if
    // the host successfully submits storage proofs for the related file
    // contracts.
    "potentialdownloadbandwidthrevenue": "123", // hastings

    // The amount of money that the host stands to make from renters that
    // uploaded files. The host will only realize this revenue if the host
    // successfully submits storage proofs for the related file contracts.
    "potentialuploadbandwidthrevenue": "123", // hastings

    // The amount of money that the host has made from renters uploading
    // their files. This money has been locked in by successful storage
    // proofs.
    "uploadbandwidthrevenue": "123" // hastings
  },

  // The settings of the host. Most interactions between the user and the
  // host occur by changing the internal settings.
  "internalsettings": {
    // When set to true, the host will accept new file contracts if the
    // terms are reasonable. When set to false, the host will not accept new
    // file contracts at all.
    "acceptingcontracts": true,

    // The maximum size of a single download request from a renter. Each
    // download request has multiple round trips of communication that
    // exchange money. Larger batch sizes mean fewer round trips, but more
    // financial risk for the host - the renter can get a free batch when
    // downloading by refusing to provide a signature.
    "maxdownloadbatchsize": 17825792, // bytes

    // The maximum duration of a file contract that the host will accept.
    // The storage proof window must end before the current height +
    // maxduration.
    "maxduration": 25920, // blocks

    // The maximum size of a single batch of file contract revisions. The
    // renter can perform DoS attacks on the host by uploading a batch of
    // data then refusing to provide a signature to pay for the data. The
    // host can reduce this exposure by limiting the batch size. Larger
    // batch sizes allow for higher throughput as there is significant
    // communication overhead associated with performing a batch upload.
    "maxrevisebatchsize": 17825792, // bytes

    // The IP address or hostname (including port) that the host should be
    // contacted at. If left blank, the host will automatically figure out
    // its ip address and use that. If given, the host will use the address
    // given.
    "netaddress": "123.456.789.0:9982",

    // The storage proof window is the number of blocks that the host has
    // to get a storage proof onto the blockchain. The window size is the
    // minimum size of window that the host will accept in a file contract.
    "windowsize": 144, // blocks

    // The maximum amount of money that the host will put up as collateral
    // per byte per block of storage that is contracted by the renter.
    "collateral": "57870370370", // hastings / byte / block

    // The total amount of money that the host will allocate to collateral
    // across all file contracts.
    "collateralbudget": "2000000000000000000000000000000", // hastings

    // The maximum amount of collateral that the host will put into a
    // single file contract.
    "maxcollateral": "100000000000000000000000000000", // hastings

    // The minimum price that the host will demand from a renter when
    // forming a contract. Typically this price is to cover transaction
    // fees on the file contract revision and storage proof, but can also
    // be used if the host has a low amount of collateral. The price is a
    // minimum because the host may automatically adjust the price upwards
    // in times of high demand.
    "mincontractprice": "30000000000000000000000000", // hastings

    // The minimum price that the host will demand from a renter when the
    // renter is downloading data. If the host is saturated, the host may
    // increase the price from the minimum.
    "mindownloadbandwidthprice": "250000000000000", // hastings / byte

    // The minimum price that the host will demand when storing data for
    // extended periods of time. If the host is low on space, the price of
    // storage may be set higher than the minimum.
    "minstorageprice": "231481481481", // hastings / byte / block

    // The minimum price that the host will demand from a renter when the
    // renter is uploading data. If the host is saturated, the host may
    // increase the price from the minimum.
    "minuploadbandwidthprice": "100000000000000" // hastings / byte
  },

  // Information about the network, specifically various ways in which
  // renters have contacted the host.
  "networkmetrics": {
    // The number of times that a renter has attempted to download
    // something from the host.
    "downloadcalls": 0,

    // The number of calls that have resulted in errors. A small number of
    // errors are expected, but a large number of errors indicate either
    // buggy software or malicious network activity. Usually buggy
    // software.
    "errorcalls": 1,

    // The number of times that a renter has tried to form a contract with
    // the host.
    "formcontractcalls": 2,

    // The number of times that a renter has tried to renew a contract with
    // the host.
    "renewcalls": 3,

    // The number of times that the renter has tried to revise a contract
    // with the host.
    "revisecalls": 4,

    // The number of times that a renter has queried the host for the
    // host's settings. The settings include the price of bandwidth, which
    // is a price that can adjust every few minutes. This value is usually
    // very high compared to the others.
    "settingscalls": 5,

    // The number of times that a renter has attempted to use an
    // unrecognized call. Larger numbers typically indicate buggy software.
    "unrecognizedcalls": 6
  },

  // Information about the health of the host.

  // connectabilitystatus is one of "checking", "connectable",
  // or "not connectable", and indicates if the host can connect to
  // itself on its configured NetAddress.
  "connectabilitystatus": "checking",

  // workingstatus is one of "checking", "working", or "not working"
  // and indicates if the host is being actively used by renters.
  "workingstatus": "checking"
}
```

#### /host [POST]

configures hosting parameters. All parameters are optional; unspecified
parameters will be left unchanged.

###### Query String Parameters
```
// When set to true, the host will accept new file contracts if the
// terms are reasonable. When set to false, the host will not accept new
// file contracts at all.
acceptingcontracts // Optional, true / false

// The maximum size of a single download request from a renter. Each
// download request has multiple round trips of communication that
// exchange money. Larger batch sizes mean fewer round trips, but more
// financial risk for the host - the renter can get a free batch when
// downloading by refusing to provide a signature.
maxdownloadbatchsize // Optional, bytes

// The maximum duration of a file contract that the host will accept.
// The storage proof window must end before the current height +
// maxduration.
maxduration // Optional, blocks

// The maximum size of a single batch of file contract revisions. The
// renter can perform DoS attacks on the host by uploading a batch of
// data then refusing to provide a signature to pay for the data. The
// host can reduce this exposure by limiting the batch size. Larger
// batch sizes allow for higher throughput as there is significant
// communication overhead associated with performing a batch upload.
maxrevisebatchsize // Optional, bytes

// The IP address or hostname (including port) that the host should be
// contacted at. If left blank, the host will automatically figure out
// its ip address and use that. If given, the host will use the address
// given.
netaddress // Optional

// The storage proof window is the number of blocks that the host has
// to get a storage proof onto the blockchain. The window size is the
// minimum size of window that the host will accept in a file contract.
windowsize // Optional, blocks

// The maximum amount of money that the host will put up as collateral
// per byte per block of storage that is contracted by the renter.
collateral // Optional, hastings / byte / block

// The total amount of money that the host will allocate to collateral
// across all file contracts.
collateralbudget // Optional, hastings

// The maximum amount of collateral that the host will put into a
// single file contract.
maxcollateral // Optional, hastings

// The minimum price that the host will demand from a renter when
// forming a contract. Typically this price is to cover transaction
// fees on the file contract revision and storage proof, but can also
// be used if the host has a low amount of collateral. The price is a
// minimum because the host may automatically adjust the price upwards
// in times of high demand.
mincontractprice // Optional, hastings

// The minimum price that the host will demand from a renter when the
// renter is downloading data. If the host is saturated, the host may
// increase the price from the minimum.
mindownloadbandwidthprice // Optional, hastings / byte

// The minimum price that the host will demand when storing data for
// extended periods of time. If the host is low on space, the price of
// storage may be set higher than the minimum.
minstorageprice // Optional, hastings / byte / block

// The minimum price that the host will demand from a renter when the
// renter is uploading data. If the host is saturated, the host may
// increase the price from the minimum.
minuploadbandwidthprice // Optional, hastings / byte
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/announce [POST]

Announce the host to the network as a source of storage. Generally only needs 
to be called once.

Note that even after the host has been announced, it will not accept new 
contracts unless configured to do so. To configure the host to accept 
contracts, see [/host](https://github.com/NebulousLabs/Sia/blob/master/doc/api/Host.md#host-post).

###### Query String Parameters
```
// The address to be announced. If no address is provided, the automatically
// discovered address will be used instead.
netaddress string // Optional
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/contracts [GET]

Get contract information from the host database. This call will return all storage obligations on the host. Its up to the caller to filter the contracts based on his needs.

###### JSON Response
```javascript
{
  "contracts": [
    // Amount in hastings to cover the transaction fees for this storage obligation.
    "contractcost":		"1234",		// hastings

    // Size of the data that is protected by the contract.
    "datasize":			50000,		// bytes

    // Amount that is locked as collateral for this storage obligation.
    "lockedcollateral":		"1234",		// hastings

    // Id of the storageobligation, which is defined by the file contract id of the file contract that governs the storage obligation.
    "obligationid":		"fff48010dcbbd6ba7ffd41bc4b25a3634ee58bbf688d2f06b7d5a0c837304e13",

    // Potential revenue for downloaded data that the host will reveive upon successful completion of the obligation.
    "potentialdownloadrevenue":	"1234",		// hastings

    // Potential revenue for storage of data that the host will reveive upon successful completion of the obligation.
    "potentialstoragerevenue":	"1234",		// hastings

    // Potential revenue for uploaded data that the host will reveive upon successful completion of the obligation.
    "potentialuploadrevenue":	"1234",		// hastings

    // Amount that the host might lose if the submission of the storage proof is not successful.
    "riskedcollateral":		"1234",		// hastings

    // Number of sector roots.
    "sectorrootscount":		2,

    // Amount for transaction fees that the host added to the storage obligation.
    "transactionfeesadded":	"1234",		// hastings

    // Experation height is the height at which the storage obligation expires.
    "expirationheight":		123456,		// blocks

    // Negotion height is the height at which the storage obligation was negotiated.
    "negotiationheight":	0,		// blocks

    // The proof deadline is the height by which the storage proof must be submitted.
    "proofdeadline":		123456,		// blocks

    // Status of the storage obligation. There are 4 different statuses:
    // obligationFailed:	the storage obligation failed, potential revenues and risked collateral are lost
    // obligationRejected:	the storage obligation was never started, no revenues gained or lost
    // obligationSucceeded:	the storage obligation was completed, revenues were gained
    // obligationUnresolved: 	the storage obligation has an uninitialized value. When the "proofdeadline" is in the past this might be a stale obligation.
    "obligationstatus":		"obligationFailed",

    // Origin confirmed indicates whether the file contract was seen on the blockchain for this storage obligation.
    "originconfirmed":		true,

    // Proof confirmed indicates whether there was a storage proof seen on the blockchain for this storage obligation.
    "proofconfirmed":		true,

    // The host has constructed a storage proof
    "proofconstructed":		false
 
    // Revision confirmed indicates whether there was a file contract revision seen on the blockchain for this storage obligation.
    "revisionconfirmed":	true,
 
    // Revision constructed indicates whether there was a file contract revision constructed for this storage obligation.
    "revisionconstructed":	true,
 ]
}
```

#### /host/storage [GET]

gets a list of folders tracked by the host's storage manager.

###### JSON Response
```javascript
{
  "folders": [
    {
      // Absolute path to the storage folder on the local filesystem.
      "path": "/home/foo/bar",

      // Maximum capacity of the storage folder. The host will not store more
      // than this many bytes in the folder. This capacity is not checked
      // against the drive's remaining capacity. Therefore, you must manually
      // ensure the disk has sufficient capacity for the folder at all times.
      // Otherwise you risk losing renter's data and failing storage proofs.
      "capacity": 50000000000, // bytes

      // Unused capacity of the storage folder.
      "capacityremaining": 100000, // bytes

      // Number of failed disk read & write operations. A large number of
      // failed reads or writes indicates a problem with the filesystem or
      // drive's hardware.
      "failedreads":  0,
      "failedwrites": 1,

      // Number of successful read & write operations.
      "successfulreads":  2,
      "successfulwrites": 3
    }
  ]
}
```

#### /host/storage/folders/add [POST]

adds a storage folder to the manager. The manager may not check that there is
enough space available on-disk to support as much storage as requested

###### Query String Parameters
```
// Local path on disk to the storage folder to add.
path // Required

// Initial capacity of the storage folder. This value isn't validated so it is
// possible to set the capacity of the storage folder greater than the capacity
// of the disk. Do not do this.
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

###### Query String Parameters
```
// Local path on disk to the storage folder to remove.
path // Required

// If `force` is true, the storage folder will be removed even if the data in
// the storage folder cannot be moved to other storage folders, typically
// because they don't have sufficient capacity. If `force` is true and the data
// cannot be moved, data will be lost.
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

###### Query String Parameters
```
// Local path on disk to the storage folder to resize.
path // Required

// Desired new size of the storage folder. This will be the new capacity of the
// storage folder.
newsize // bytes, Required
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/storage/sectors/delete/___*merkleroot___ [POST]

deletes a sector, meaning that the manager will be unable to upload that sector
and be unable to provide a storage proof on that sector. This endpoint is for
removing the data entirely, and will remove instances of the sector appearing
at all heights. The primary purpose is to comply with legal requests to remove
data.

###### Path Parameters
```
// Merkleroot of the sector to delete.
:merkleroot 
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/estimatescore [GET]

returns the estimated HostDB score of the host using its current settings,
combined with the provided settings.

###### JSON Response
```javascript
{
	// estimatedscore is the estimated HostDB score of the host given the
	// settings passed to estimatescore.
	"estimatedscore": "123456786786786786786786786742133",
	// conversionrate is the likelihood given the settings passed to
	// estimatescore that the host will be selected by renters forming contracts.
	"conversionrate": 95
}
```

###### Query String Parameters
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

