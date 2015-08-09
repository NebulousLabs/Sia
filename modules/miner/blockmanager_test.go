package miner

import (
	"errors"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// reconstructBlock reconstructs a block given the miner and the header
func reconstructBlock(m *Miner, header types.BlockHeader) (*types.Block, error) {
	block, exists := m.blockMem[header]
	if !exists {
		return nil, errors.New("Header is either invalid or too old")
	}
	arbData := m.arbDataMem[header]

	block.Transactions[0].ArbitraryData[0] = arbData
	return block, nil
}

// mineHeader takes a header, and nonce grinds it. It returns
// the header with a nonce that solves the corresponding block
func mineHeader(m *Miner, header types.BlockHeader) (types.BlockHeader, error) {
	b, err := m.reconstructBlock(header)
	if err != nil {
		return types.BlockHeader{}, err
	}
	*b, _ = m.SolveBlock(*b, m.target)
	return b.Header(), nil
}

// TestBlockManager creates a miner, then polls the Miner for block
// headers to mine. It ensures that the blockmanager properly hands
// out headers, then reconstructs the blocks
func TestBlockManager(t *testing.T) {
	mt, err := createMinerTester("TestBlockManager")
	if err != nil {
		t.Fatal(err)
	}

	// Once we have polled for 2*headerForWorkMemory headers, the first
	// headerForWorkMemory headers should be overwritten and no longer
	// work.
	headers := make([]types.BlockHeader, 2*headerForWorkMemory)

	for i := 0; i < headerForWorkMemory; i++ {
		headers[i], _ = mt.miner.HeaderForWork()
	}

	// Make sure Miner still has headerForWorkMemory headers stored
	for i := 0; i < headerForWorkMemory; i++ {
		_, exists := mt.miner.blockMem[headers[i]]
		if !exists {
			t.Error("Miner did not remember enough headers")
		}
		_, exists = mt.miner.arbDataMem[headers[i]]
		if !exists {
			t.Error("Miner did not remember enough headers")
		}
	}

	// Make sure Miner isn't storing a block for each header
	numUniqueBlocks := 0
	stored := make(map[*types.Block]bool)
	for _, value := range mt.miner.blockMem {
		if !stored[value] {
			stored[value] = true
			numUniqueBlocks++
		}
	}
	if numUniqueBlocks != blockForWorkMemory {
		t.Error("Miner is storing an incorrect number of blocks ", numUniqueBlocks)
	}

	// Start getting headers beyond headerForWorkMemory
	for i := headerForWorkMemory; i < 2*headerForWorkMemory; i++ {
		headers[i], _ = mt.miner.HeaderForWork()

		// Make sure the oldest headers are being erased
		_, exists := mt.miner.blockMem[headers[i-headerForWorkMemory]]
		if exists {
			t.Error("Miner remembered too many headers")
		}
		_, exists = mt.miner.arbDataMem[headers[i-headerForWorkMemory]]
		if exists {
			t.Error("Miner remembered too many headers")
		}
	}

	// Try submitting a header that's just barely too old
	err = mt.miner.SubmitHeader(headers[headerForWorkMemory-1])
	if err == nil {
		t.Error("Miner accepted a header that should have been too old")
	}

	// Try submitting the oldest header that should still work
	minedHeader, err := mineHeader(mt.miner, headers[headerForWorkMemory])
	if err != nil {
		t.Fatal(err)
	}
	err = mt.miner.SubmitHeader(minedHeader)
	if err != nil {
		t.Fatal(err)
	}
}
