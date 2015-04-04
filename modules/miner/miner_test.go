package miner

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// TestMiner creates a miner, mines a few blocks, and checks that the wallet
// balance is updating as the blocks get mined.
func TestMiner(t *testing.T) {
	directory := "TestMiner"

	// Create the miner and all of it's dependencies.
	s := consensus.CreateGenesisState()

	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := gateway.New(":0", s, gDir)
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(s, g)
	if err != nil {
		t.Fatal(err)
	}
	wDir := tester.TempDir(directory, modules.WalletDir)
	w, err := wallet.New(s, tpool, wDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, g, tpool, w)
	if err != nil {
		t.Fatal(err)
	}
	minerChan := m.MinerSubscribe()

	// Check that the wallet balance starts at 0.
	if !w.Balance(true).IsZero() {
		t.Fatal("expecting initial wallet balance to be zero")
	}

	for i := 0; i <= types.MaturityDelay; i++ {
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
		t.Skip()
	}
	directory := "TestManyBlocks"

	// Create the miner and all of it's dependencies.
	s := consensus.CreateGenesisState()
	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := gateway.New(":0", s, gDir)
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(s, g)
	if err != nil {
		t.Fatal(err)
	}
	wDir := tester.TempDir(directory, modules.WalletDir)
	w, err := wallet.New(s, tpool, wDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, g, tpool, w)
	if err != nil {
		t.Fatal(err)
	}
	minerChan := m.MinerSubscribe()

	for i := 0; i < 200; i++ {
		_, _, err = m.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		<-minerChan
	}
}
