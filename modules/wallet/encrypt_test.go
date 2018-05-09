package wallet

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// postEncryptionTesting runs a series of checks on the wallet after it has
// been encrypted, to make sure that locking, unlocking, and spending after
// unlocking are all happening in the correct order and returning the correct
// errors.
func postEncryptionTesting(m modules.TestMiner, w *Wallet, masterKey crypto.TwofishKey) {
	encrypted, err := w.Encrypted()
	if err != nil {
		panic(err)
	}
	unlocked, err := w.Unlocked()
	if err != nil {
		panic(err)
	}
	if !encrypted {
		panic("wallet is not encrypted when starting postEncryptionTesting")
	}
	if unlocked {
		panic("wallet is unlocked when starting postEncryptionTesting")
	}
	if len(w.seeds) != 0 {
		panic("wallet has seeds in it when startin postEncryptionTesting")
	}

	// Try unlocking and using the wallet.
	err = w.Unlock(masterKey)
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
	siacoinBal, _, _, err := w.ConfirmedBalance()
	if err != nil {
		panic(err)
	}
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
	siacoinBal2, _, _, err := w.ConfirmedBalance()
	if err != nil {
		panic(err)
	}
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
	wt, err := createBlankWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Check that the wallet knows it's not encrypted.
	encrypted, err := wt.wallet.Encrypted()
	if err != nil {
		t.Fatal(err)
	}
	if encrypted {
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
	encrypted, err = w1.Encrypted()
	if encrypted {
		t.Error("wallet is reporting that it has been encrypted when no such action has occurred")
	}
	unlocked, err := w1.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	unlocked, err = w1.Unlocked()
	if err != nil {
		t.Fatal(err)
	}
	if unlocked {
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
	wt, err := createBlankWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	var masterKey crypto.TwofishKey
	fastrand.Read(masterKey[:])
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
	wt, err := createBlankWalletTester(t.Name())
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
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
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
	siacoinBalance, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Error(err)
	}
	err = wt.wallet.Lock()
	if err != nil {
		t.Error(err)
	}
	// Compare to the original balance.
	siacoinBalance2, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Error(err)
	}
	if !siacoinBalance2.Equals(siacoinBalance) {
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
	siacoinBalance3, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Error(err)
	}
	if siacoinBalance3.Cmp(siacoinBalance2) <= 0 {
		t.Error("balance should increase after a block was mined")
	}
}

// TestInitFromSeedConcurrentUnlock verifies that calling InitFromSeed and
// then Unlock() concurrently results in the correct balance.
func TestInitFromSeedConcurrentUnlock(t *testing.T) {
	t.Skip("Test has poor concurrency design")
	if testing.Short() {
		t.SkipNow()
	}
	// create a wallet with some money
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	seed, _, err := wt.wallet.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	origBal, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}

	// create a blank wallet
	dir := filepath.Join(build.TempDir(modules.WalletDir, t.Name()+"-new"), modules.WalletDir)
	w, err := New(wt.cs, wt.tpool, dir)
	if err != nil {
		t.Fatal(err)
	}

	// spawn an initfromseed goroutine
	go w.InitFromSeed(crypto.TwofishKey{}, seed)

	// pause for 10ms to allow the seed sweeper to start
	time.Sleep(time.Millisecond * 10)

	// unlock should now return an error
	err = w.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != errScanInProgress {
		t.Fatal("expected errScanInProgress, got", err)
	}
	// wait for init to finish
	for i := 0; i < 100; i++ {
		time.Sleep(time.Millisecond * 10)
		err = w.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
		if err == nil {
			break
		}
	}

	// starting balance should match the original wallet
	newBal, _, _, err := w.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	if newBal.Cmp(origBal) != 0 {
		t.Log(w.UnconfirmedBalance())
		t.Fatalf("wallet should have correct balance after loading seed: wanted %v, got %v", origBal, newBal)
	}
}

