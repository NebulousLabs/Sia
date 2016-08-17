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

| Route                                                                                 | HTTP verb |
| ------------------------------------------------------------------------------------- | --------- |
| [/host](#host-get)                                                                    | GET       |
| [/host](#host-post)                                                                   | POST      |
| [/host/announce](#hostannounce-post)                                                  | POST      |
| [/host/delete/___:filecontractid___](#hostdeletefilecontractid-post)                  | POST      |
| [/host/storage](#hoststorage-get)                                                     | GET       |
| [/host/storage/folders/add](#hoststoragefoldersadd-post)                              | POST      |
| [/host/storage/folders/remove](#hoststoragefoldersremove-post)                        | POST      |
| [/host/storage/folders/resize](#hoststoragefoldersresize-post)                        | POST      |
| [/host/storage/sectors/delete/___:merkleroot___](#hoststoragesectorsdeletemerkleroot) | POST      |

#### /host [GET]

fetches status information about the host.

// TODO: convert to example JSON response and add units.
###### JSON Response
```go
struct {
	// The settings that get displayed to untrusted nodes querying the host's
	// status.
	externalsettings {
		// Whether or not the host is accepting new contracts.
		acceptingcontracts bool

		// The maximum size of a single download request from a renter. Each
		// download request has multiple round trips of communication that
		// exchange money. Larger batch sizes mean fewer round trips, but more
		// financial risk for the host - the renter can get a free batch when
		// downloading by refusing to provide a signature.
		maxdownloadbatchsize uint64

		// The maximum duration that a host will allow for a file contract. The
		// host commits to keeping files for the full duration under the threat
		// of facing a large penalty for losing or dropping data before the
		// duration is complete. The storage proof window of an incoming file
		// contract must end before the current height + maxduration.
		maxduration types.BlockHeight (uint64)

		// The maximum size of a single batch of file contract revisions. The
		// renter can perform DoS attacks on the host by uploading a batch of
		// data then refusing to provide a signature to pay for the data. The
		// host can reduce this exposure by limiting the batch size. Larger
		// batch sizes allow for higher throughput as there is significant
		// communication overhead associated with performing a batch upload.
		maxrevisebatchsize uint64

		// The IP address or hostname (including port) that the host should be
		// contacted at.
		netaddress modules.NetAddress (string)

		// The amount of unused storage capacity on the host in bytes. It
		// should be noted that the host can lie.
		remainingstorage uint64

		// The smallest amount of data in bytes that can be uploaded or
		// downloaded when performing calls to the host.
		sectorsize uint64

		// The total amount of storage capacity on the host. It should be noted
		// that the host can lie.
		totalstorage uint64

		// The unlock hash is the address at which the host can be paid when
		// forming file contracts.
		unlockhash types.UnlockHash (string)

		// The storage proof window is the number of blocks that the host has
		// to get a storage proof onto the blockchain. The window size is the
		// minimum size of window that the host will accept in a file contract.
		windowsize types.BlockHeight (uint64)

		// The maximum amount of money that the host will put up as collateral
		// for storage that is contracted by the renter.
		//
		// The unit is hastings per byte per block.
		collateral types.Currency (string)

		// The maximum amount of collateral that the host will put into a
		// single file contract.
		//
		// The unit is hastings.
		maxcollateral types.Currency (string)

		// The price that a renter has to pay to create a contract with the
		// host. The payment is intended to cover transaction fees
		// for the file contract revision and the storage proof that the host
		// will be submitting to the blockchain.
		//
		// The unit is hastings per contract.
		contractprice types.Currency (string)

		// The price that a renter has to pay when downloading data from the
		// host.
		//
		// The unit is hastings per byte.
		downloadbandwidthprice types.Currency (string)

		// The price that a renter has to pay to store files with the host.
		//
		// The unit is hastings per byte per block.
		storageprice types.Currency (string)

		// The price that a renter has to pay when uploading data to the host.
		//
		// The unit is hastings per byte.
		uploadbandwidthprice types.Currency (string)

		// The revision number indicates to the renter what iteration of
		// settings the host is currently at. Settings are generally signed.
		// If the renter has multiple conflicting copies of settings from the
		// host, the renter can expect the one with the higher revision number
		// to be more recent.
		revisionnumber uint64

		// The version of external settings being used. This field helps
		// coordinate updates while preserving compatibility with older nodes.
		version string
	}

	// The financial status of the host.
	financialmetrics {
		// The amount of money that renters have given to the host to pay for
		// file contracts. The host is required to submit a file contract
		// revision and a storage proof for every file contract that gets created,
		// and the renter pays for the miner fees on these objects.
		//
		// The unit is hastings.
		contractcompensation types.Currency (string)

		// The amount of money that renters have given to the host to pay for
		// file contracts which have not been confirmed yet. The potential
		// compensation becomes compensation after the storage proof is
		// submitted.
		//
		// The unit is hastings.
		potentialcontractcompensation types.Currency (string)

		// The amount of storage collateral which the host has tied up in file
		// contracts. The host has to commit collateral to a file contract even
		// if there is no storage, but the locked collateral will be returned
		// even if the host does not submit a storage proof - the collateral is
		// not at risk, it is merely set aside so that it can be put at risk
		// later.
		//
		// The unit is hastings.
		lockedstoragecollateral types.Currency (string)

		// The amount of revenue, including storage revenue and bandwidth
		// revenue, that has been lost due to failed file contracts and
		// failed storage proofs.
		//
		// The unit is hastings.
		lostrevenue types.Currency (string)

		// The amount of collateral that was put up to protect data which has
		// been lost due to failed file contracts and missed storage proofs.
		//
		// The unit is hastings.
		loststoragecollateral types.Currency (string)

		// The amount of revenue that the host stands to earn if all storage
		// proofs are submitted corectly and in time.
		//
		// The unit is hastings.
		potentialstoragerevenue types.Currency (string)

		// The amount of money that the host has risked on file contracts. If
		// the host starts missing storage proofs, the host can forfeit up to
		// this many coins. In the event of a missed storage proof, locked
		// storage collateral gets returned, but risked storage collateral
		// does not get returned.
		//
		// The unit is hastings.
		riskedstoragecollateral types.Currency (string)

		// The amount of money that the host has earned from storing data. This
		// money has been locked down by successful storage proofs.
		//
		// The unit is hastings.
		storagerevenue types.Currency (string)

		// The amount of money that the host has spent on transaction fees when
		// submitting host announcements, file contract revisions, and storage
		// proofs.
		//
		// The unit is hastings.
		transactionfeeexpenses types.Currency (string)

		// The amount of money that the host has made from renters downloading
		// their files. This money has been locked in by successsful storage
		// proofs.
		//
		// The unit is hastings.
		downloadbandwidthrevenue types.Currency (string)

		// The amount of money that the host stands to make from renters that
		// downloaded their files. The host will only realize this revenue if
		// the host successfully submits storage proofs for the related file
		// contracts.
		//
		// The unit is hastings.
		potentialdownloadbandwidthrevenue types.Currency (string)

		// The amount of money that the host stands to make from renters that
		// uploaded files. The host will only realize this revenue if the host
		// successfully submits storage proofs for the related file contracts.
		//
		// The unit is hastings.
		potentialuploadbandwidthrevenue types.Currency (string)

		// The amount of money that the host has made from renters uploading
		// their files. This money has been locked in by successful storage
		// proofs.
		uploadbandwidthrevenue types.Currency (string)
	}

	// The settings of the host. Most interactions between the user and the
	// host occur by changing the internal settings.
	internalsettings {
		// When set to true, the host will accept new file contracts if the
		// terms are reasonable. When set to false, the host will not accept new
		// file contracts at all.
		acceptingcontracts bool

		// The maximum size of a single download request from a renter. Each
		// download request has multiple round trips of communication that
		// exchange money. Larger batch sizes mean fewer round trips, but more
		// financial risk for the host - the renter can get a free batch when
		// downloading by refusing to provide a signature.
		maxdownloadbatchsize uint64

		// The maximum duration of a file contract that the host will accept.
		// The storage proof window must end before the current height +
		// maxduration.
		maxduration types.BlockHeight (uint64)

		// The maximum size of a single batch of file contract revisions. The
		// renter can perform DoS attacks on the host by uploading a batch of
		// data then refusing to provide a signature to pay for the data. The
		// host can reduce this exposure by limiting the batch size. Larger
		// batch sizes allow for higher throughput as there is significant
		// communication overhead associated with performing a batch upload.
		maxrevisebatchsize uint64

		// The IP address or hostname (including port) that the host should be
		// contacted at. If left blank, the host will automatically figure out
		// its ip address and use that. If given, the host will use the address
		// given.
		netaddress modules.NetAddress (string)

		// The storage proof window is the number of blocks that the host has
		// to get a storage proof onto the blockchain. The window size is the
		// minimum size of window that the host will accept in a file contract.
		windowsize types.BlockHeight (uint64)

		// The maximum amount of money that the host will put up as collateral
		// per byte per block of storage that is contracted by the renter.
		//
		// The unit is hastings per byte per block.
		collateral types.Currency (string)

		// The total amount of money that the host will allocate to collateral
		// across all file contracts.
		//
		// The unit is hastings.
		collateralbudget types.Currency (string)

		// The maximum amount of collateral that the host will put into a
		// single file contract.
		//
		// The unit is hastings.
		maxcollateral types.Currency (string)

		// The minimum price that the host will demand from a renter when
		// forming a contract. Typically this price is to cover transaction
		// fees on the file contract revision and storage proof, but can also
		// be used if the host has a low amount of collateral. The price is a
		// minimum because the host may automatically adjust the price upwards
		// in times of high demand.
		//
		// The unit is hastings.
		mincontractprice types.Currency (string)

		// The minimum price that the host will demand from a renter when the
		// renter is downloading data. If the host is saturated, the host may
		// increase the price from the minimum.
		//
		// The unit is hastings per byte.
		mindownloadbandwidthprice types.Currency (string)

		// The minimum price that the host will demand when storing data for
		// extended periods of time. If the host is low on space, the price of
		// storage may be set higher than the minimum.
		//
		// The unit is hastings per byte per block.
		minstorageprice types.Currency (string)

		// The minimum price that the host will demand from a renter when the
		// renter is uploading data. If the host is saturated, the host may
		// increase the price from the minimum.
		//
		// The unit is hastings per byte.
		minuploadbandwidthprice types.Currency (string)
	}

	// Information about the network, specifically various ways in which
	// renters have contacted the host.
	networkmetrics {
		// The number of times that a renter has attempted to download
		// something from the host.
		downloadcalls uint64

		// The number of calls that have resulted in errors. A small number of
		// errors are expected, but a large number of errors indicate either
		// buggy software or malicious network activity. Usually buggy
		// software.
		errorcalls uint64

		// The number of times that a renter has tried to form a contract with
		// the host.
		formcontractcalls uint64

		// The number of times that a renter has tried to renew a contract with
		// the host.
		renewcalls uint64

		// The number of times that the renter has tried to revise a contract
		// with the host.
		revisecalls uint64

		// The number of times that a renter has queried the host for the
		// host's settings. The settings include the price of bandwidth, which
		// is a price that can adjust every few minutes. This value is usually
		// very high compared to the others.
		settingscalls uint64

		// The number of times that a renter has attempted to use an
		// unrecognized call. Larger numbers typically indicate buggy software.
		unrecognizedcalls uint64
	}
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
acceptingcontracts bool // Optional

// The maximum size of a single download request from a renter. Each
// download request has multiple round trips of communication that
// exchange money. Larger batch sizes mean fewer round trips, but more
// financial risk for the host - the renter can get a free batch when
// downloading by refusing to provide a signature.
maxdownloadbatchsize uint64 // Optional

// The maximum duration of a file contract that the host will accept.
// The storage proof window must end before the current height +
// maxduration.
maxduration types.BlockHeight (uint64) // Optional

// The maximum size of a single batch of file contract revisions. The
// renter can perform DoS attacks on the host by uploading a batch of
// data then refusing to provide a signature to pay for the data. The
// host can reduce this exposure by limiting the batch size. Larger
// batch sizes allow for higher throughput as there is significant
// communication overhead associated with performing a batch upload.
maxrevisebatchsize uint64 // Optional

// The IP address or hostname (including port) that the host should be
// contacted at. If left blank, the host will automatically figure out
// its ip address and use that. If given, the host will use the address
// given.
netaddress modules.NetAddress (string) // Optional

// The storage proof window is the number of blocks that the host has
// to get a storage proof onto the blockchain. The window size is the
// minimum size of window that the host will accept in a file contract.
windowsize types.BlockHeight (uint64) // Optional

// The maximum amount of money that the host will put up as collateral
// per byte per block of storage that is contracted by the renter.
//
// The unit is hastings per byte per block.
collateral types.Currency (string) // Optional

// The total amount of money that the host will allocate to collateral
// across all file contracts.
//
// The unit is hastings.
collateralbudget types.Currency (string) // Optional

// The maximum amount of collateral that the host will put into a
// single file contract.
//
// The unit is hastings.
maxcollateral types.Currency (string) // Optional

// The minimum price that the host will demand from a renter when
// forming a contract. Typically this price is to cover transaction
// fees on the file contract revision and storage proof, but can also
// be used if the host has a low amount of collateral. The price is a
// minimum because the host may automatically adjust the price upwards
// in times of high demand.
//
// The unit is hastings..
mincontractprice types.Currency (string) // Optional

// The minimum price that the host will demand from a renter when the
// renter is downloading data. If the host is saturated, the host may
// increase the price from the minimum.
//
// The unit is hastings per byte.
mindownloadbandwidthprice types.Currency (string) // Optional

// The minimum price that the host will demand when storing data for
// extended periods of time. If the host is low on space, the price of
// storage may be set higher than the minimum.
//
// The unit is hastings per byte per block.
minstorageprice types.Currency (string) // Optional

// The minimum price that the host will demand from a renter when the
// renter is uploading data. If the host is saturated, the host may
// increase the price from the minimum.
//
// The unit is hastings per byte.
minuploadbandwidthprice types.Currency (string) // Optional
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /host/announce [POST]

Announce the host to the network as a source of storage. Generally only needs
to be called once.

###### Query String Parameters
```
// The address to be announced. If no address is provided, the automatically
// discovered address will be used instead.
netaddress string // Optional
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).
