package contractmanager

import (
	"time"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/persist"
)

const (
	// logFile is the name of the file that is used for logging in the contract
	// manager.
	logFile = "contractmanager.log"

	// metadataFile is the name of the file that stores all of the sector
	// metadata associated with a storage folder.
	metadataFile = "siahostmetadata.dat"

	// sectorFile is the file that is placed inside of a storage folder to
	// house all of the sectors associated with a storage folder.
	sectorFile = "siahostdata.dat"

	// settingsFile is the name of the file that is used to save the contract
	// manager's settings.
	settingsFile = "contractmanager.json"

	// settingsFileTmp is the name of the file that is used to hold unfinished
	// writes to the contract manager's settings. After this file is completed,
	// a copy-on-write operation is performed to make sure that the contract
	// manager's persistent settings are updated atomically.
	settingsFileTmp = "contractmanager.json_temp"

	// walFile is the name of the file that is used to save the write ahead log
	// for the contract manager.
	walFile = "contractmanager.wal"

	// walFileTmp is used for incomplete writes to the WAL. Data could be
	// interrupted by power outages, etc., and is therefore written to a
	// temporary file before being atomically renamed to the correct name.
	walFileTmp = "contractmanager.wal_temp"
)

const (
	// folderAllocationStepSize is the amount of data that gets allocated at a
	// time when writing out the sparse sector file during a storageFolderAdd or
	// a storageFolderGrow.
	folderAllocationStepSize = 1 << 35

	// maxSectorBatchThreads is the maximum number of threads updating
	// sector counters on disk in AddSectorBatch and RemoveSectorBatch.
	maxSectorBatchThreads = 100

	// sectorMetadataDiskSize defines the number of bytes it takes to store the
	// metadata of a single sector on disk.
	sectorMetadataDiskSize = 14

	// storageFolderGranularity defines the number of sectors that a storage
	// folder must cleanly divide into. 64 sectors is a requirement due to the
	// way the storage folder bitfield (field 'Usage') is constructed - the
	// bitfield defines which sectors are available, and the bitfield must be
	// constructed 1 uint64 at a time (8 bytes, 64 bits, or 64 sectors).
	//
	// This corresponds to a granularity of 256 MiB on the production network,
	// which is a high granluarity relative the to the TiBs of storage that
	// hosts are expected to provide.
	storageFolderGranularity = 64
)

var (
	// settingsMetadata is the header that is used when writing the contract
	// manager's settings to disk.
	settingsMetadata = persist.Metadata{
		Header:  "Sia Contract Manager",
		Version: "1.2.0",
	}

	// walMetadata is the header that is used when writing the write ahead log
	// to disk, so that it may be identified at startup.
	walMetadata = persist.Metadata{
		Header:  "Sia Contract Manager WAL",
		Version: "1.2.0",
	}
)

var (
	// MaximumSectorsPerStorageFolder sets an upper bound on how large storage
	// folders in the host are allowed to be. There is a hard limit at 4
	// billion sectors because the sector location map only uses 4 bytes to
	// indicate the location of a sector.
	MaximumSectorsPerStorageFolder = build.Select(build.Var{
		Dev:      uint64(1 << 20), // 256 GiB
		Standard: uint64(1 << 32), // 16 PiB
		Testing:  uint64(1 << 12), // 16 MiB
	}).(uint64)

	// maximumStorageFolders defines the maximum number of storage folders that
	// the host can support.
	maximumStorageFolders = build.Select(build.Var{
		Dev:      uint64(1 << 5),
		Standard: uint64(1 << 16),
		Testing:  uint64(1 << 3),
	}).(uint64)

	// MinimumSectorsPerStorageFolder defines the minimum number of sectors
	// that a storage folder is allowed to have.
	MinimumSectorsPerStorageFolder = build.Select(build.Var{
		Dev:      uint64(1 << 6), // 16 MiB
		Standard: uint64(1 << 6), // 256 MiB
		Testing:  uint64(1 << 6), // 256 KiB
	}).(uint64)
)

var (
	// folderRecheckInitialInterval specifies the amount of time that the
	// contract manager will initially wait when checking to see if an
	// unavailable storage folder has become available.
	folderRecheckInitialInterval = build.Select(build.Var{
		Dev:      time.Second,
		Standard: time.Second * 5,
		Testing:  time.Second,
	}).(time.Duration)

	// maxFolderRecheckInterval specifies the maximum amount of time that the
	// contract manager will wait between checking if an unavailable storage
	// folder has become available.
	maxFolderRecheckInterval = build.Select(build.Var{
		Dev:      time.Second * 30,
		Standard: time.Second * 60 * 5,
		Testing:  time.Second * 8,
	}).(time.Duration)
)
