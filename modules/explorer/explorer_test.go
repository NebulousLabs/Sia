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
	"github.com/NebulousLabs/Sia/persist"
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
	testdir  string
}

// createExplorerTester creates a tester object for the explorer module.
func createExplorerTester(name string) (*explorerTester, error) {
	if testing.Short() {
		panic("createExplorerTester called when in a short test")
	}

	// Create and assemble the dependencies.
	testdir := build.TempDir(modules.ExplorerDir, name)
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	key := crypto.GenerateTwofishKey()
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
		testdir:  testdir,
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

// reorgToBlank creates a bunch of empty blocks on top of the genesis block
// that reorgs the explorer to a state of all blank blocks.
func (et *explorerTester) reorgToBlank() error {
	// Get a unique directory name to house the persistence of the miner
	// dependencies.
	dir := et.testdir + " - " + persist.RandomSuffix()

	// Create a miner and all dependencies to create an alternate chain.
	g, err := gateway.New("localhost:0", false, filepath.Join(dir, modules.GatewayDir))
	if err != nil {
		return err
	}
	cs, err := consensus.New(g, false, filepath.Join(dir, modules.ConsensusDir))
	if err != nil {
		return err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(dir, modules.TransactionPoolDir))
	if err != nil {
		return err
	}
	w, err := wallet.New(cs, tp, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		return err
	}
	key := crypto.GenerateTwofishKey()
	_, err = w.Encrypt(key)
	if err != nil {
		return err
	}
	err = w.Unlock(key)
	if err != nil {
		return err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(dir, modules.RenterDir))
	if err != nil {
		return err
	}

	// Mine blocks until the height is higher than the existing consensus,
	// submitting each block to the explorerTester.
	currentHeight := cs.Height()
	for i := types.BlockHeight(0); i <= currentHeight+1; i++ {
		block, err := m.AddBlock()
		if err != nil {
			return err
		}
		et.cs.AcceptBlock(block) // error is not checked, will not always be nil
	}
	return nil
}

// TestNilExplorerDependencies tries to initialize an explorer with nil
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
	if testing.Short() {
		t.SkipNow()
	}
	// Create the dependencies.
	testdir := build.TempDir(modules.HostDir, t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the explorer - from the subscription only the genesis block will
	// be received.
	e, err := New(cs, testdir)
	if err != nil {
		t.Fatal(err)
	}
	block, height, exists := e.Block(types.GenesisID)
	if !exists {
		t.Error("explorer missing genesis block after initialization")
	}
	if block.ID() != types.GenesisID {
		t.Error("explorer returned wrong genesis block")
	}
	if height != 0 {
		t.Errorf("genesis block hash wrong height: expected 0, got %v", height)
	}
}
