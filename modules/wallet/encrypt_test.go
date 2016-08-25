package wallet

import (
	"bytes"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// postEncryptionTesting runs a series of checks on the wallet after it has
// been encrypted, to make sure that locking, unlocking, and spending after
// unlocking are all happening in the correct order and returning the correct
// errors.
func postEncryptionTesting(m modules.TestMiner, w *Wallet, masterKey crypto.TwofishKey) {
	if !w.Encrypted() {
		panic("wallet is not encrypted when starting postEncryptionTesting")
	}
	if w.Unlocked() {
		panic("wallet is unlocked when starting postEncryptionTesting")
	}
	if len(w.seeds) != 0 {
		panic("wallet has seeds in it when startin postEncryptionTesting")
	}

	// Try unlocking and using the wallet.
	err := w.Unlock(masterKey)
	if err != nil {
		panic(err)
	}
	err = w.Unlock(masterKey)
	if err != errAlreadyUnlocked {
		panic(err)
	}
	// Mine enough coins so that a balance appears (and some buffer for the
	// send later).
	for i := types.BlockHeight(0); i <= types.MaturityDelay+1; i++ {
		_, err := m.AddBlock()
		if err != nil {
			panic(err)
		}
	}
	siacoinBal, _, _ := w.ConfirmedBalance()
	if siacoinBal.IsZero() {
		panic("wallet balance reported as 0 after maturing some mined blocks")
	}
	err = w.Unlock(masterKey)
	if err != errAlreadyUnlocked {
		panic(err)
	}

	// Lock, unlock, and trying using the wallet some more.
	err = w.Lock()
	if err != nil {
		panic(err)
	}
	err = w.Lock()
	if err != modules.ErrLockedWallet {
		panic(err)
	}
	err = w.Unlock(crypto.TwofishKey{})
	if err != modules.ErrBadEncryptionKey {
		panic(err)
	}
	err = w.Unlock(masterKey)
	if err != nil {
		panic(err)
	}
	// Verify that the secret keys have been restored by sending coins to the
	// void. Send more coins than are received by mining a block.
	_, err = w.SendSiacoins(types.CalculateCoinbase(0), types.UnlockHash{})
	if err != nil {
		panic(err)
	}
	_, err = m.AddBlock()
	if err != nil {
		panic(err)
	}
	siacoinBal2, _, _ := w.ConfirmedBalance()
	if siacoinBal2.Cmp(siacoinBal) >= 0 {
		panic("balance did not increase")
	}
}

// TestIntegrationPreEncryption checks that the wallet operates as expected
// prior to encryption.
func TestIntegrationPreEncryption(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createBlankWalletTester("TestIntegrationPreEncryption")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet knows it's not encrypted.
	if wt.wallet.Encrypted() {
		t.Error("wallet is reporting that it has been encrypted")
	}
	err = wt.wallet.Lock()
	if err != modules.ErrLockedWallet {
		t.Fatal(err)
	}
	err = wt.wallet.Unlock(crypto.TwofishKey{})
	if err != errUnencryptedWallet {
		t.Fatal(err)
	}
	wt.closeWt()

	// Create a second wallet using the same directory - make sure that if any
	// files have been created, the wallet is still being treated as new.
	w1, err := New(wt.cs, wt.tpool, filepath.Join(wt.persistDir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	if w1.Encrypted() {
		t.Error("wallet is reporting that it has been encrypted when no such action has occurred")
	}
	if w1.Unlocked() {
		t.Error("new wallet is not being treated as locked")
	}
	w1.Close()
}

// TestIntegrationUserSuppliedEncryption probes the encryption process when the
// user manually supplies an encryption key.
func TestIntegrationUserSuppliedEncryption(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create and wallet and user-specified key, then encrypt the wallet and
	// run post-encryption tests on it.
	wt, err := createBlankWalletTester("TestIntegrationUserSuppliedEncryption")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	var masterKey crypto.TwofishKey
	_, err = rand.Read(masterKey[:])
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.Encrypt(masterKey)
	if err != nil {
		t.Error(err)
	}
	postEncryptionTesting(wt.miner, wt.wallet, masterKey)
}

// TestIntegrationBlankEncryption probes the encryption process when the user
// supplies a blank encryption key during the encryption process.
func TestIntegrationBlankEncryption(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create the wallet.
	wt, err := createBlankWalletTester("TestIntegrationBlankEncryption")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	// Encrypt the wallet using a blank key.
	seed, err := wt.wallet.Encrypt(crypto.TwofishKey{})
	if err != nil {
		t.Error(err)
	}

	// Try unlocking the wallet using a blank key.
	err = wt.wallet.Unlock(crypto.TwofishKey{})
	if err != modules.ErrBadEncryptionKey {
		t.Fatal(err)
	}
	// Try unlocking the wallet using the correct key.
	err = wt.wallet.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != nil {
		t.Fatal(err)
	}
	err = wt.wallet.Lock()
	if err != nil {
		t.Fatal(err)
	}
	postEncryptionTesting(wt.miner, wt.wallet, crypto.TwofishKey(crypto.HashObject(seed)))
}

// TestLock checks that lock correctly wipes keys when locking the wallet,
// while still being able to track the balance of the wallet.
func TestLock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester("TestLock")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Grab a block for work - miner will not supply blocks after the wallet
	// has been locked, and the test needs to mine a block after locking the
	// wallet to verify  that the balance reporting of a locked wallet is
	// correct.
	block, target, err := wt.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}

	// Lock the wallet.
	siacoinBalance, _, _ := wt.wallet.ConfirmedBalance()
	err = wt.wallet.Lock()
	if err != nil {
		t.Error(err)
	}
	// Compare to the original balance.
	siacoinBalance2, _, _ := wt.wallet.ConfirmedBalance()
	if siacoinBalance2.Cmp(siacoinBalance) != 0 {
		t.Error("siacoin balance reporting changed upon closing the wallet")
	}
	// Check that the keys and seeds were wiped.
	wipedKey := make([]byte, crypto.SecretKeySize)
	for _, key := range wt.wallet.keys {
		for i := range key.SecretKeys {
			if !bytes.Equal(wipedKey, key.SecretKeys[i][:]) {
				t.Error("Key was not wiped after closing the wallet")
			}
		}
	}
	if len(wt.wallet.seeds) != 0 {
		t.Error("seeds not wiped from wallet")
	}
	if !bytes.Equal(wipedKey[:crypto.EntropySize], wt.wallet.primarySeed[:]) {
		t.Error("primary seed not wiped from memory")
	}

	// Solve the block generated earlier and add it to the consensus set, this
	// should boost the balance of the wallet.
	solvedBlock, _ := wt.miner.SolveBlock(block, target)
	err = wt.cs.AcceptBlock(solvedBlock)
	if err != nil {
		t.Fatal(err)
	}
	siacoinBalance3, _, _ := wt.wallet.ConfirmedBalance()
	if siacoinBalance3.Cmp(siacoinBalance2) <= 0 {
		t.Error("balance should increase after a block was mined")
	}
}
