package wallet

import (
	"testing"
)

// TestSaveLoad tests that saving and loading a wallet restores its data.
func TestSaveLoad(t *testing.T) {
	wt := NewWalletTester("Wallet - TestSaveLoad", t)
	// add an output to the wallet
	wt.testCoinAddress()

	// save wallet data
	err := wt.save()
	if err != nil {
		wt.Fatal(err)
	}

	// create a new wallet using the saved data
	newWallet, err := New(wt.state, wt.tpool, wt.saveDir)
	if err != nil {
		wt.Fatal(err)
	}

	// check that the wallets match
	for mapKey := range wt.keys {
		if _, exists := newWallet.keys[mapKey]; !exists {
			wt.Fatal("Loaded wallet is missing a key")
		}
	}
	for mapKey := range wt.timelockedKeys {
		if _, exists := newWallet.timelockedKeys[mapKey]; !exists {
			wt.Fatal("Loaded wallet is missing a time-locked key")
		}
	}

	if wt.Balance(true).Cmp(newWallet.Balance(true)) != 0 {
		wt.Fatal("Loaded wallet has wrong balance")
	}
}
