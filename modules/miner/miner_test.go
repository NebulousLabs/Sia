package miner

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

// TestMiner creates a miner, mines a few blocks, and checks that the wallet
// balance is updating as the blocks get mined.
func TestMiner(t *testing.T) {
	// Create the miner and all of it's dependencies.
	state := consensus.CreateGenesisState()
	tpool, err := transactionpool.New(state)
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := wallet.New(state, tpool, "")
	if err != nil {
		t.Fatal(err)
	}
	miner, err := New(state, tpool, wallet)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet balance starts at 0.
	if wallet.Balance(true).Cmp(consensus.ZeroCurrency) != 0 {
		t.Fatal("expecting initial wallet balance to be zero")
	}

	// Mine enough blocks for outputs to mature and check that the wallet
	// balance updates accordingly.
	_, solved, err := miner.SolveBlock()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= consensus.MaturityDelay; i++ {
		for !solved {
			_, solved, err = miner.SolveBlock()
			if err != nil {
				t.Fatal(err)
			}
		}
		solved = false
	}

	if wallet.Balance(true).Cmp(consensus.ZeroCurrency) == 0 {
		t.Error("expecting mining full balance to not be zero")
	}
	if wallet.Balance(false).Cmp(consensus.ZeroCurrency) == 0 {
		t.Error("expecting mining nonfull balance to not be zero")
	}
}
