package contractmanager

import (
	"sync"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// BenchmarkSectorLocations explores the cost of creating the sectorLocations
// map when there are 24 million elements to load.
//
// On my t540p it takes about 10 seconds to create a map with 24 million
// elements in it, via random insertions. The map appears to consume
// approximately 1.2 GB of RAM.
func BenchmarkSectorLocations(b *testing.B) {
	// Create a bunch of data to insert into the map.
	ids := make([][12]byte, 24e6)
	sectorLocations := make([]sectorLocation, 24e6)
	// Fill out the arrays in 8 threads.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			for j := i * 3e6; j < i*3e6+3e6; j++ {
				crypto.Read(ids[j][:])
				index, err := crypto.RandIntn(1 << 32)
				if err != nil {
					b.Fatal(err)
				}
				sectorLocations[j].index = uint32(index)
				storageFolder, err := crypto.RandIntn(1 << 16)
				if err != nil {
					b.Fatal(err)
				}
				sectorLocations[j].storageFolder = uint16(storageFolder)
				count, err := crypto.RandIntn(1 << 16)
				if err != nil {
					b.Fatal(err)
				}
				sectorLocations[j].count = uint16(count)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Reset the timer and then benchmark the cost of doing 24 million
	// insertions into a map.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := make(map[sectorID]sectorLocation)
		for i := 0; i < 24e6; i++ {
			m[ids[i]] = sectorLocations[i]
		}
	}
}
