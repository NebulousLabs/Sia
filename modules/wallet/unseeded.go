package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// The header for all siag files. Do not change. Because siag was created
	// early in development, compatibility with siag requires manually handling
	// the headers and version instead of using the persist package.
	SiagFileHeader    = "siag"
	SiagFileExtension = ".siakey"
	SiagFileVersion   = "1.0"
)

var (
	ErrInconsistentKeys = errors.New("keyfiles provided that are for different addresses")
	ErrInsufficientKeys = errors.New("not enough keys provided to spend the siafunds")
	ErrNoKeyfile        = errors.New("no keyfile has been presented")
	ErrUnknownHeader    = errors.New("file contains the wrong header")
	ErrUnknownVersion   = errors.New("file has an unknown version number")

	errAllDuplicates         = errors.New("old wallet has no new seeds")
	errDuplicateSpendableKey = errors.New("key has already been loaded into the wallet")
)

// A SiagKeyPair is the struct representation of the bytes that get saved to
// disk by siag when a new keyfile is created.
type SiagKeyPair struct {
	Header           string
	Version          string
	Index            int // should be uint64 - too late now
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
}

// SavedKey033x is the persist structure that was used to save and load private
// keys in versions v0.3.3.x for siad.
type SavedKey033x struct {
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
	Visible          bool
}

// initUnseededKeys loads all of the unseeded keys into the wallet after the
// wallet gets unlocked.
func (w *Wallet) initUnseededKeys(masterKey crypto.TwofishKey) error {
	for _, uk := range w.persist.UnseededKeys {
		// Verify that the decryption key is correct.
		encKey := uidEncryptionKey(masterKey, uk.UID)
		expectedDecryptedVerification := make([]byte, crypto.EntropySize)
		decryptedVerification, err := encKey.DecryptBytes(uk.EncryptionVerification)
		if err != nil {
			return err
		}
		if !bytes.Equal(expectedDecryptedVerification, decryptedVerification) {
			return modules.ErrBadEncryptionKey
		}

		// Decrypt the spendable key and add it to the wallet.
		encodedKey, err := encKey.DecryptBytes(uk.SpendableKey)
		if err != nil {
			return err
		}
		var sk spendableKey
		err = encoding.Unmarshal(encodedKey, &sk)
		if err != nil {
			return err
		}
		w.keys[sk.UnlockConditions.UnlockHash()] = sk
	}
	return nil
}

// loadSpendableKey loads a spendable key into the wallet's persist structure.
func (w *Wallet) loadSpendableKey(masterKey crypto.TwofishKey, sk spendableKey) error {
	// Duplication is detected by looking at the set of unlock conditions. If
	// the wallet is locked, correct deduplication is uncertain.
	if !w.unlocked {
		return modules.ErrLockedWallet
	}

	// Check for duplicates.
	_, exists := w.keys[sk.UnlockConditions.UnlockHash()]
	if exists {
		return errDuplicateSpendableKey
	}

	// TODO: Check that the key is actually spendable.

	// Create a UID and encryption verification.
	var skf SpendableKeyFile
	_, err := rand.Read(skf.UID[:])
	if err != nil {
		return err
	}
	encryptionKey := uidEncryptionKey(masterKey, skf.UID)
	plaintextVerification := make([]byte, encryptionVerificationLen)
	skf.EncryptionVerification, err = encryptionKey.EncryptBytes(plaintextVerification)
	if err != nil {
		return err
	}

	// Encrypt and save the key.
	skf.SpendableKey, err = encryptionKey.EncryptBytes(encoding.Marshal(sk))
	if err != nil {
		return err
	}
	w.persist.UnseededKeys = append(w.persist.UnseededKeys, skf)
	// w.keys[sk.UnlockConditions.UnlockHash()] = sk -> aids with duplicate
	// detection, but causes db inconsistency. Rescanning is probably the
	// solution.
	return nil
}

// loadSiagKeys loads a set of siag keyfiles into the wallet, so that the
// wallet may spend the siafunds.
func (w *Wallet) loadSiagKeys(masterKey crypto.TwofishKey, keyfiles []string) error {
	// Load the keyfiles from disk.
	if len(keyfiles) < 1 {
		return ErrNoKeyfile
	}
	skps := make([]SiagKeyPair, len(keyfiles))
	for i, keyfile := range keyfiles {
		err := encoding.ReadFile(keyfile, &skps[i])
		if err != nil {
			return err
		}

		if skps[i].Header != SiagFileHeader {
			return ErrUnknownHeader
		}
		if skps[i].Version != SiagFileVersion {
			return ErrUnknownVersion
		}
	}

	// Check that all of the loaded files have the same address, and that there
	// are enough to create the transaction.
	baseUnlockHash := skps[0].UnlockConditions.UnlockHash()
	for _, skp := range skps {
		if skp.UnlockConditions.UnlockHash() != baseUnlockHash {
			return ErrInconsistentKeys
		}
	}
	if uint64(len(skps)) < skps[0].UnlockConditions.SignaturesRequired {
		return ErrInsufficientKeys
	}
	// Drop all unneeded keys.
	skps = skps[0:skps[0].UnlockConditions.SignaturesRequired]

	// Merge the keys into a single spendableKey and save it to the wallet.
	var sk spendableKey
	sk.UnlockConditions = skps[0].UnlockConditions
	for _, skp := range skps {
		sk.SecretKeys = append(sk.SecretKeys, skp.SecretKey)
	}
	err := w.loadSpendableKey(masterKey, sk)
	if err != nil {
		return err
	}
	err = w.saveSettingsSync()
	if err != nil {
		return err
	}
	return w.createBackup(filepath.Join(w.persistDir, "Sia Wallet Encrypted Backup - "+persist.RandomSuffix()+".json"))
}

// LoadSiagKeys loads a set of siag-generated keys into the wallet.
func (w *Wallet) LoadSiagKeys(masterKey crypto.TwofishKey, keyfiles []string) error {
	if err := w.tg.Add(); err != nil {
		return err
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.checkMasterKey(masterKey)
	if err != nil {
		return err
	}
	return w.loadSiagKeys(masterKey, keyfiles)
}

// Load033xWallet loads a v0.3.3.x wallet as an unseeded key, such that the
// funds become spendable to the current wallet.
func (w *Wallet) Load033xWallet(masterKey crypto.TwofishKey, filepath033x string) error {
	if err := w.tg.Add(); err != nil {
		return err
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.checkMasterKey(masterKey)
	if err != nil {
		return err
	}

	var savedKeys []SavedKey033x
	err = encoding.ReadFile(filepath033x, &savedKeys)
	if err != nil {
		return err
	}
	var seedsLoaded int
	for _, savedKey := range savedKeys {
		spendKey := spendableKey{
			UnlockConditions: savedKey.UnlockConditions,
			SecretKeys:       []crypto.SecretKey{savedKey.SecretKey},
		}
		err = w.loadSpendableKey(masterKey, spendKey)
		if err != nil && err != errDuplicateSpendableKey {
			return err
		}
		if err == nil {
			seedsLoaded++
		}
	}
	err = w.saveSettingsSync()
	if err != nil {
		return err
	}
	if seedsLoaded == 0 {
		return errAllDuplicates
	}
	return w.createBackup(filepath.Join(w.persistDir, "Sia Wallet Encrypted Backup - "+persist.RandomSuffix()+".json"))
}
