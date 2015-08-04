package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// BenchmarkAddBlockMap benchmarks adding many blocks to the set database
func BenchmarkAddBlockMap(b *testing.B) {
	b.ReportAllocs()
	cst, err := createConsensusSetTester("BenchmarkAddBlockMap")
	if err != nil {
		b.Fatal(err)
	}

	// create a bunch of blocks to be added
	blocks := make([]processedBlock, b.N)
	var nonce types.BlockNonce
	for i := 0; i < b.N; i++ {
		nonceBytes := encoding.Marshal(i)
		copy(nonce[:], nonceBytes[:8])
		blocks[i] = processedBlock{
			Block: types.Block{
				Nonce: nonce,
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cst.cs.db.addBlockMap(&blocks[i])
	}
}

func BenchmarkGetBlockMap(b *testing.B) {
	b.ReportAllocs()
	cst, err := createConsensusSetTester("BenchmarkGetBlockMap")
	if err != nil {
		b.Fatal(err)
	}

	// create a bunch of blocks to be added
	blocks := make([]processedBlock, 100)
	blockIDs := make([]types.BlockID, 100)
	var nonce types.BlockNonce
	for i := 0; i < 100; i++ {
		nonceBytes := encoding.Marshal(i)
		copy(nonce[:], nonceBytes[:8])
		blocks[i] = processedBlock{
			Block: types.Block{
				Nonce: nonce,
			},
		}
		blockIDs[i] = blocks[i].Block.ID()
	}

	for i := 0; i < 100; i++ {
		cst.cs.db.addBlockMap(&blocks[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Just do the lookup/allocation, but don't even store
		cst.cs.db.getBlockMap(blockIDs[i%100])
	}
}
