package explorer

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// Explorer tester struct is the helper object for explorer
// testing. It holds the helper modules for its testing
type explorerTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	explorer *Explorer
}

// createExplorerTester creates a tester object for the explorer module.
func createExplorerTester(name string) (*explorerTester, error) {
	// Create and assemble the dependencies.
	testdir := build.TempDir(modules.HostDir, name)
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
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return nil, err
	}
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}
	e, err := New(cs, filepath.Join(testdir, modules.ExplorerDir))
	if err != nil {
		return nil, err
	}
	et := &explorerTester{
		cs:        cs,
		gateway:   g,
		miner:     m,
		tpool:     tp,
		wallet:    w,
		walletKey: key,

		explorer: e,
	}

	// Mine until the wallet has money.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := et.miner.FindBlock()
		err = et.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
	}
	return et, nil
}

// TestNilExplorerDependencies tries to initalize an explorer with nil
// dependencies, checks that the correct error is returned.
func TestNilExplorerDependencies(t *testing.T) {
	_, err := New(nil, "expdir")
	if err != errNilCS {
		t.Fatal("Expecting errNilCS")
	}
}

// TestExplorerGenesisHeight checks that when the explorer is initialized and given the
// genesis block, the result has the correct height.
func TestExplorerGenesisHeight(t *testing.T) {
	// Create the dependencies.
	testdir := build.TempDir(modules.HostDir, "TestExplorerGenesisHeight")
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the explorer - from the subscription only the genesis block will
	// be received.
	e, err := New(cs, testdir)
	if err != nil {
		t.Fatal(err)
	}
	if e.blockchainHeight != 0 {
		t.Error("genesis height was not set to zero after explorer initialization")
	}
}
