package wallet

import (
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/types"
)

// A Wallet tester contains a ConsensusTester and has a bunch of helpful
// functions for facilitating wallet integration testing.
type walletTester struct {
	cs      modules.ConsensusSet
	gateway modules.Gateway
	tpool   modules.TransactionPool
	miner   modules.TestMiner
	wallet  *Wallet

	walletMasterKey crypto.TwofishKey

	persistDir string
}

// createWalletTester takes a testing.T and creates a WalletTester.
func createWalletTester(name string) (*walletTester, error) {
	// Create the modules
	testdir := build.TempDir(modules.WalletDir, name)
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
	w, err := New(cs, tp, filepath.Join(testdir, modules.WalletDir))
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
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}

	// Assemble all componenets into a wallet tester.
	wt := &walletTester{
		cs:      cs,
		gateway: g,
		tpool:   tp,
		miner:   m,
		wallet:  w,

		walletMasterKey: masterKey,

		persistDir: testdir,
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := wt.miner.FindBlock()
		err := wt.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
	}
	return wt, nil
}

// createBlankWalletTester creates a wallet tester that has not mined any
// blocks or encrypted the wallet.
func createBlankWalletTester(name string) (*walletTester, error) {
	// Create the modules
	testdir := build.TempDir(modules.WalletDir, name)
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
	w, err := New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all componenets into a wallet tester.
	wt := &walletTester{
		gateway: g,
		cs:      cs,
		tpool:   tp,
		miner:   m,
		wallet:  w,

		persistDir: testdir,
	}
	return wt, nil
}

// closeWt closes all of the modules in the wallet tester.
func (wt *walletTester) closeWt() error {
	errs := []error{
		wt.gateway.Close(),
		wt.cs.Close(),
		wt.tpool.Close(),
		wt.miner.Close(),
		wt.wallet.Close(),
	}
	return build.JoinErrors(errs, "; ")
}

// TestNilInputs tries starting the wallet using nil inputs.
func TestNilInputs(t *testing.T) {
	testdir := build.TempDir(modules.WalletDir, "TestNilInputs")
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal(err)
	}

	wdir := filepath.Join(testdir, modules.WalletDir)
	_, err = New(cs, nil, wdir)
	if err != errNilTpool {
		t.Error(err)
	}
	_, err = New(nil, tp, wdir)
	if err != errNilConsensusSet {
		t.Error(err)
	}
	_, err = New(nil, nil, wdir)
	if err != errNilConsensusSet {
		t.Error(err)
	}
}

// TestAllAddresses checks that AllAddresses returns all of the wallet's
// addresses in sorted order.
func TestAllAddresses(t *testing.T) {
	wt, err := createBlankWalletTester("TestAllAddresses")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	wt.wallet.keys[types.UnlockHash{1}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{5}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{0}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{2}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{4}] = spendableKey{}
	wt.wallet.keys[types.UnlockHash{3}] = spendableKey{}
	addrs := wt.wallet.AllAddresses()
	for i := range addrs {
		if addrs[i][0] != byte(i) {
			t.Error("address sorting failed:", i, addrs[i][0])
		}
	}
}

// TestCloseWallet tries to close the wallet.
func TestCloseWallet(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	testdir := build.TempDir(modules.WalletDir, "TestCloseWallet")
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal(err)
	}
	wdir := filepath.Join(testdir, modules.WalletDir)
	w, err := New(cs, tp, wdir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
