package types

import (
	"io/ioutil"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
)

// BenchmarkEncodeEmptyBlock benchmarks encoding an empty block.
//
// i5-4670K, 9a90f86: 48 MB/s
// i5-4670K, f8f2df2: 211 MB/s
func BenchmarkEncodeEmptyBlock(b *testing.B) {
	var block Block
	b.SetBytes(int64(len(encoding.Marshal(block))))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		block.MarshalSia(ioutil.Discard)
	}
}

// BenchmarkDecodeEmptyBlock benchmarks decoding an empty block.
//
// i7-4770,  b0b162d: 38 MB/s
// i5-4670K, 9a90f86: 55 MB/s
// i5-4670K, f8f2df2: 166 MB/s
func BenchmarkDecodeEmptyBlock(b *testing.B) {
	var block Block
	encodedBlock := encoding.Marshal(block)
	b.SetBytes(int64(len(encodedBlock)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		err := encoding.Unmarshal(encodedBlock, &block)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncodeHeavyBlock benchmarks encoding a "heavy" block.
//
// i5-4670K, f8f2df2: 250 MB/s
func BenchmarkEncodeHeavyBlock(b *testing.B) {
	b.SetBytes(int64(len(encoding.Marshal(heavyBlock))))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		heavyBlock.MarshalSia(ioutil.Discard)
	}
}

// BenchmarkDecodeHeavyBlock benchmarks decoding a "heavy" block.
//
// i5-4670K, f8f2df2: 326 MB/s
func BenchmarkDecodeHeavyBlock(b *testing.B) {
	var block Block
	encodedBlock := encoding.Marshal(heavyBlock)
	b.SetBytes(int64(len(encodedBlock)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := encoding.Unmarshal(encodedBlock, &block)
		if err != nil {
			b.Fatal(err)
		}
	}
}
