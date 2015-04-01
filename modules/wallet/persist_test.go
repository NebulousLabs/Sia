package wallet

import (
	"testing"
)

// TestSaveLoad tests that saving and loading a wallet restores its data.
func TestSaveLoad(t *testing.T) {
	wt := NewWalletTester("Wallet - TestSaveLoad", t)

	// save wallet data
	err := wt.wallet.save()
	if err != nil {
		wt.t.Fatal(err)
	}

	// create a new wallet using the saved data
	newWallet, err := New(wt.cs, wt.tpool, wt.wallet.saveDir)
	if err != nil {
		wt.t.Fatal(err)
	}

	// check that the wallets match
	for mapKey := range wt.wallet.keys {
		if _, exists := newWallet.keys[mapKey]; !exists {
			wt.t.Fatal("Loaded wallet is missing a key")
		}
	}
	for mapKey := range wt.wallet.timelockedKeys {
		if _, exists := newWallet.timelockedKeys[mapKey]; !exists {
			wt.t.Fatal("Loaded wallet is missing a time-locked key")
		}
	}

	// TODO: I don't know how to synchronize the wallet.
}
