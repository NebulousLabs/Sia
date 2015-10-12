package miner

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// TestIntegrationHeaderForWork checks that header requesting, solving, and
// submitting naively works.
func TestIntegrationHeaderForWork(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester("TestIntegreationHeaderForWork")
	if err != nil {
		t.Fatal(err)
	}

	// Get a header to grind on.
	header, target, err := mt.miner.HeaderForWork()
	if err != nil {
		t.Fatal(err)
	}

	// Solve the header.
	for {
		id := crypto.HashObject(header)
		if bytes.Compare(target[:], id[:]) >= 0 {
			break
		}
		header.Nonce[0]++
	}

	// Submit the header.
	err = mt.miner.SubmitHeader(header)
	if err != nil {
		t.Fatal(err)
	}
}

///////////////////

/*
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
	b, err := reconstructBlock(m, header)
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
	if testing.Short() {
		t.SkipNow()
	}
	mt, err := createMinerTester("TestBlockManager")
	if err != nil {
		t.Fatal(err)
	}

	// Once we have polled for 2*HeaderMemory headers, the first
	// HeaderMemory headers should be overwritten and no longer
	// work.
	headers := make([]types.BlockHeader, 2*HeaderMemory)

	for i := 0; i < HeaderMemory; i++ {
		headers[i], _, err = mt.miner.HeaderForWork()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Make sure Miner still has HeaderMemory headers stored
	for i := 0; i < HeaderMemory; i++ {
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
	if numUniqueBlocks != BlockMemory {
		t.Error("Miner is storing an incorrect number of blocks ", numUniqueBlocks)
	}

	// Start getting headers beyond HeaderMemory
	for i := HeaderMemory; i < 2*HeaderMemory; i++ {
		headers[i], _, err = mt.miner.HeaderForWork()
		if err != nil {
			t.Fatal(err)
		}

		// Make sure the oldest headers are being erased
		_, exists := mt.miner.blockMem[headers[i-HeaderMemory]]
		if exists {
			t.Error("Miner remembered too many headers")
		}
		_, exists = mt.miner.arbDataMem[headers[i-HeaderMemory]]
		if exists {
			t.Error("Miner remembered too many headers")
		}
	}

	// Try submitting a header that's just barely too old
	err = mt.miner.SubmitHeader(headers[HeaderMemory-1])
	if err == nil {
		t.Error("Miner accepted a header that should have been too old")
	}

	// Try submitting the oldest header that should still work
	minedHeader, err := mineHeader(mt.miner, headers[HeaderMemory])
	if err != nil {
		t.Fatal(err)
	}
	err = mt.miner.SubmitHeader(minedHeader)
	if err != nil {
		t.Fatal(err)
	}
}
*/
