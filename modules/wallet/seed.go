package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	seedFilePrefix = "Sia Wallet Encrypted Backup Seed - "
	seedFileSuffix = ".seed"
)

var (
	errAddressExhaustion = errors.New("current seed has used all available addresses")
	errKnownSeed         = errors.New("seed is already known")
)

type (
	// UniqueID is a unique id randomly generated and put at the front of every
	// persistence object. It is used to make sure that a different encryption
	// key can be used for every persistence object.
	UniqueID [crypto.EntropySize]byte

	// SeedFile stores an encrypted wallet seed on disk.
	SeedFile struct {
		UID                    UniqueID
		EncryptionVerification crypto.Ciphertext
		Seed                   crypto.Ciphertext
	}

	// SpendableKeyFile stores an encrypted spendable key on disk.
	SpendableKeyFile struct {
		UID                    UniqueID
		EncryptionVerification crypto.Ciphertext
		SpendableKey           crypto.Ciphertext
	}
)

// uidEncryptionKey creates an encryption key that is used to decrypt a
// specific key file.
func uidEncryptionKey(masterKey crypto.TwofishKey, uid UniqueID) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, uid))
}

// generateUnlockConditions provides the unlock conditions that would be
// automatically generated from the input public key.
func generateUnlockConditions(pk crypto.PublicKey) types.UnlockConditions {
	return types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       pk[:],
		}},
		SignaturesRequired: 1,
	}
}

// generateSpendableKey creates the keys and unlock conditions a given index of a
// seed.
func generateSpendableKey(seed modules.Seed, index uint64) spendableKey {
	// Generate the keys and unlock conditions.
	entropy := crypto.HashAll(seed, index)
	sk, pk := crypto.DeterministicSignatureKeys(entropy)
	return spendableKey{
		UnlockConditions: generateUnlockConditions(pk),
		SecretKeys:       []crypto.SecretKey{sk},
	}
}

// decryptSeedFile decrypts a seed file using the encryption key.
func decryptSeedFile(masterKey crypto.TwofishKey, sf SeedFile) (seed modules.Seed, err error) {
	// Verify that the provided master key is the correct key.
	decryptionKey := uidEncryptionKey(masterKey, sf.UID)
	expectedDecryptedVerification := make([]byte, crypto.EntropySize)
	decryptedVerification, err := decryptionKey.DecryptBytes(sf.EncryptionVerification)
	if err != nil {
		return modules.Seed{}, err
	}
	if !bytes.Equal(expectedDecryptedVerification, decryptedVerification) {
		return modules.Seed{}, modules.ErrBadEncryptionKey
	}

	// Decrypt and return the seed.
	plainSeed, err := decryptionKey.DecryptBytes(sf.Seed)
	if err != nil {
		return modules.Seed{}, err
	}
	copy(seed[:], plainSeed)
	return seed, nil
}

// integrateSeed takes an address seed as input and from that generates
// 'publicKeysPerSeed' addresses that the wallet is able to spend.
func (w *Wallet) integrateSeed(seed modules.Seed) {
	for i := uint64(0); i < modules.PublicKeysPerSeed; i++ {
		// Generate the key and check it is new to the wallet.
		spendableKey := generateSpendableKey(seed, i)
		w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	}
	w.seeds = append(w.seeds, seed)
}

// recoverSeed integrates a recovery seed into the wallet.
func (w *Wallet) recoverSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	// Check that the seed is not already known.
	for _, wSeed := range w.seeds {
		if seed == wSeed {
			return errKnownSeed
		}
	}

	// Encrypt the seed and save the seed file.
	var sfuid UniqueID
	_, err := rand.Read(sfuid[:])
	if err != nil {
		return err
	}
	sek := uidEncryptionKey(masterKey, sfuid)
	plaintextVerification := make([]byte, encryptionVerificationLen)
	encryptionVerification, err := sek.EncryptBytes(plaintextVerification)
	if err != nil {
		return err
	}
	cryptSeed, err := sek.EncryptBytes(seed[:])
	if err != nil {
		return err
	}
	seedFilename := filepath.Join(w.persistDir, seedFilePrefix+persist.RandomSuffix()+seedFileSuffix)
	seedFile := SeedFile{
		UID: sfuid,
		EncryptionVerification: encryptionVerification,
		Seed: cryptSeed,
	}
	err = persist.SaveFile(seedMetadata, seedFile, seedFilename)
	if err != nil {
		return err
	}

	// Add the seed file to the wallet's set of tracked seeds and save the
	// wallet settings.
	w.settings.AuxiliarySeedFiles = append(w.settings.AuxiliarySeedFiles, seedFile)
	err = w.saveSettings()
	if err != nil {
		return err
	}
	w.integrateSeed(seed)
	return nil

}

