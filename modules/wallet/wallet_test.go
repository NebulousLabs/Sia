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
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	miner  modules.Miner
	wallet *Wallet

	walletMasterKey crypto.TwofishKey

	persistDir string
}

// createWalletTester takes a testing.T and creates a WalletTester.
func createWalletTester(name string) (*walletTester, error) {
	// Create the modules
	testdir := build.TempDir(modules.WalletDir, name)
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
	w, err := New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	var masterKey crypto.TwofishKey
	_, err = rand.Read(masterKey[:])
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
		cs:     cs,
		tpool:  tp,
		miner:  m,
		wallet: w,

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

// TestNilInputs tries starting the wallet using nil inputs.
func TestNilInputs(t *testing.T) {
	testdir := build.TempDir(modules.WalletDir, "TestNilInputs")
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

// TestSendSiacoins probes the SendSiacoins method of the wallet.
func TestSendSiacoins(t *testing.T) {
	// Create a wallet tester.
	wt, err := createWalletTester("TestSendSiacoins")
	if err != nil {
		t.Fatal(err)
	}

	// Get the initial balance - should be 1 block. The unconfirmed balances
	// should be 0.
	confirmedBal, _, _ := wt.wallet.ConfirmedBalance()
	unconfirmedOut, unconfirmedIn := wt.wallet.UnconfirmedBalance()
	if confirmedBal.Cmp(types.CalculateCoinbase(1)) != 0 {
		t.Error("unexpected confirmed balance")
	}
	if unconfirmedOut.Cmp(types.ZeroCurrency) != 0 {
		t.Error("unconfirmed balance should be 0")
	}
	if unconfirmedIn.Cmp(types.ZeroCurrency) != 0 {
		t.Error("unconfirmed balance should be 0")
	}

	// Send 5000 hastings. The wallet will automatically add a fee. Outgoing
	// unconfirmed siacoins - incoming unconfirmed siacoins should equal 5000 +
	// fee.
	tpoolFee := types.NewCurrency64(10).Mul(types.SiacoinPrecision) // TODO: tpool fee algo needs to be written.
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5000), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	confirmedBal2, _, _ := wt.wallet.ConfirmedBalance()
	unconfirmedOut2, unconfirmedIn2 := wt.wallet.UnconfirmedBalance()
	if confirmedBal2.Cmp(confirmedBal) != 0 {
		t.Error("confirmed balance changed without introduction of blocks")
	}
	if unconfirmedOut2.Cmp(unconfirmedIn2.Add(types.NewCurrency64(5000)).Add(tpoolFee)) != 0 {
		t.Error("sending siacoins appears to be ineffective")
	}

	// Move the balance into the confirmed set.
	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	confirmedBal3, _, _ := wt.wallet.ConfirmedBalance()
	unconfirmedOut3, unconfirmedIn3 := wt.wallet.UnconfirmedBalance()
	if confirmedBal3.Cmp(confirmedBal2.Add(types.CalculateCoinbase(2)).Sub(types.NewCurrency64(5000)).Sub(tpoolFee)) != 0 {
		t.Error("confirmed balance did not adjust to the expected value")
	}
	if unconfirmedOut3.Cmp(types.ZeroCurrency) != 0 {
		t.Error("unconfirmed balance should be 0")
	}
	if unconfirmedIn3.Cmp(types.ZeroCurrency) != 0 {
		t.Error("unconfirmed balance should be 0")
	}
}
