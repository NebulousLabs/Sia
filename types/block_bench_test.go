package types

import (
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
)

// BenchmarkDecodeEmptyBlock benchmarks decoding an empty block.
//
// i7-4770, 08-20-2015: 38 MB/s
func BenchmarkDecodeBlock(b *testing.B) {
	var block Block
	encodedBlock := encoding.Marshal(block)
	b.SetBytes(int64(len(encodedBlock)))
	for i := 0; i < b.N; i++ {
		err := encoding.Unmarshal(encodedBlock, &block)
		if err != nil {
			b.Fatal(err)
		}
	}
}
