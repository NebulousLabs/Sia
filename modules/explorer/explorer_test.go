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

	t *testing.T
}

func createExplorerTester(name string, t *testing.T) (*explorerTester, error) {
	testdir := build.TempDir(modules.HostDir, name)

	// Create the modules
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Encrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	err = w.Unlock(key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(cs, filepath.Join(testdir, modules.ExplorerDir))
	if err != nil {
		t.Fatal(err)
	}

	et := &explorerTester{
		cs:        cs,
		gateway:   g,
		miner:     m,
		tpool:     tp,
		wallet:    w,
		walletKey: key,

		explorer: e,

		t: t,
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
