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

// TODO: factor out newMinerTester

// TestMiner creates a miner, mines a few blocks, and checks that the wallet
// balance is updating as the blocks get mined.
func TestMiner(t *testing.T) {
	testdir := build.TempDir("miner", "TestMiner")

	// Create the miner and all of its dependencies.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	s, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(s, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(s, tpool, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, tpool, w)
	if err != nil {
		t.Fatal(err)
	}
	minerChan := m.MinerNotify()

	// Check that the wallet balance starts at 0.
	if !w.Balance(true).IsZero() {
		t.Fatal("expecting initial wallet balance to be zero")
	}

	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = m.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		<-minerChan
	}

	if w.Balance(true).IsZero() {
		t.Error("expecting mining full balance to not be zero")
	}
	if w.Balance(false).IsZero() {
		t.Error("expecting mining nonfull balance to not be zero")
	}
}

// TestManyBlocks creates a miner, mines a bunch of blocks, and checks that
// nothing goes wrong. This test is here because previously, mining many blocks
// resulted in the state deadlocking.
func TestManyBlocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testdir := build.TempDir("miner", "TestMiner")

	// Create the miner and all of it's dependencies.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	s, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(s, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(s, tpool, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, tpool, w)
	if err != nil {
		t.Fatal(err)
	}
	minerChan := m.MinerNotify()

	for i := 0; i < 200; i++ {
		_, _, err = m.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		<-minerChan
	}
}
