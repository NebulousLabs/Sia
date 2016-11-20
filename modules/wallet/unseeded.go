package wallet

import (
	"crypto/rand"
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
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

// A siagKeyPair is the struct representation of the bytes that get saved to
// disk by siag when a new keyfile is created.
type siagKeyPair struct {
	Header           string
	Version          string
	Index            int // should be uint64 - too late now
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
}

// savedKey033x is the persist structure that was used to save and load private
// keys in versions v0.3.3.x for siad.
type savedKey033x struct {
	SecretKey        crypto.SecretKey
	UnlockConditions types.UnlockConditions
	Visible          bool // indicates whether user created the key manually
}

// decryptSpendableKeyFile decrypts a spendableKeyFile, returning a
// spendableKey.
func decryptSpendableKeyFile(masterKey crypto.TwofishKey, uk spendableKeyFile) (sk spendableKey, err error) {
	// Verify that the decryption key is correct.
	decryptionKey := uidEncryptionKey(masterKey, uk.UID)
	err = verifyEncryption(decryptionKey, uk.EncryptionVerification)
	if err != nil {
		return
	}

	// Decrypt the spendable key and add it to the wallet.
	encodedKey, err := decryptionKey.DecryptBytes(uk.SpendableKey)
	if err != nil {
		return
	}
	err = encoding.Unmarshal(encodedKey, &sk)
	return
}

// integrateSpendableKey loads a spendableKey into the wallet.
func (w *Wallet) integrateSpendableKey(masterKey crypto.TwofishKey, sk spendableKey) {
	w.keys[sk.UnlockConditions.UnlockHash()] = sk
}

// loadSpendableKey loads a spendable key into the wallet database.
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
	var skf spendableKeyFile
	_, err := rand.Read(skf.UID[:])
	if err != nil {
		return err
	}
	encryptionKey := uidEncryptionKey(masterKey, skf.UID)
	skf.EncryptionVerification, err = encryptionKey.EncryptBytes(verificationPlaintext)
	if err != nil {
		return err
	}

	// Encrypt and save the key.
	skf.SpendableKey, err = encryptionKey.EncryptBytes(encoding.Marshal(sk))
	if err != nil {
		return err
	}
	return w.db.Update(func(tx *bolt.Tx) error {
		err := checkMasterKey(tx, masterKey)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketSpendableKeyFiles).Put(skf.UID[:], encoding.Marshal(skf))
	})

	// w.keys[sk.UnlockConditions.UnlockHash()] = sk -> aids with duplicate
	// detection, but causes db inconsistency. Rescanning is probably the
	// solution.
}

// recoverSiagKey creates a spendableKey from a set of siag keyfiles.
func recoverSiagKey(keyfiles []string) (spendableKey, error) {
	// Load the keyfiles from disk.
	if len(keyfiles) < 1 {
		return spendableKey{}, ErrNoKeyfile
	}
	skps := make([]siagKeyPair, len(keyfiles))
	for i, keyfile := range keyfiles {
		err := encoding.ReadFile(keyfile, &skps[i])
		if err != nil {
			return spendableKey{}, err
		}

		if skps[i].Header != SiagFileHeader {
			return spendableKey{}, ErrUnknownHeader
		}
		if skps[i].Version != SiagFileVersion {
			return spendableKey{}, ErrUnknownVersion
		}
	}

	// Check that all of the loaded files have the same address, and that there
	// are enough to create the transaction.
	baseUnlockHash := skps[0].UnlockConditions.UnlockHash()
	for _, skp := range skps {
		if skp.UnlockConditions.UnlockHash() != baseUnlockHash {
			return spendableKey{}, ErrInconsistentKeys
		}
	}
	if uint64(len(skps)) < skps[0].UnlockConditions.SignaturesRequired {
		return spendableKey{}, ErrInsufficientKeys
	}
	// Drop all unneeded keys.
	skps = skps[0:skps[0].UnlockConditions.SignaturesRequired]

	// Merge the keys into a single spendableKey and save it to the wallet.
	var sk spendableKey
	sk.UnlockConditions = skps[0].UnlockConditions
	for _, skp := range skps {
		sk.SecretKeys = append(sk.SecretKeys, skp.SecretKey)
	}
	return sk, nil
}

// LoadSiagKeys loads a set of siag-generated keys into the wallet.
func (w *Wallet) LoadSiagKeys(masterKey crypto.TwofishKey, keyfiles []string) error {
	if err := w.tg.Add(); err != nil {
		return err
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()
	sk, err := recoverSiagKey(keyfiles)
	if err != nil {
		return err
	}
	return w.loadSpendableKey(masterKey, sk)
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

	var savedKeys []savedKey033x
	err := encoding.ReadFile(filepath033x, &savedKeys)
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
	if seedsLoaded == 0 {
		return errAllDuplicates
	}
	return nil
}

// Sweep033x sweeps the outputs of a v0.3.3.x wallet into the current wallet.
func (w *Wallet) Sweep033x(filepath033x string) (coins, funds types.Currency, err error) {
	if err = w.tg.Add(); err != nil {
		return
	}
	defer w.tg.Done()

	if !w.cs.Synced() {
		return types.Currency{}, types.Currency{}, errors.New("cannot sweep until blockchain is synced")
	}

	// read savedKeys and convert to spendableKeys
	var savedKeys []savedKey033x
	err = encoding.ReadFile(filepath033x, &savedKeys)
	if err != nil {
		return
	}
	keys := make([]spendableKey, len(savedKeys))
	for i := range savedKeys {
		keys[i] = spendableKey{
			UnlockConditions: savedKeys[i].UnlockConditions,
			SecretKeys:       []crypto.SecretKey{savedKeys[i].SecretKey},
		}
	}

	s := newKeyScanner(keys)
	_, maxFee := w.tpool.FeeEstimation()
	s.dustThreshold = maxFee.Mul64(outputSize)
	err = s.scan(w.cs)
	if err != nil {
		return
	}

	scos := make([]scannedOutput, 0, len(s.siacoinOutputs))
	sfos := make([]scannedOutput, 0, len(s.siafundOutputs))
	for _, output := range s.siacoinOutputs {
		scos = append(scos, output)
	}
	for _, output := range s.siafundOutputs {
		sfos = append(sfos, output)
	}
	return w.sweepOutputs(scos, sfos)
}

// SweepSiag sweeps the outputs belonging to a siag key into the current
// wallet.
func (w *Wallet) SweepSiag(keyfiles []string) (coins, funds types.Currency, err error) {
	if err = w.tg.Add(); err != nil {
		return
	}
	defer w.tg.Done()

	if !w.cs.Synced() {
		return types.Currency{}, types.Currency{}, errors.New("cannot sweep until blockchain is synced")
	}

	sk, err := recoverSiagKey(keyfiles)
	if err != nil {
		return
	}

	s := newKeyScanner([]spendableKey{sk})
	_, maxFee := w.tpool.FeeEstimation()
	s.dustThreshold = maxFee.Mul64(outputSize)
	err = s.scan(w.cs)
	if err != nil {
		return
	}

	scos := make([]scannedOutput, 0, len(s.siacoinOutputs))
	sfos := make([]scannedOutput, 0, len(s.siafundOutputs))
	for _, output := range s.siacoinOutputs {
		scos = append(scos, output)
	}
	for _, output := range s.siafundOutputs {
		sfos = append(sfos, output)
	}
	return w.sweepOutputs(scos, sfos)
}
