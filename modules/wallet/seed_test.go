package wallet

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
)

// resetChangeID clears the wallet's ConsensusChangeID. When Unlock is called,
// the wallet will rescan from the genesis block.
func resetChangeID(w *Wallet) {
	err := w.db.Update(func(tx *bolt.Tx) error {
		return dbPutConsensusChangeID(tx, modules.ConsensusChangeBeginning)
	})
	if err != nil {
		panic(err)
	}
}

// TestPrimarySeed checks that the correct seed is returned when calling
// PrimarySeed.
func TestPrimarySeed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Start with a blank wallet tester.
	wt, err := createBlankWalletTester("TestPrimarySeed")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Create a seed and unlock the wallet.
	seed, err := wt.wallet.Encrypt(crypto.TwofishKey{})
	if err != nil {
		t.Fatal(err)
	}
	err = wt.wallet.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != nil {
		t.Fatal(err)
	}

	// Try getting an address, see that the seed advances correctly.
	primarySeed, progress, err := wt.wallet.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(primarySeed[:], seed[:]) {
		t.Error("PrimarySeed is returning a value inconsitent with the seed returned by Encrypt")
	}
	if progress != 0 {
		t.Error("primary seed is returning the wrong progress")
	}
	_, err = wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	_, progress, err = wt.wallet.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	if progress != 1 {
		t.Error("primary seed is returning the wrong progress")
	}

	// Lock then unlock the wallet and check the responses.
	err = wt.wallet.Lock()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = wt.wallet.PrimarySeed()
	if err != modules.ErrLockedWallet {
		t.Error("unexpected err:", err)
	}
	err = wt.wallet.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != nil {
		t.Fatal(err)
	}
	primarySeed, progress, err = wt.wallet.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(primarySeed[:], seed[:]) {
		t.Error("PrimarySeed is returning a value inconsitent with the seed returned by Encrypt")
	}
	if progress != 1 {
		t.Error("progress reporting an unexpected value")
	}
}

// TestLoadSeed checks that a seed can be successfully recovered from a wallet,
// and then remain available on subsequent loads of the wallet.
func TestLoadSeed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester("TestLoadSeed")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	seed, _, err := wt.wallet.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	allSeeds, err := wt.wallet.AllSeeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(allSeeds) != 1 {
		t.Fatal("AllSeeds should be returning the primary seed.")
	} else if allSeeds[0] != seed {
		t.Fatal("AllSeeds returned the wrong seed")
	}
	wt.wallet.Close()

	dir := filepath.Join(build.TempDir(modules.WalletDir, "TestLoadSeed - 0"), modules.WalletDir)
	w, err := New(wt.cs, wt.tpool, dir)
	if err != nil {
		t.Fatal(err)
	}
	newSeed, err := w.Encrypt(crypto.TwofishKey{})
	if err != nil {
		t.Fatal(err)
	}
	err = w.Unlock(crypto.TwofishKey(crypto.HashObject(newSeed)))
	if err != nil {
		t.Fatal(err)
	}
	// Balance of wallet should be 0.
	siacoinBal, _, _ := w.ConfirmedBalance()
	if siacoinBal.Cmp(types.NewCurrency64(0)) != 0 {
		t.Error("fresh wallet should not have a balance")
	}
	err = w.LoadSeed(crypto.TwofishKey(crypto.HashObject(newSeed)), seed)
	if err != nil {
		t.Fatal(err)
	}
	allSeeds, err = w.AllSeeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(allSeeds) != 2 {
		t.Error("AllSeeds should be returning the primary seed with the recovery seed.")
	}
	if allSeeds[0] != newSeed {
		t.Error("AllSeeds returned the wrong seed")
	}
	if !bytes.Equal(allSeeds[1][:], seed[:]) {
		t.Error("AllSeeds returned the wrong seed")
	}
	w.Close()

	// Rather than worry about a rescan, which isn't implemented and has
	// synchronization difficulties, just load a new wallet from the same
	// settings file - the same effect is achieved without the difficulties.
	//
	// TODO: when proper seed loading is implemented, just check the balance
	// of w directly.
	w2, err := New(wt.cs, wt.tpool, dir)
	if err != nil {
		t.Fatal(err)
	}
	// reset the ccID so that the wallet does a full rescan
	resetChangeID(w2)
	err = w2.Unlock(crypto.TwofishKey(crypto.HashObject(newSeed)))
	if err != nil {
		t.Fatal(err)
	}
	siacoinBal2, _, _ := w2.ConfirmedBalance()
	if siacoinBal2.Cmp(types.NewCurrency64(0)) <= 0 {
		t.Error("wallet failed to load a seed with money in it")
	}
	allSeeds, err = w2.AllSeeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(allSeeds) != 2 {
		t.Error("AllSeeds should be returning the primary seed with the recovery seed.")
	}
	if !bytes.Equal(allSeeds[0][:], newSeed[:]) {
		t.Error("AllSeeds returned the wrong seed")
	}
	if !bytes.Equal(allSeeds[1][:], seed[:]) {
		t.Error("AllSeeds returned the wrong seed")
	}
	w2.Close()
}

// TestSweepSeed tests that sweeping a seed results in a transfer of its
// outputs to the wallet.
func TestSweepSeed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create a wallet with some money
	wt, err := createWalletTester("TestSweepSeed0")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	seed, _, err := wt.wallet.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	// send money to ourselves, so that we sweep a real output (instead of
	// just a miner payout)
	uc, err := wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.SiacoinPrecision, uc.UnlockHash())
	if err != nil {
		t.Fatal(err)
	}
	wt.miner.AddBlock()

	// create a blank wallet
	dir := filepath.Join(build.TempDir(modules.WalletDir, "TestSweepSeed1"), modules.WalletDir)
	w, err := New(wt.cs, wt.tpool, dir)
	if err != nil {
		t.Fatal(err)
	}
	newSeed, err := w.Encrypt(crypto.TwofishKey{})
	if err != nil {
		t.Fatal(err)
	}
	err = w.Unlock(crypto.TwofishKey(crypto.HashObject(newSeed)))
	if err != nil {
		t.Fatal(err)
	}
	// starting balance should be 0.
	siacoinBal, _, _ := w.ConfirmedBalance()
	if !siacoinBal.IsZero() {
		t.Error("fresh wallet should not have a balance")
	}

	// sweep the seed of the first wallet into the second
	swept, err := w.SweepSeed(crypto.TwofishKey{}, seed)
	if err != nil {
		t.Fatal(err)
	}

	// new wallet should have exactly 'swept' coins
	_, incoming := w.UnconfirmedBalance()
	if incoming.Cmp(swept) != 0 {
		t.Fatalf("wallet should have correct balance after sweeping seed: wanted %v, got %v", swept, incoming)
	}
}
