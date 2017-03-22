package contractmanager

import (
	"testing"
)

// BenchmarkUnsetManySectors checks that unsetting a bunch of sectors
// individually takes an acceptable amount of time. Millions should be possible
// in well under a second.
//
// My laptop is doing 10e6 in 1.2 seconds. This is on the edge of too slow, but
// overall it's close enough.
func BenchmarkUnsetManySectors(b *testing.B) {
	// Create a uint64 that has all of the bits set.
	base := uint64(0)
	base--

	// Benchmark how long it takes to unset the bits.
	for i := 0; i < b.N; i++ {
		// Create a usage array with all bits set.
		b.StopTimer()
		usageArray := make([]uint64, 10e6)
		for i := 0; i < len(usageArray); i++ {
			usageArray[i] = base
		}
		b.StartTimer()

		// Set all of the bits to zero.
		for j := 0; j < len(usageArray); j++ {
			// Set each bit to zero.
			for k := 0; k < storageFolderGranularity; k++ {
				usageElement := usageArray[j]
				usageElementUpdated := usageElement & (^(1 << uint64(k)))
				usageArray[j] = usageElementUpdated
			}
		}
	}
}
