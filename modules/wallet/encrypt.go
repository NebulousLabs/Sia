package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errAlreadyUnlocked   = errors.New("wallet has already been unlocked")
	errReencrypt         = errors.New("wallet is already encrypted, cannot encrypt again")
	errUnencryptedWallet = errors.New("wallet has not been encrypted yet")

	unlockModifier = types.Specifier{'u', 'n', 'l', 'o', 'c', 'k'}
)

// uidEncryptionKey creates an encryption key that is used to decrypt a
// specific key file.
func uidEncryptionKey(masterKey crypto.TwofishKey, uid UniqueID) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, uid))
}

// checkMasterKey verifies that the master key is correct.
func (w *Wallet) checkMasterKey(masterKey crypto.TwofishKey) error {
	uk := uidEncryptionKey(masterKey, w.persist.UID)
	verification, err := uk.DecryptBytes(w.persist.EncryptionVerification)
	if err != nil {
		// Most of the time, the failure is an authentication failure.
		return modules.ErrBadEncryptionKey
	}
	expected := make([]byte, encryptionVerificationLen)
	if !bytes.Equal(expected, verification) {
		return modules.ErrBadEncryptionKey
	}
	return nil
}

// initEncryption checks that the provided encryption key is the valid
// encryption key for the wallet. If encryption has not yet been established
// for the wallet, an encryption key is created.
func (w *Wallet) initEncryption(masterKey crypto.TwofishKey) (modules.Seed, error) {
	// Check if the wallet encryption key has already been set.
	if len(w.persist.EncryptionVerification) != 0 {
		return modules.Seed{}, errReencrypt
	}

	// Create a random seed and use it to generate the seed file for the
	// wallet.
	var seed modules.Seed
	_, err := rand.Read(seed[:])
	if err != nil {
		return modules.Seed{}, err
	}

	// If the input key is blank, use the seed to create the master key.
	// Otherwise, use the input key.
	if masterKey == (crypto.TwofishKey{}) {
		masterKey = crypto.TwofishKey(crypto.HashObject(seed))
	}
	err = w.createSeed(masterKey, seed)
	if err != nil {
		return modules.Seed{}, err
	}

	// Establish the encryption verification using the masterKey. After this
	// point, the wallet is encrypted.
	uk := uidEncryptionKey(masterKey, w.persist.UID)
	encryptionBase := make([]byte, encryptionVerificationLen)
	w.persist.EncryptionVerification, err = uk.EncryptBytes(encryptionBase)
	if err != nil {
		return modules.Seed{}, err
	}
	err = w.saveSettings(false)
	if err != nil {
		return modules.Seed{}, err
	}
	return seed, nil
}

// unlock loads all of the encrypted file structures into wallet memory. Even
// after loading, the structures are kept encrypted, but some data such as
// addresses are decrypted so that the wallet knows what to track.
func (w *Wallet) unlock(masterKey crypto.TwofishKey) error {
	// Wallet should only be unlocked once.
	if w.unlocked {
		return errAlreadyUnlocked
	}

	// Check if the wallet encryption key has already been set.
	if len(w.persist.EncryptionVerification) == 0 {
		return errUnencryptedWallet
	}

	// Initialize the encryption of the wallet.
	err := w.checkMasterKey(masterKey)
	if err != nil {
		return err
	}

	// Load the wallet seed that is used to generate new addresses.
	err = w.initPrimarySeed(masterKey)
	if err != nil {
		return err
	}

	// Load all wallet seeds that are not used to generate new addresses.
	err = w.initAuxiliarySeeds(masterKey)
	if err != nil {
		return err
	}

	// Load all keys that were not generated by a seed.
	err = w.initUnseededKeys(masterKey)
	if err != nil {
		return err
	}
	w.unlocked = true
	return nil
}

// wipeSecrets erases all of the seeds and secret keys in the wallet.
func (w *Wallet) wipeSecrets() {
	// 'for i := range' must be used to prevent copies of secret data from
	// being made.
	for i := range w.keys {
		for j := range w.keys[i].SecretKeys {
			crypto.SecureWipe(w.keys[i].SecretKeys[j][:])
		}
	}
	for i := range w.seeds {
		crypto.SecureWipe(w.seeds[i][:])
	}
	crypto.SecureWipe(w.primarySeed[:])
	w.seeds = w.seeds[:0]
}

// Encrypted returns whether or not the wallet has been encrypted.
func (w *Wallet) Encrypted() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if build.DEBUG && w.unlocked && len(w.persist.EncryptionVerification) == 0 {
		panic("wallet is both unlocked and unencrypted")
	}
	return len(w.persist.EncryptionVerification) != 0
}

// Encrypt will encrypt the wallet using the input key. Upon encryption, a
// primary seed will be created for the wallet (no seed exists prior to this
// point). If the key is blank, then the hash of the seed that is generated
// will be used as the key. The wallet will still be locked after encryption.
//
// Encrypt can only be called once throughout the life of the wallet, and will
// return an error on subsequent calls (even after restarting the wallet). To
// reset the wallet, the wallet files must be moved to a different directory or
// deleted.
func (w *Wallet) Encrypt(masterKey crypto.TwofishKey) (modules.Seed, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.initEncryption(masterKey)
}

// Unlocked indicates whether the wallet is locked or unlocked.
func (w *Wallet) Unlocked() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.unlocked
}

// Lock will erase all keys from memory and prevent the wallet from spending
// coins until it is unlocked.
func (w *Wallet) Lock() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return modules.ErrLockedWallet
	}
	w.log.Println("INFO: Locking wallet.")

	// Wipe all of the seeds and secret keys, they will be replaced upon
	// calling 'Unlock' again.
	w.wipeSecrets()
	w.unlocked = false
	return nil
}

// Unlock will decrypt the wallet seed and load all of the addresses into
// memory.
func (w *Wallet) Unlock(masterKey crypto.TwofishKey) error {
	w.log.Println("INFO: Unlocking wallet.")

	// Initialize all of the keys in the wallet under a lock. While holding the
	// lock, also grab the subscriber status.
	w.mu.Lock()
	subscribed := w.subscribed
	err := w.unlock(masterKey)
	w.mu.Unlock()
	if err != nil {
		return err
	}

	// Subscribe to the consensus set if this is the first unlock for the
	// wallet object.
	if !subscribed {
		err = w.cs.ConsensusSetPersistentSubscribe(w, modules.ConsensusChangeID{})
		if err != nil {
			return errors.New("wallet subscription failed: " + err.Error())
		}
		w.tpool.TransactionPoolSubscribe(w)
		w.mu.Lock()
		w.subscribed = true
		w.mu.Unlock()
	}
	return nil
}