// createSeed creates a wallet seed and encrypts it using a key derived from
// the master key, then addds it to the wallet as the primary seed, while
// making a disk backup.
func (w *Wallet) createSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	// Derive the key used to encrypt the seed file, and create the encryption
	// verification object.
	var sfuid UniqueID
	_, err := rand.Read(sfuid[:])
	if err != nil {
		return err
	}
	sfek := uidEncryptionKey(masterKey, sfuid)
	plaintextVerification := make([]byte, encryptionVerificationLen)
	encryptionVerification, err := sfek.EncryptBytes(plaintextVerification)
	if err != nil {
		return err
	}

	// Encrypt the seed and save the seed file.
	seedName := seedFilePrefix + persist.RandomSuffix() + seedFileSuffix
	filename := filepath.Join(w.persistDir, seedName)
	cryptSeed, err := sfek.EncryptBytes(seed[:])
	if err != nil {
		return err
	}
	w.primarySeed = seed
	w.settings.PrimarySeedFile = SeedFile{
		UID: sfuid,
		EncryptionVerification: encryptionVerification,
		Seed: cryptSeed,
	}
	w.settings.PrimarySeedProgress = 0
	err = persist.SaveFile(seedMetadata, &w.settings.PrimarySeedFile, filename)
	if err != nil {
		return err
	}
	err = w.saveSettings()
	if err != nil {
		return err
	}
	return nil
}

// initPrimarySeed loads the primary seed into the wallet, creating a new one
// if the primary seed does not exist. The primary seed is used to generate new
// addresses.
func (w *Wallet) initPrimarySeed(masterKey crypto.TwofishKey) error {
	seed, err := decryptSeedFile(masterKey, w.settings.PrimarySeedFile)
	if err != nil {
		return err
	}
	for i := uint64(0); i < w.settings.PrimarySeedProgress; i++ {
		spendableKey := generateSpendableKey(seed, i)
		w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	}
	w.primarySeed = seed
	w.seeds = append(w.seeds, seed)
	return nil
}

// initAuxiliarySeeds scans the wallet folder for wallet seeds. Auxiliary seeds
// are not used to generate new addresses.
func (w *Wallet) initAuxiliarySeeds(masterKey crypto.TwofishKey) error {
	for _, seedFile := range w.settings.AuxiliarySeedFiles {
		seed, err := decryptSeedFile(masterKey, seedFile)
		if build.DEBUG && err != nil {
			panic(err)
		}
		if err != nil {
			w.log.Println("UNLOCK: failed to load an auxiliary seed:", err)
			continue
		}
		w.integrateSeed(seed)
	}
	return nil
}

// nextPrimarySeedAddress fetches the next address from the primary seed.
func (w *Wallet) nextPrimarySeedAddress() (types.UnlockConditions, error) {
	// Check that the wallet has been unlocked.
	if !w.unlocked {
		return types.UnlockConditions{}, modules.ErrLockedWallet
	}

	// Integrate the next key into the wallet, and return the unlock
	// conditions.
	spendableKey := generateSpendableKey(w.primarySeed, w.settings.PrimarySeedProgress)
	w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	w.settings.PrimarySeedProgress++
	err := w.saveSettings()
	if err != nil {
		return types.UnlockConditions{}, err
	}
	return spendableKey.UnlockConditions, nil
}

// AllSeeds returns a list of all seeds known to and used by the wallet.
func (w *Wallet) AllSeeds() ([]modules.Seed, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	if !w.unlocked {
		return nil, modules.ErrLockedWallet
	}
	return w.seeds, nil
}

// PrimarySeed returns the decrypted primary seed of the wallet.
func (w *Wallet) PrimarySeed() (modules.Seed, uint64, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	if !w.unlocked {
		return modules.Seed{}, 0, modules.ErrLockedWallet
	}
	return w.primarySeed, w.settings.PrimarySeedProgress, nil
}

// NextAddress returns an unlock hash that is ready to recieve siacoins or
// siafunds. The address is generated using the primary address seed.
func (w *Wallet) NextAddress() (types.UnlockConditions, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.nextPrimarySeedAddress()
}

// RecoverSeed will track all of the addresses generated by the input seed,
// reclaiming any funds that were lost due to a deleted file or lost encryption
// key. An error will be returned if the seed has already been integrated with
// the wallet.
//
// NOTE: The recovery implementation is incomplete.
func (w *Wallet) RecoverSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	if !w.unlocked {
		return modules.ErrLockedWallet
	}
	err := w.checkMasterKey(masterKey)
	if err != nil {
		return err
	}
	return w.recoverSeed(masterKey, seed)
}
