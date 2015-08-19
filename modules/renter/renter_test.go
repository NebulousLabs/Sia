package renter

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// renterTester contains all of the modules that are used while testing the renter.
type renterTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	hostdb    modules.HostDB
	miner     modules.Miner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	renter *Renter
}

// Close shuts down the renter tester.
func (rt *renterTester) Close() error {
	rt.wallet.Lock()
	rt.cs.Close()
	rt.gateway.Close()
	return nil
}

// newRenterTester creates a ready-to-use renter tester with money in the
// wallet.
func newRenterTester(name string) (*renterTester, error) {
	testdir := build.TempDir("renter", name)

	// Create the gateway.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}

	// Create the consensus set.
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}

	// Create the hostdb.
	hdb, err := hostdb.New(cs, g)
	if err != nil {
		return nil, err
	}

	// Create the tpool.
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return nil, err
	}

	// Create the wallet.
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
		t.Fatal(err)
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}

	// Create the renter.
	r, err := New(cs, hdb, w, tp, filepath.Join(testdir, modules.RenterDir))
	if err != nil {
		return nil, err
	}

	// Create the miner.
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all pieces into a renter tester.
	rt := &renterTester{
		cs:      cs,
		gateway: g,
		hostdb:  hdb,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		renter: r,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := rt.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}
	return rt, nil
}

// TestNilInputs tries supplying the renter with nil inputs and checks for
// correct rejection.
func TestNilInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester("TestNilInputs")
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(rt.cs, rt.hostdb, rt.wallet, rt.tpool, rt.renter.saveDir+"1")
	if err != nil {
		t.Error(err)
	}
	_, err = New(nil, nil, nil, nil, rt.renter.saveDir+"2")
	if err == nil {
		t.Error("no error returned for nil inputs")
	}
	_, err = New(nil, rt.hostdb, rt.wallet, rt.tpool, rt.renter.saveDir+"3")
	if err != ErrNilCS {
		t.Error(err)
	}
	_, err = New(rt.cs, nil, rt.wallet, rt.tpool, rt.renter.saveDir+"5")
	if err != ErrNilHostDB {
		t.Error(err)
	}
	_, err = New(rt.cs, rt.hostdb, nil, rt.tpool, rt.renter.saveDir+"6")
	if err != ErrNilWallet {
		t.Error(err)
	}
	_, err = New(rt.cs, rt.hostdb, rt.wallet, nil, rt.renter.saveDir+"6")
	if err != ErrNilTpool {
		t.Error(err)
	}
}
