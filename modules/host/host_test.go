package host

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

// A hostTester is the helper object for host testing, including helper modules
// and methods for controlling synchronization.
type hostTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	host *Host

	persistDir string
}

// initWallet creates a wallet key, initializes the host wallet, unlocks it,
// and then stores the key in the host tester.
func (ht *hostTester) initWallet() error {
	// Create the keys for the wallet and unlock it.
	key, err := crypto.GenerateTwofishKey()
	if err != nil {
		return err
	}
	ht.walletKey = key
	_, err = ht.wallet.Encrypt(key)
	if err != nil {
		return err
	}
	err = ht.wallet.Unlock(key)
	if err != nil {
		return err
	}
	return nil
}

// blankHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankHostTester(name string) (*hostTester, error) {
	testdir := build.TempDir(modules.HostDir, name)

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
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	h, err := New(cs, tp, w, ":0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}

	// Assemble all objects into a hostTester
	ht := &hostTester{
		cs:      cs,
		gateway: g,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		host: h,

		persistDir: testdir,
	}

	return ht, nil
}

// newHostTester creates a host tester with an initialized wallet and money in
// that wallet.
func newHostTester(name string) (*hostTester, error) {
	// Create a blank host tester.
	ht, err := blankHostTester(name)
	if err != nil {
		return nil, err
	}

	// Initialize the wallet and mine blocks until the wallet has money.
	err = ht.initWallet()
	if err != nil {
		return nil, err
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return ht, nil
}

// TestHostInitialization checks that the host intializes to sensisble default
// values.
func TestHostInitialization(t *testing.T) {
	// Create a blank host tester and check that the height is zero.
	bht, err := blankHostTester("TestHostInitialization")
	if err != nil {
		t.Fatal(err)
	}
	if bht.host.blockHeight != 0 {
		t.Error("host initialized to the wrong block height")
	}

	// Initialize the wallet so that a block can be mined, then mine a block
	// and check that it sets the host height to 1.
	err = bht.initWallet()
	if err != nil {
		t.Fatal(err)
	}
	_, err = bht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if bht.host.blockHeight != 1 {
		t.Fatal("block height did not increase correctly after first block mined")
	}
}
