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

// A minerTester is the helper object for miner testing.
type minerTester struct {
	gateway modules.Gateway
	cs      modules.ConsensusSet
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	miner *Miner
}

// createMinerTester creates a minerTester that's ready for use.
func createMinerTester(name string) (*minerTester, error) {
	testdir := build.TempDir(modules.MinerDir, name)

	// Create the modules.
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

	// Assemble the minerTester.
	mt := &minerTester{
		gateway: g,
		cs:      cs,
		tpool:   tp,
		wallet:  w,

		miner: m,
	}

	// Mine until the wallet has money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := m.FindBlock()
		err = cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
	}

	return mt, nil
}

// TestMiner creates a miner, mines a few blocks, and checks that the wallet
// balance is updating as the blocks get mined.
func TestMiner(t *testing.T) {
	mt, err := createMinerTester("TestMiner")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet has money.
	if mt.wallet.Balance(true).IsZero() {
		t.Error("expecting mining full balance to not be zero")
	}
	if mt.wallet.Balance(false).IsZero() {
		t.Error("expecting mining nonfull balance to not be zero")
	}

	// Mine a bunch of blocks.
	if testing.Short() {
		t.SkipNow()
	}
	for i := 0; i < 100; i++ {
		b, _ := mt.miner.FindBlock()
		err = mt.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}
}
