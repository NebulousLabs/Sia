package wallet

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/types"
)

// TestPrimarySeed checks that the correct seed is returned when calling
// PrimarySeed.
func TestPrimarySeed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a wallet and fetch the seed at startup.
	dir := build.TempDir(modules.WalletDir, "TestPrimarySeed")
	g, err := gateway.New(":0", filepath.Join(dir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, filepath.Join(dir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(cs, tp, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := w.Encrypt(crypto.TwofishKey{})
	if err != nil {
		t.Fatal(err)
	}
	err = w.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != nil {
		t.Fatal(err)
	}

	primarySeed, progress, err := w.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(primarySeed[:], seed[:]) {
		t.Error("PrimarySeed is returning a value inconsitent with the seed returned by Encrypt")
	}
	if progress != 0 {
		t.Error("primary seed is returning the wrong progress")
	}
	_, err = w.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	_, progress, err = w.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	if progress != 1 {
		t.Error("primary seed is returning the wrong progress")
	}

	// Lock then unlock the wallet and check the responses.
	err = w.Lock()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = w.PrimarySeed()
	if err != modules.ErrLockedWallet {
		t.Error("unexpected err:", err)
	}
	err = w.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != nil {
		t.Fatal(err)
	}
	primarySeed, progress, err = w.PrimarySeed()
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

// TestRecoverSeed checks that a seed can be successfully recovered from a
// wallet, and then remain available on subsequent loads of the wallet.
func TestRecoverSeed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester("TestRecoverSeed")
	if err != nil {
		t.Fatal(err)
	}
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
	if !bytes.Equal(allSeeds[0][:], seed[:]) {
		t.Error("AllSeeds returned the wrong seed")
	}

	dir := filepath.Join(build.TempDir(modules.WalletDir, "TestRecoverSeed - 0"), modules.WalletDir)
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
	err = w.RecoverSeed(crypto.TwofishKey(crypto.HashObject(newSeed)), seed)
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
		t.Error("wallet failed to recover a seed with money in it")
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
