package miner

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

// TestMiner creates a miner, mines a few blocks, and checks that the wallet
// balance is updating as the blocks get mined.
func TestMiner(t *testing.T) {
	// Create the miner and all of it's dependencies.
	s := consensus.CreateGenesisState()
	g, err := gateway.New(":8900", s)
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(s, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(s, tpool, "../../miner_test.wallet")
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, g, tpool, w)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet balance starts at 0.
	if w.Balance(true).Cmp(consensus.ZeroCurrency) != 0 {
		t.Fatal("expecting initial wallet balance to be zero")
	}

	var solved bool
	for i := 0; i <= consensus.MaturityDelay; i++ {
		for !solved {
			_, solved, err = m.SolveBlock()
			if err != nil {
				t.Fatal(err)
			}
		}
		solved = false
	}

	if w.Balance(true).Cmp(consensus.ZeroCurrency) == 0 {
		t.Error("expecting mining full balance to not be zero")
	}
	if w.Balance(false).Cmp(consensus.ZeroCurrency) == 0 {
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

	// Create the miner and all of it's dependencies.
	s := consensus.CreateGenesisState()
	g, err := gateway.New(":8600", s)
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(s, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(s, tpool, "../../miner_test.wallet")
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, g, tpool, w)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 200; i++ {
		var solved bool
		for !solved {
			_, solved, err = m.SolveBlock()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
