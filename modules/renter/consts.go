package renter

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	// defaultMemory establishes the default amount of memory that the renter
	// will use when performing uploads and downloads. The mapping is currently
	// not perfect due to GC overhead and other places where we don't count all
	// of the memory usage accurately.
	defaultMemory = build.Select(build.Var{
		Dev:      uint64(1 << 28),     // 256 MiB
		Standard: uint64(3 * 1 << 28), // 768 MiB
		Testing:  uint64(1 << 17),     // 128 KiB - 4 KiB sector size, need to test memory exhaustion
	}).(uint64)
)

var (
	// defaultDataPieces is the number of data pieces per erasure-coded chunk
	defaultDataPieces = func() int {
		switch build.Release {
		case "dev":
			return 1
		case "standard":
			return 10
		case "testing":
			return 1
		}
		panic("undefined defaultDataPieces")
	}()

	// defaultParityPieces is the number of parity pieces per erasure-coded
	// chunk
	defaultParityPieces = func() int {
		switch build.Release {
		case "dev":
			return 1
		case "standard":
			return 20
		case "testing":
			return 8
		}
		panic("undefined defaultParityPieces")
	}()

	// Erasure-coded piece size
	pieceSize = modules.SectorSize - crypto.TwofishOverhead
)

const (
	// defaultFilePerm defines the default permissions used for a new file if no
	// permissions are supplied.
	defaultFilePerm = 0666

	// downloadFailureCooldown defines how long to wait for a worker after a
	// worker has experienced a download failure.
	downloadFailureCooldown = time.Second * 3

	// memoryPriorityLow is used to request low priority memory
	memoryPriorityLow = false

	// memoryPriorityHigh is used to request high priority memory
	memoryPriorityHigh = true

	// destinationTypeSeekStream is the destination type used for downloads
	// from the /renter/stream endpoint.
	destinationTypeSeekStream = "httpseekstream"

	// downloadCacheSize is the cache size of the /renter/stream cache in
	// chunks.
	downloadCacheSize = 2
)

var (
	// chunkDownloadTimeout defines the maximum amount of time to wait for a
	// chunk download to finish before returning in the download-to-upload repair
	// loop
	chunkDownloadTimeout = build.Select(build.Var{
		Dev:      15 * time.Minute,
		Standard: 15 * time.Minute,
		Testing:  1 * time.Minute,
	}).(time.Duration)

	// maxConsecutivePenalty determines how many times the timeout/cooldown for
	// being a bad host can be doubled before a maximum cooldown is reached.
	maxConsecutivePenalty = build.Select(build.Var{
		Dev:      4,
		Standard: 10,
		Testing:  3,
	}).(int)

	// maxScheduledDownloads specifies the number of chunks that can be downloaded
	// for auto repair at once. If the limit is reached new ones will only be scheduled
	// once old ones are scheduled for upload
	maxScheduledDownloads = build.Select(build.Var{
		Dev:      5,
		Standard: 10,
		Testing:  5,
	}).(int)

	// offlineCheckFrequency is how long the renter will wait to check the
	// online status if it is offline.
	offlineCheckFrequency = build.Select(build.Var{
		Dev:      3 * time.Second,
		Standard: 10 * time.Second,
		Testing:  250 * time.Millisecond,
	}).(time.Duration)

	// rebuildChunkHeapInterval defines how long the renter sleeps between
	// checking on the filesystem health.
	rebuildChunkHeapInterval = build.Select(build.Var{
		Dev:      90 * time.Second,
		Standard: 15 * time.Minute,
		Testing:  3 * time.Second,
	}).(time.Duration)

	// RemoteRepairDownloadThreshold defines the threshold in percent under
	// which the renter starts repairing a file that is not available on disk.
	RemoteRepairDownloadThreshold = build.Select(build.Var{
		Dev:      0.25,
		Standard: 0.25,
		Testing:  0.25,
	}).(float64)

	// Prime to avoid intersecting with regular events.
	uploadFailureCooldown = build.Select(build.Var{
		Dev:      time.Second * 7,
		Standard: time.Second * 61,
		Testing:  time.Second,
	}).(time.Duration)

	// workerPoolUpdateTimeout is the amount of time that can pass before the
	// worker pool should be updated.
	workerPoolUpdateTimeout = build.Select(build.Var{
		Dev:      30 * time.Second,
		Standard: 5 * time.Minute,
		Testing:  3 * time.Second,
	}).(time.Duration)
)
