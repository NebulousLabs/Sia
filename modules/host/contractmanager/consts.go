package contractmanager

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	// logFile is the name of the file that is used for logging in the contract
	// manager.
	logFile = "contractmanager.log"

	// settingsFile is the name of the file that is used to save the contract
	// manager's settings.
	settingsFile = "contractmanager.json"

	// sectorFile is the file that is placed inside of a storage folder to
	// house all of the sectors and sector metadata associated with a storage
	// folder.
	sectorFile = "siahostdata.dat"

	// walFile is the name of the file that is used to save the write ahead log
	// for the contract manager.
	walFile = "contractmanager.wal"

	// walFileTmp is used for incomplete writes to the WAL. Data could be
	// interrupted by power outages, etc., and is therefore written to a
	// temporary file before being atomically renamed to the correct name.
	walFileTmp = "contractmanager.wal.tmp"
)

var (
	// settingsMetadata is the header that is used when writing the contract
	// manager's settings to disk.
	settingsMetadata = persist.Metadata{
		Header:  "Sia Contract Manager",
		Version: "1.0.2",
	}

	// walMetadata is the header that is used when writing the write ahead log
	// to disk, so that it may be identified at startup.
	walMetadata = persist.Metadata{
		Header:  "Sia Contract Manager WAL",
		Version: "1.0.2",
	}
)

var (
	// maximumStorageFolders defines the maximum number of storage folders that
	// the host can support.
	maximumStorageFolders = func() uint64 {
		if build.Release == "dev" {
			return 1 << 5
		}
		if build.Release == "standard" {
			return 1 << 16
		}
		if build.Release == "testing" {
			return 1 << 3
		}
		panic("unrecognized release constant in host - maximum storage folders")
	}()

	// maximumSectorsPerStorageFolder sets an upper bound on how large storage
	// folders in the host are allowed to be. There is a hard limit at 4
	// billion sectors because the sector location map only uses 4 bytes to
	// indicate the location of a sector.
	//
	// On slower machines, it takes around 4ms to scan the bitfield for a
	// completely full 4 million sector bitfield, at which point the bitfield
	// becomes a limiting performance factor when adding a new sector.
	maximumSectorsPerStorageFolder = func() uint64 {
		if build.Release == "dev" {
			return 1 << 20 // 4 TiB
		}
		if build.Release == "standard" {
			return 1 << 25 // 256 TiB
		}
		if build.Release == "testing" {
			return 1 << 12 // 16 MiB
		}
		panic("unrecognized release constant in host - maximum storage folder size")
	}()

	// minimumSectorsPerStorageFolder defines the minimum number of sectors
	// that a storage folder is allowed to have. The minimum has been set as a
	// guide to assist with network health, and to help discourage spammy hosts
	// with very little storage. Even if the spammy hosts were allowed, they
	// would be ignored, but the blockchain would still clutter with their
	// announcements and users may fall into the trap of thinking that such
	// small volumes of storage are worthwhile.
	//
	// There are plans to continue raising the minimum storage requirements as
	// the network gains maturity.
	minimumSectorsPerStorageFolder = func() uint64 {
		if build.Release == "dev" {
			return 1 << 3 // 32 MiB
		}
		if build.Release == "standard" {
			return 1 << 12 // 32 GiB
		}
		if build.Release == "testing" {
			return 1 << 3 // 32 KiB
		}
		panic("unrecognized release constant in host - minimum storage folder size")
	}()

	// storageFolderGranularity defines the number of sectors that a storage
	// folder must cleanly divide into. 32 sectors is a requirement due to the
	// way the storage folder bitfield (field 'Usage') is constructed - the
	// bitfield defines which sectors are available, and the bitfield must be
	// constructed 1 uint32 at a time (4 bytes, 32 bits, or 32 sectors).
	//
	// This corresponds to a granularity of 32 MiB on the production network,
	// which relative to the TiBs of storage that hosts are expected to
	// provide, is a large amount of granularity.
	storageFolderGranularity = uint64(32)
)
