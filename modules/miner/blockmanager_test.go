package miner

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// createBlockManagerTester creates a Miner ready to be tested
func createBlockManagerTester(name string) (*Miner, error) {
	testdir := build.TempDir(modules.MinerDir, name)

	// Create the modules
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	m, err := New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	return m, nil
}

// reconstructBlock reconstructs a block given the miner and the header
func reconstructBlock(m *Miner, header types.BlockHeader) types.Block {
	block := m.blockMem[header]
	randTxn := m.randTxnMem[header]

	blockTransactions := append([]types.Transaction{randTxn}, block.Transactions[1:]...)
	return types.Block{
		ParentID:     block.ParentID,
		Timestamp:    block.Timestamp,
		MinerPayouts: block.MinerPayouts,
		Transactions: blockTransactions,
	}
}

// mineHeader takes a header, and nonce grinds it. It returns
// the header with a nonce that solves the corresponding block
func mineHeader(m *Miner, header types.BlockHeader) types.BlockHeader {
	b := reconstructBlock(m, header)
	b, _ = m.SolveBlock(b, m.target)
	return b.Header()
}

// TestBlockManager creates a miner, then polls the Miner for block
// headers to mine. It ensures that the blockmanager properly hands
// out headers, then reconstructs the blocks
func TestBlockManager(t *testing.T) {
	m, err := createBlockManagerTester("TestMiner")
	if err != nil {
		t.Fatal(err)
	}

	headers := make([]types.BlockHeader, 2*headerForWorkMemory)

	for i := 0; i < headerForWorkMemory; i++ {
		headers[i], _ = m.HeaderForWork()
	}

	// Make sure Miner still has headerForWorkMemory headers stored
	for i := 0; i < headerForWorkMemory; i++ {
		_, exists := m.blockMem[headers[i]]
		if !exists {
			t.Error("Miner did not remember enough headers")
		}
		_, exists = m.randTxnMem[headers[i]]
		if !exists {
			t.Error("Miner did not remember enough headers")
		}
	}

	// Make sure Miner isn't storing a block for each header
	numUniqueBlocks := 0
	stored := make(map[*types.Block]bool)
	for _, value := range m.blockMem {
		if !stored[value] {
			stored[value] = true
			numUniqueBlocks++
		}
	}
	if numUniqueBlocks != headerForWorkMemory/headersPerBlockMemory {
		t.Error("Miner is storing an incorrect number of blocks ", numUniqueBlocks)
	}

	// Start getting headers beyond headerForWorkMemory
	for i := headerForWorkMemory; i < 2*headerForWorkMemory; i++ {
		headers[i], _ = m.HeaderForWork()

		// Make sure the oldest headers are being erased
		_, exists := m.blockMem[headers[i-headerForWorkMemory]]
		if exists {
			t.Error("Miner remembered too many headers")
		}
		_, exists = m.randTxnMem[headers[i-headerForWorkMemory]]
		if exists {
			t.Error("Miner remembered too many headers")
		}
	}

	// Try submitting the oldest header
	err = m.SubmitHeader(mineHeader(m, headers[2*headerForWorkMemory-1]))
	if err != nil {
		t.Fatal(err)
	}

	// TODO: Try submitting 10 other random headers
}
