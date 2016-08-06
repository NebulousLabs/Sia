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
	//
	// There is a second bottleneck, which is the sector map. There are some
	// optimizations in place which help, but occasionally the contract manager
	// will perform an operation that involves writing the entire sector map
	// for a storage folder to disk. A 4 million sector storage folder has a
	// storage map which is 80 MiB. The commit, while rare, will pause all
	// operations in the contract manager (due to the ACID requirements) until
	// the full 80 MiB can be synced to disk. On slower disks (and, most of the
	// disks are expected to be on the slower side), this can take several
	// seconds.
	//
	// The commit should be rare even when there are thousands of drives, as
	// they will usually all sync at the same time, causing a few seconds of
	// delay, but then providing minutes to hours before the next round of
	// syncing.
	maximumSectorsPerStorageFolder = func() uint64 {
		if build.Release == "dev" {
			return 1 << 20 // 4 TiB
		}
		if build.Release == "standard" {
			return 1 << 25 // 256 TiB
		}
		if build.Release == "testing" {
			return 1 << 8 // 1 MiB
		}
		panic("unrecognized release constant in host - maximum storage folder size")
	}()

	// minimumStorageFolderSize defines the smallest size that a storage folder
	// is allowed to be. The minimum is in place to help guide the network
	// health, and to keep out spammy hosts with very little storage. After Sia
	// has more maturity, the plan is to raise the production minimum storage
	// size from 32 GiB to 256 GiB.
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

	// storageFolderGranularity defines the number of sectors that a storage
	// folder must cleanly divide into. 8 sectors is a requirement due to the
	// way the storage folder bitfield (field 'Usage') is constructed - the
	// bitfield defines which sectors are available, and the bitfield must be
	// constructed 1 byte at a time.
	//
	// This corresponds to a granularity of 32 MiB on the production network,
	// which relative to the TiBs of storage that hosts are expected to
	// provide, is a large amount of granularity.
	storageFolderGranularity = uint64(8)
)
