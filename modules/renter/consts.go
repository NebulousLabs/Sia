package renter

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

var (
	// Prime to avoid intersecting with regular events.
	uploadFailureCooldown = build.Select(build.Var{
		Dev:      time.Second * 7,
		Standard: time.Second * 61,
		Testing:  time.Second,
	}).(time.Duration)

	// Limit the number of doublings to prevent overflows.
	maxConsecutivePenalty = build.Select(build.Var{
		Dev:      4,
		Standard: 10,
		Testing:  3,
	}).(int)

	// Minimum number of pieces that need to be repaired before the renter will
	// initiate a repair.
	minPiecesRepair = build.Select(build.Var{
		Dev:      2,
		Standard: 5,
		Testing:  3,
	}).(int)

	repairQueueInterval = build.Select(build.Var{
		Dev:      30 * time.Second,
		Standard: time.Minute * 15,
		Testing:  10 * time.Second,
	}).(time.Duration)

	// maxChunkCacheSize determines the maximum number of chunks that will be
	// cached in memory.
	maxChunkCacheSize = build.Select(build.Var{
		Dev:      10,
		Standard: 10,
		Testing:  5,
	}).(int)

	// maxScheduledDownloads specifies the number of chunks that can be downloaded
	// for auto repair at once. If the limit is reached new ones will only be scheduled
	// once old ones are scheduled for upload
	maxScheduledDownloads = build.Select(build.Var{
		Dev:      5,
		Standard: 10,
		Testing:  5,
	}).(int)

	// chunkDownloadTimeout defines the maximum amount of time to wait for a
	// chunk download to finish before returning in the download-to-upload repair
	// loop
	chunkDownloadTimeout = build.Select(build.Var{
		Dev:      15 * time.Minute,
		Standard: 15 * time.Minute,
		Testing:  1 * time.Minute,
	}).(time.Duration)
)
