package wallet

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

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
		t.Error("AllSeeds should be returning the primary seed.")
	}
	if allSeeds[0] != seed {
		t.Error("AllSeeds returned the wrong seed")
	}

	wt.wallet.db.Close() // db must be closed before reuse

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
	if !bytes.Equal(allSeeds[0][:], newSeed[:]) {
		t.Error("AllSeeds returned the wrong seed")
	}
	if !bytes.Equal(allSeeds[1][:], seed[:]) {
		t.Error("AllSeeds returned the wrong seed")
	}

	w.db.Close() // db must be closed before reuse

	// Rather than worry about a rescan, which isn't implemented and has
	// synchronization difficulties, just load a new wallet from the same
	// settings file - the same effect is achieved without the difficulties.
	w2, err := New(wt.cs, wt.tpool, dir)
	if err != nil {
		t.Fatal(err)
	}
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
}
