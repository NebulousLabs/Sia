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
		Testing:  time.Second * 61,
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
		Testing:  time.Minute * 15,
	}).(time.Duration)
)
