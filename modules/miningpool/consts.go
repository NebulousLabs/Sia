package pool

import (
	"github.com/NebulousLabs/Sia/build"

	"time"
)

const ()

var (
	// workingStatusFirstCheck defines how frequently the Pool's working status
	// check runs
	workingStatusFirstCheck = build.Select(build.Var{
		Standard: time.Minute * 3,
		Dev:      time.Minute * 1,
		Testing:  time.Second * 3,
	}).(time.Duration)

	// workingStatusFrequency defines how frequently the Pool's working status
	// check runs
	workingStatusFrequency = build.Select(build.Var{
		Standard: time.Minute * 10,
		Dev:      time.Minute * 5,
		Testing:  time.Second * 10,
	}).(time.Duration)

	// workingStatusThreshold defines how many settings calls must occur over the
	// workingStatusFrequency for the pool to be considered working.
	workingStatusThreshold = build.Select(build.Var{
		Standard: uint64(3),
		Dev:      uint64(1),
		Testing:  uint64(1),
	}).(uint64)

	// connectablityCheckFirstWait defines how often the pool's connectability
	// check is run.
	connectabilityCheckFirstWait = build.Select(build.Var{
		Standard: time.Minute * 2,
		Dev:      time.Minute * 1,
		Testing:  time.Second * 3,
	}).(time.Duration)

	// connectablityCheckFrequency defines how often the pool's connectability
	// check is run.
	connectabilityCheckFrequency = build.Select(build.Var{
		Standard: time.Minute * 10,
		Dev:      time.Minute * 5,
		Testing:  time.Second * 10,
	}).(time.Duration)

	// connectabilityCheckTimeout defines how long a connectability check's dial
	// will be allowed to block before it times out.
	connectabilityCheckTimeout = build.Select(build.Var{
		Standard: time.Minute * 2,
		Dev:      time.Minute * 5,
		Testing:  time.Second * 90,
	}).(time.Duration)

	// logAllLimit is the number of errors of each type that the host will log
	// before switching to probabilistic logging. If there are not many errors,
	// it is reasonable that all errors get logged. If there are lots of
	// errors, to cut down on the noise only some of the errors get logged.
	logAllLimit = build.Select(build.Var{
		Dev:      uint64(50),
		Standard: uint64(250),
		Testing:  uint64(100),
	}).(uint64)

	// logFewLimit is the number of errors of each type that the host will log
	// before substantially constricting the amount of logging that it is
	// doing.
	logFewLimit = build.Select(build.Var{
		Dev:      uint64(500),
		Standard: uint64(2500),
		Testing:  uint64(500),
	}).(uint64)

	// rpcRatelimit prevents someone from spamming the pool with connections,
	// causing it to spin up enough goroutines to crash.
	rpcRatelimit = build.Select(build.Var{
		Dev:      time.Millisecond * 10,
		Standard: time.Millisecond * 50,
		Testing:  time.Millisecond,
	}).(time.Duration)
)

// All of the following variables define the names of buckets used by the pool
// in the database.
var (
	// bucketActionItems maps a blockchain height to a list of storage
	// obligations that need to be managed in some way at that height. The
	// height is stored as a big endian uint64, which means that bolt will
	// store the heights sorted in numerical order. The action item itself is
	// an array of file contract ids. The host is able to contextually figure
	// out what the necessary actions for that item are based on the file
	// contract id and the associated storage obligation that can be retrieved
	// using the id.
	bucketActionItems = []byte("BucketActionItems")

	// bucketStorageObligations contains a set of serialized
	// 'storageObligations' sorted by their file contract id.
	bucketStorageObligations = []byte("BucketStorageObligations")
)

// init runs a series of sanity checks to verify that the constants have sane
// values.
func init() {
}
