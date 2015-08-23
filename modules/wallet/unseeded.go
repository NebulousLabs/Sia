package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
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

// initUnseededKeys loads all of the unseeded keys into the wallet after the
// wallet gets unlocked.
func (w *Wallet) initUnseededKeys(masterKey crypto.TwofishKey) error {
	for _, uk := range w.settings.UnseededKeys {
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
		w.keys[sk.unlockConditions.UnlockHash()] = sk
	}
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

	// Merge the keys into a single spendableKey.
	var sk spendableKey
	sk.unlockConditions = skps[0].UnlockConditions
	for _, skp := range skps {
		sk.secretKeys = append(sk.secretKeys, skp.SecretKey)
	}

	// Create the encrypted spendable key file that gets saved to disk.
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
	skf.SpendableKey, err = encryptionKey.EncryptBytes(encoding.Marshal(sk))
	if err != nil {
		return err
	}
	w.settings.UnseededKeys = append(w.settings.UnseededKeys, skf)
	return w.saveSettings()
}

// LoadSiagKeys loads a set of siag-generated keys into the wallet.
func (w *Wallet) LoadSiagKeys(masterKey crypto.TwofishKey, keyfiles []string) error {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.loadSiagKeys(masterKey, keyfiles)
}
