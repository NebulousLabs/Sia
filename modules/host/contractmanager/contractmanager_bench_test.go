package contractmanager

import (
	"sync"
	"testing"

	"github.com/NebulousLabs/fastrand"
)

// BenchmarkSectorLocations explores the cost of creating the sectorLocations
// map when there are 24 million elements to load. 24 million elements would
// cover 96 TiB of data storage.
//
// On my t540p it takes about 10 seconds to create a map with 24 million
// elements in it, via random insertions. The map appears to consume
// approximately 1.2 GiB of RAM. In terms of performance, lock contention within
// the contract manager is by far the bottleneck when compared to the cost of
// interacting with massive maps.
func BenchmarkSectorLocations(b *testing.B) {
	// Create a bunch of data to insert into the map - metadata equivalent to
	// storing 96 TiB in the contract manager.
	ids := make([][12]byte, 24e6)
	sectorLocations := make([]sectorLocation, 24e6)
	// Fill out the arrays in 8 threads.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			for j := i * 3e6; j < i*3e6+3e6; j++ {
				fastrand.Read(ids[j][:])
				sectorLocations[j] = sectorLocation{
					index:         uint32(fastrand.Intn(1 << 32)),
					storageFolder: uint16(fastrand.Intn(1 << 16)),
					count:         uint16(fastrand.Intn(1 << 16)),
				}
			}
		}(i)
	}
	wg.Wait()

	// Reset the timer and then benchmark the cost of doing 24 million
	// insertions into a map - equivalent to initializng the map for a host
	// storing 96 TiB of data.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := make(map[sectorID]sectorLocation)
		for i := 0; i < 24e6; i++ {
			m[ids[i]] = sectorLocations[i]
		}
	}
}

// BenchmarkStorageFolders explores the cost of maintaining and updating a
// massive usage array. The storageFolder object is small and fast with the
// exception of the usage array, which is why benchmarking is focused on the
// usage array.
//
// The usage array for 96 TiB of storage consumes less than 10 MB of RAM, far
// dwarfed by the size of the corresponding sectorLocations map that is used to
// support it.
func BenchmarkStorageFolders(b *testing.B) {
	// Create a massive usage array, matching a 96 TiB storage folder on disk.
	// The array is a bit-array, so 24e6 sectors (96 TiB) is represented by
	// 375e3 usage elements.
	usage := make([]uint64, 375e3)

	// Fill the folder to ~99.99% capacity, which will degrade performance.
	for i := 0; i < 23999e3; i++ {
		randFreeSector(usage)
	}

	// Perform insertions and get a benchmark.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		randFreeSector(usage)
	}
}
