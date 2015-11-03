package hostdb

// hostdb_test.go creates the hostdb tester and implements a few helper
// functions for managing hostdb tests.

import (
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

type hdbTester struct {
	cs      modules.ConsensusSet
	gateway modules.Gateway
	host    modules.Host
	miner   modules.TestMiner
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	walletMasterKey crypto.TwofishKey

	hostdb *HostDB
}

// newHDBTester returns a ready-to-use hdb tester, with all modules
// initialized.
func newHDBTester(name string) (*hdbTester, error) {
	// Create the modules.
	testdir := build.TempDir("hostdb", name)
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	hdb, err := New(cs, g, filepath.Join(testdir, modules.HostDBDir))
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
	var masterKey crypto.TwofishKey
	_, err = rand.Read(masterKey[:])
	if err != nil {
		return nil, err
	}
	_, err = w.Encrypt(masterKey)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(masterKey)
	if err != nil {
		return nil, err
	}
	h, err := host.New(cs, hdb, tp, w, ":0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all objects into an hdbTester.
	hdbt := &hdbTester{
		cs:      cs,
		gateway: g,
		host:    h,
		miner:   m,
		tpool:   tp,
		wallet:  w,

		walletMasterKey: masterKey,

		hostdb: hdb,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := hdbt.miner.FindBlock()
		err = hdbt.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
	}

	return hdbt, err
}

// TestNilInputs tries supplying the hostdb with nil inputs and checks for
// correct rejection.
func TestNilInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdbt, err := newHDBTester("TestNilInputs")
	if err != nil {
		t.Fatal(err)
	}

	_, err = New(nil, nil, "")
	if err == nil {
		t.Error("Should get an error when using nil inputs")
	}
	_, err = New(nil, hdbt.gateway, "")
	if err != errNilConsensusSet {
		t.Error("expecting ErrNilConsensusSet:", err)
	}
	_, err = New(hdbt.cs, nil, "")
	if err != errNilGateway {
		t.Error("expecting ErrNilGateway:", err)
	}
}
