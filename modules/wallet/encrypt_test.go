package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"
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

// For some of the encryption testing, a wallet that has not been encrypted,
// unlocked, or been used to mine coins is necessary.
func createBlankWallet(name string) (string, modules.ConsensusSet, modules.TransactionPool, modules.Miner, *Wallet, error) {
	dir := build.TempDir(modules.WalletDir, name)
	g, err := gateway.New(":0", filepath.Join(dir, modules.GatewayDir))
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	cs, err := consensus.New(g, filepath.Join(dir, modules.ConsensusDir))
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	tp, err := transactionpool.New(cs, g)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	w, err := New(cs, tp, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	return dir, cs, tp, m, w, nil
}

// postEncryptionTesting runs a series of checks on the wallet after it has
// been encrypted, to make sure that locking, unlocking, and spending after
// unlocking are all happening in the correct order and returning the correct
// errors.
func postEncryptionTesting(m modules.Miner, w *Wallet, masterKey crypto.TwofishKey) error {
	if !w.Encrypted() {
		return errors.New("supplied wallet has not been encrypted")
	}
	if w.Unlocked() {
		return errors.New("supplied wallet has been unlocked")
	}
	if len(w.seeds) != 0 {
		return errors.New("seeds value should be empty while the wallet is locked")
	}

	// Try unlocking and using the wallet.
	err := w.Unlock(masterKey)
	if err != nil {
		return err
	}
	err = w.Unlock(masterKey)
	if err != errAlreadyUnlocked {
		return errors.New("expecting errAlreadyUnlocked, grep '328'")
	}
	// Mine enough coins so that a balance appears (and some buffer for the
	// send later).
	for i := types.BlockHeight(0); i <= types.MaturityDelay+1; i++ {
		_, err := m.AddBlock()
		if err != nil {
			return err
		}
	}
	siacoinBal, _, _ := w.ConfirmedBalance()
	if siacoinBal.Cmp(types.NewCurrency64(0)) <= 0 {
		return errors.New("wallet not receiving coins, grep '626'")
	}
	err = w.Unlock(masterKey)
	if err != errAlreadyUnlocked {
		return errors.New("expecting errAlreadyUnlocked, grep '835'")
	}

	// Lock, unlock, and trying using the wallet some more.
	err = w.Lock()
	if err != nil {
		return err
	}
	err = w.Lock()
	if err != errLockedWallet {
		return errors.New("expecting errLockedWallet, grep '624'")
	}
	err = w.Unlock(crypto.TwofishKey{})
	if err != errBadEncryptionKey {
		return errors.New("expecting errBadEncryptionKey, grep '257'")
	}
	err = w.Unlock(masterKey)
	if err != nil {
		return err
	}
	// Verify that the secret keys have been restored by sending coins to the
	// void. Send more coins than are received by mining a block.
	_, err = w.SendSiacoins(types.CalculateCoinbase(0), types.UnlockHash{})
	if err != nil {
		return err
	}
	_, err = m.AddBlock()
	if err != nil {
		return err
	}
	siacoinBal2, _, _ := w.ConfirmedBalance()
	if siacoinBal2.Cmp(siacoinBal) >= 0 {
		return errors.New("sending coins failed")
	}
	return errTestCompleted
}

// TestIntegrationPreEncryption checks that the wallet operates as expected
// prior to encryption.
func TestIntegrationPreEncryption(t *testing.T) {
	dir, cs, tp, _, w0, err := createBlankWallet("TestIntegrationPreEncryption")
	if err != nil {
		t.Fatal(err)
	}
	// Check that the wallet knows it's not encrypted.
	if w0.Encrypted() {
		t.Error("wallet is reporting that it has been encrypted")
	}
	err = w0.Lock()
	if err != errLockedWallet {
		t.Fatal(err)
	}
	err = w0.Unlock(crypto.TwofishKey{})
	if err != errUnencryptedWallet {
		t.Fatal(err)
	}

	// Create a second wallet using the same directory - make sure that if any
	// files have been created, the wallet is still being treated as new.
	w1, err := New(cs, tp, filepath.Join(dir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	if w1.Encrypted() {
		t.Error("wallet is reporting that it has been encrypted when no such action has occured")
	}
	if w1.Unlocked() {
		t.Error("new wallet is not being treated as locked")
	}

}

// TestIntegrationUserSuppliedEncryption probes the encryption process when the
// user manually supplies an encryption key.
func TestIntegrationUserSuppliedEncryption(t *testing.T) {
	// Create and wallet and user-specified key, then encrypt the wallet and
	// run post-encryption tests on it.
	_, _, _, m, w, err := createBlankWallet("TestIntegrationUserSuppliedEncryption")
	if err != nil {
		t.Fatal(err)
	}
	var masterKey crypto.TwofishKey
	_, err = rand.Read(masterKey[:])
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Encrypt(masterKey)
	if err != nil {
		t.Error(err)
	}
	err = postEncryptionTesting(m, w, masterKey)
	if err != errTestCompleted {
		t.Error(err)
	}
}

// TestIntegrationBlankEncryption probes the encryption process when the user
// supplies a blank encryption key during the encryption process.
func TestIntegrationBlankEncryption(t *testing.T) {
	// Create the wallet.
	_, _, _, m, w, err := createBlankWallet("TestIntegrationBlankEncryption")
	if err != nil {
		t.Fatal(err)
	}
	// Encrypt the wallet using a blank key.
	seed, err := w.Encrypt(crypto.TwofishKey{})
	if err != nil {
		t.Error(err)
	}

	// Try unlocking the wallet using a blank key.
	err = w.Unlock(crypto.TwofishKey{})
	if err != errBadEncryptionKey {
		t.Fatal(err)
	}
	// Try unlocking the wallet using the correct key.
	err = w.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != nil {
		t.Fatal(err)
	}
	err = w.Lock()
	if err != nil {
		t.Fatal(err)
	}

	err = postEncryptionTesting(m, w, crypto.TwofishKey(crypto.HashObject(seed)))
	if err != errTestCompleted {
		t.Fatal(err)
	}

}

// TestLock checks that lock correctly wipes keys when locking the wallet,
// while still being able to track the balance of the wallet.
func TestLock(t *testing.T) {
	wt, err := createWalletTester("TestLock")
	if err != nil {
		t.Fatal(err)
	}

	// Grab a block for work - miner will not supply blocks after the wallet
	// has been locked, and the test needs to mine a block after locking the
	// wallet to verify  that the balance reporting of a locked wallet is
	// correct.
	block, _, target, err := wt.miner.BlockForWork()
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
		for i := range key.secretKeys {
			if !bytes.Equal(wipedKey, key.secretKeys[i][:]) {
				t.Error("Key was not wiped after closing the wallet")
			}
		}
	}
	if len(wt.wallet.seeds) != 0 {
		t.Error("seeds not wiped from wallet")
	}
	if !bytes.Equal(wipedKey, w.primarySeed[:]) {
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
