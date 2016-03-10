package host

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// defaultMaxDuration defines the maximum number of blocks into the future
	// that the host will accept for the duration of an incoming file contract
	// obligation. 6 months is chosen because hosts are expected to be
	// long-term entities, and because we want to have a set of hosts that
	// support 6 month contracts when Sia leaves beta.
	defaultMaxDuration = 144 * 30 * 6 // 6 months.

	// maximumStorageFolders indicates the maximum number of storage folders
	// that the host allows. Some operations, such as creating a new storage
	// folder, take longer if there are more storage folders. Static RAM usage
	// also increases as the number of storage folders increase. For this
	// reason, a limit on the maximum number of storage folders has been set.
	maximumStorageFolders = 100

	// resubmissionTimeout defines the number of blocks that a host will wait
	// before attempting to resubmit a transaction to the blockchain.
	// Typically, this transaction will contain either a file contract, a file
	// contract revision, or a storage proof.
	resubmissionTimeout = 3
)

var (
	// defaultContractPrice defines the default price of creating a contract
	// with the host. The default is set to 50 siacoins, which means that the
	// opening file contract can have 5 siacoins put towards it, the file
	// contract revision can have 15 siacoins put towards it, and the storage
	// proof can have 15 siacoins put towards it, with 5 left over for the
	// host, a sort of compensation for keeping and backing up the storage
	// obligation data.
	defaultContractPrice = types.NewCurrency64(40).Mul(types.SiacoinPrecision) // 40 siacoins

	// defaultUploadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 1 siacoin per GB, because the host is
	// presumed to have a large amount of downstream bandwidth. Furthermore,
	// the host is typically only downloading data if it is planning to store
	// the data, meaning that the host serves to profit from accepting the
	// data.
	defaultUploadBandwidthPrice = modules.BandwidthPriceToConsensus(1e3) // 1 SC / GB

	// defaultDownloadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 10 siacoins per gigabyte, because
	// download bandwidth is expected to be plentiful but also in-demand.
	defaultDownloadBandwidthPrice = modules.BandwidthPriceToConsensus(10e3) // 10 SC / GB

	// defaultStoragePrice defines the starting price for hosts selling
	// storage. We try to match a number that is both reasonably profitable and
	// reasonably competitive.
	defaultStoragePrice = modules.StoragePriceToConsensus(20e3) // 20 SC / GB / Month

	// defaultCollateral defines the amount of money that the host puts up as
	// collateral per-byte by default. The collateral should be considered as
	// an absolute instead of as a percentage, because low prices result in
	// collaterals which may be significant by percentage, but insignificant
	// overall. A default of 50 SC / GB / Month has been chosen, which is 2.5x
	// the default price for storage. The host is expected to put up a
	// significant amount of collateral as a commitment to faithfulness,
	// because this guarantees that the incentives are aligned for the host to
	// keep the data even if the price of siacoin fluctuates, the price of raw
	// storage fluctuates, or the host realizes that there is unexpected
	// opportunity cost in being a host.
	defaultCollateral = types.NewCurrency64(50) // 50 SC / GB / Month

	// defaultWindowSize is the size of the proof of storage window requested
	// by the host. The host will not delete any obligations until the window
	// has closed and buried under several confirmations. For release builds,
	// the default is set to 144 blocks, or about 1 day. This gives the host
	// flexibility to experience downtime without losing file contracts. The
	// optimal default, especially as the network matures, is probably closer
	// to 36 blocks. An experienced or high powered host should not be
	// frustrated by lost coins due to long periods of downtime.
	defaultWindowSize = func() types.BlockHeight {
		if build.Release == "dev" {
			return 36 // 3.6 minutes.
		}
		if build.Release == "standard" {
			return 144 // 1 day.
		}
		if build.Release == "testing" {
			return 5 // 5 seconds.
		}
		panic("unrecognized release constant in host - defaultWindowSize")
	}()

	// maximumLockedStorageObligations sets the maximum number of storage
	// obligations that are allowed to be locked at a time. The map uses an
	// in-memory lock, but also a locked storage obligation could be reading a
	// whole sector into memory, which could use a bunch of system resources.
	maximumLockedStorageObligations = func() uint64 {
		if build.Release == "dev" {
			return 20
		}
		if build.Release == "standard" {
			return 100
		}
		if build.Release == "testing" {
			return 5
		}
		panic("unrecognized release constant in host - maximumLockedStorageObligations")
	}()

	// maximumStorageFolderSize sets an upper bound on how large storage
	// folders in the host are allowed to be. It makes sure that inputs and
	// constructions are sane. While it's conceivable that someone could create
	// a rig with a single logical storage folder greater than 128 TiB in size
	// in production, it's probably not a great idea, especially when you are
	// allowed to use many storage folders. All told, a single host on today's
	// constants can support up to ~10 PB of storage.
	maximumStorageFolderSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 40 // 1 TiB
		}
		if build.Release == "standard" {
			return 1 << 50 // 1 PiB
		}
		if build.Release == "testing" {
			return 1 << 20 // 1 MiB
		}
		panic("unrecognized release constant in host - maximum storage folder size")
	}()

	// maximumVirtualSectors defines the maximum number of virtual sectors that
	// can be tied to each physical sector.
	maximumVirtualSectors = func() int {
		if build.Release == "dev" {
			// The testing value is at 35 to provide flexibility. The
			// development value is at 5 because hitting the virtual sector
			// limit in a sane development environment is more difficult than
			// hitting the virtual sector limit in a controlled testing
			// environment (dev environment doesn't have access to private
			// methods such as 'addSector'.
			return 5
		}
		if build.Release == "standard" {
			// Each virtual sector adds about 8 bytes of load to the host
			// persistence structures, and additionally adds 8 bytes of load
			// when reading or modifying a sector. Though a few virtual sectors
			// with 10e3 or even 100e3 virtual sectors would not be too
			// detrimental to the host, tens of thousands of physical sectors
			// that each have ten thousand virtual sectors could pose a problem
			// for the host. In most situations, a renter will not need more 2
			// or 3 virtual sectors when manipulating data, so 250 is generous
			// as long as the renter is properly encrypting data. 250 is
			// unlikely to cause the host problems, even if an attacker is
			// creating hundreds of thousands of phsyical sectors (an expensive
			// action!) each with 250 vitrual sectors.
			return 250
		}
		if build.Release == "testing" {
			return 35
		}
		panic("unrecognized release constant in host - maximum virtual sector size")
	}()

	// minimumStorageFolderSize defines the smallest size that a storage folder
	// is allowed to be. The new design of the storage folder structure means
	// that this limit is not as relevant as it was originally, but hosts with
	// little storage capacity are not very useful to the network, and can
	// actually frustrate price planning. 32 GB has been chosen as a minimum
	// for the early days of the network, to allow people to experiment in the
	// beta, but in the future I think something like 256GB would be much more
	// appropraite.
	minimumStorageFolderSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 25 // 32 MiB
		}
		if build.Release == "standard" {
			return 1 << 35 // 32 GiB
		}
		if build.Release == "testing" {
			return 1 << 15 // 32 KiB
		}
		panic("unrecognized release constant in host - minimum storage folder size")
	}()

	// revisionSubmissionBuffer describes the number of blocks ahead of time
	// that the host will submit a file contract revision. The host will not
	// accept any more revisions once inside the submission buffer.
	revisionSubmissionBuffer = func() types.BlockHeight {
		if build.Release == "dev" {
			return 20 // About 2 minutes
		}
		if build.Release == "standard" {
			return 288 // 2 days.
		}
		if build.Release == "testing" {
			return 4
		}
		panic("unrecognized release constant in host - revision submission buffer")
	}()

	// storageFolderUIDSize determines the number of bytes used to determine
	// the storage folder UID. Production and development environments use 4
	// bytes to minimize the possibility of accidental collisions, and testing
	// environments use 1 byte so that collisions can be forced while using the
	// live code.
	storageFolderUIDSize = func() int {
		if build.Release == "dev" {
			return 2
		}
		if build.Release == "standard" {
			return 4
		}
		if build.Release == "testing" {
			return 1
		}
		panic("unrecognized release constant in host - storageFolderUIDSize")
	}()

	// storageProofConfirmations determines the number of confirmations for a
	// storage proof that the host will wait before
	storageProofConfirmations = func() int {
		if build.Release == "dev" {
			return 20 // About 2 minutes
		}
		if build.Release == "standard" {
			return 72 // About 12 hours
		}
		if build.Release == "testing" {
			return 3
		}
		panic("unrecognized release constant in host - storageProofConfirmations")
	}()
)

// All of the following variables define the names of buckets used by the host
// in the database.
var (
	// bucketActionItems maps a blockchain height to a list of storage
	// obligations that need to be managed in some way at that height. The
	// height is stored as a big endian uint64, which means that bolt will
	// store the heights sorted in numerical order. The action item itself is
	// an array of file contract ids. The host is able to contextually figure
	// out what the necessary actions for that item are based on the file
	// contract id and the associated storage obligation that can be retreived
	// using the id.
	bucketActionItems = []byte("BucketActionItems")

	// bucketSectorUsage maps sector IDs to the number of times they are used
	// in file contracts. If all data is correctly encrypted using a unique
	// seed, each sector will be in use exactly one time. The host however
	// cannot control this, and a user may upload unencrypted data or
	// intentionally upload colliding sectors as a means of attack. The host
	// can only delete a sector when it is in use zero times. The number of
	// times a sector is in use is encoded as a big endian uint64.
	bucketSectorUsage = []byte("BucketSectorUsage")

	// bucketStorageObligations contains a set of serialized
	// 'storageObligations' sorted by their file contract id.
	bucketStorageObligations = []byte("BucketStorageObligations")
)