// TestUnlockConcurrent verifies that calling unlock multiple times
// concurrently results in only one unlock operation.
func TestUnlockConcurrent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create a wallet with some money
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// lock the wallet
	wt.wallet.Lock()

	// spawn an unlock goroutine
	errChan := make(chan error)
	go func() {
		// acquire the write lock so that Unlock acquires the trymutex, but
		// cannot proceed further
		wt.wallet.mu.Lock()
		errChan <- wt.wallet.Unlock(wt.walletMasterKey)
	}()

	// wait for goroutine to start
	time.Sleep(time.Millisecond * 10)

	// unlock should now return an error
	err = wt.wallet.Unlock(wt.walletMasterKey)
	if err != errScanInProgress {
		t.Fatal("expected errScanInProgress, got", err)
	}

	wt.wallet.mu.Unlock()
	if err := <-errChan; err != nil {
		t.Fatal("first unlock failed:", err)
	}
}

// TestInitFromSeed tests creating a wallet from a preexisting seed.
func TestInitFromSeed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create a wallet with some money
	wt, err := createWalletTester("TestInitFromSeed0", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()
	seed, _, err := wt.wallet.PrimarySeed()
	if err != nil {
		t.Fatal(err)
	}
	origBal, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}

	// create a blank wallet
	dir := filepath.Join(build.TempDir(modules.WalletDir, "TestInitFromSeed1"), modules.WalletDir)
	w, err := New(wt.cs, wt.tpool, dir)
	if err != nil {
		t.Fatal(err)
	}
	err = w.InitFromSeed(crypto.TwofishKey{}, seed)
	if err != nil {
		t.Fatal(err)
	}
	err = w.Unlock(crypto.TwofishKey(crypto.HashObject(seed)))
	if err != nil {
		t.Fatal(err)
	}
	// starting balance should match the original wallet
	newBal, _, _, err := w.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	if newBal.Cmp(origBal) != 0 {
		t.Log(w.UnconfirmedBalance())
		t.Fatalf("wallet should have correct balance after loading seed: wanted %v, got %v", origBal, newBal)
	}
}

// TestReset tests that Reset resets a wallet correctly.
func TestReset(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	wt, err := createBlankWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	var originalKey crypto.TwofishKey
	fastrand.Read(originalKey[:])
	_, err = wt.wallet.Encrypt(originalKey)
	if err != nil {
		t.Fatal(err)
	}
	postEncryptionTesting(wt.miner, wt.wallet, originalKey)

	err = wt.wallet.Reset()
	if err != nil {
		t.Fatal(err)
	}

	// reinitialize the miner so it mines into the new seed
	err = wt.miner.Close()
	if err != nil {
		t.Fatal(err)
	}
	minerData := filepath.Join(wt.persistDir, modules.MinerDir)
	err = os.RemoveAll(minerData)
	if err != nil {
		t.Fatal(err)
	}
	newminer, err := miner.New(wt.cs, wt.tpool, wt.wallet, filepath.Join(wt.persistDir, modules.MinerDir))
	if err != nil {
		t.Fatal(err)
	}
	wt.miner = newminer

	var newKey crypto.TwofishKey
	fastrand.Read(newKey[:])
	_, err = wt.wallet.Encrypt(newKey)
	if err != nil {
		t.Fatal(err)
	}
	postEncryptionTesting(wt.miner, wt.wallet, newKey)
}

// TestChangeKey tests that a wallet can only be unlocked with the new key
// after changing it and that it shows the same balance as before
func TestChangeKey(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	var newKey crypto.TwofishKey
	fastrand.Read(newKey[:])
	origBal, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}

	err = wt.wallet.ChangeKey(wt.walletMasterKey, newKey)
	if err != nil {
		t.Fatal(err)
	}

	err = wt.wallet.Lock()
	if err != nil {
		t.Fatal(err)
	}

	err = wt.wallet.Unlock(wt.walletMasterKey)
	if err == nil {
		t.Fatal("expected unlock to fail with the original key")
	}

	err = wt.wallet.Unlock(newKey)
	if err != nil {
		t.Fatal(err)
	}
	newBal, _, _, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Fatal(err)
	}
	if newBal.Cmp(origBal) != 0 {
		t.Fatal("wallet with changed key did not have the same balance")
	}

	err = wt.wallet.Lock()
	if err != nil {
		t.Fatal(err)
	}
	postEncryptionTesting(wt.miner, wt.wallet, newKey)
}
