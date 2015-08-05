package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var (
	seedModifier         = types.Specifier{'s', 'e', 'e', 'd'}
	generatedKeyModifier = types.Specifier{'g', 'e', 'n', 'k', 'e', 'y'}

	errAddressExhaustion = errors.New("current seed has used all available addresses")
)

type (
	// SeedFileUID is a unique id randomly generated and put at the front of
	// every seed file. It is used to make sure that a different encryption key
	// can be used for every seed file.
	SeedFileUID [crypto.EntropySize]byte

	// SeedFile stores an encrypted wallet seed on disk.
	SeedFile struct {
		SeedFileUID            SeedFileUID
		EncryptionVerification crypto.Ciphertext
		Seed                   crypto.Ciphertext
	}
)

// seedFileEncryptionKey creates an encryption key that is used to decrypt a
// specific key file.
func seedFileEncryptionKey(masterKey crypto.TwofishKey, sfuid SeedFileUID) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, seedModifier, sfuid))
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
		unlockConditions: generateUnlockConditions(pk),
		secretKeys:       []crypto.SecretKey{sk},
	}
}

// decryptSeedFile decrypts a seed file using the encryption key.
func decryptSeedFile(masterKey crypto.TwofishKey, sf SeedFile) (seed modules.Seed, err error) {
	// Verify that the provided master key is the correct key.
	decryptionKey := seedFileEncryptionKey(masterKey, sf.SeedFileUID)
	expectedDecryptedVerification := make([]byte, 32)
	decryptedVerification, err := decryptionKey.DecryptBytes(sf.EncryptionVerification)
	if err != nil {
		return seed, err
	}
	if !bytes.Equal(expectedDecryptedVerification, decryptedVerification) {
		return seed, errBadEncryptionKey
	}

	// Decrypt and return the seed.
	plainSeed, err := decryptionKey.DecryptBytes(sf.Seed)
	if err != nil {
		return seed, err
	}
	copy(seed[:], plainSeed)
	return seed, nil
}

// generateAndTrackKey tracks a key of a given index from a given seed.
func (w *Wallet) generateAndTrackKey(seed modules.Seed, index uint64) error {
	// Generate the key and check it is new to the wallet.
	spendableKey := generateSpendableKey(seed, index)
	_, exists := w.keys[spendableKey.unlockConditions.UnlockHash()]
	if exists {
		return errors.New("key is already being tracked")
	}
	w.keys[spendableKey.unlockConditions.UnlockHash()] = spendableKey
	return nil
}

// integrateSeed takes an address seed as input and from that generates
// 'publicKeysPerSeed' addresses that the wallet is able to spend.
func (w *Wallet) integrateSeed(seed modules.Seed) error {
	for i := uint64(0); i < modules.PublicKeysPerSeed; i++ {
		err := w.generateAndTrackKey(seed, i)
		if err != nil {
			return err
		}
	}
	return nil
}

// loadSeedFile loads an encrypted seed from disk, decrypting it and
// integrating all of the derived keys into the wallet. An error is returned if
// decryption fails.
func (w *Wallet) loadSeedFile(masterKey crypto.TwofishKey, fileInfo os.FileInfo) error {
	var seedFile SeedFile
	err := persist.LoadFile(seedMetadata, &seedFile, fileInfo.Name())
	if err != nil {
		return err
	}
	seed, err := decryptSeedFile(masterKey, seedFile)
	if err != nil {
		return err
	}
	return w.integrateSeed(seed)
}

// recoverSeed integrates a recovery seed into the wallet.
func (w *Wallet) recoverSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	// Integrate the seed with the wallet.
	seedFilename := filepath.Join(w.persistDir, seedFilePrefix+persist.RandomSuffix()+seedFileSuffix)
	err := w.integrateSeed(seed)
	if err != nil {
		return err
	}

	// Encrypt the seed and save the seed file.
	var sfuid SeedFileUID
	_, err = rand.Read(sfuid[:])
	if err != nil {
		return err
	}
	sek := seedFileEncryptionKey(masterKey, sfuid)
	plaintextVerification := make([]byte, encryptionVerificationLen)
	encryptionVerification, err := sek.EncryptBytes(plaintextVerification)
	if err != nil {
		return err
	}
	cryptSeed, err := sek.EncryptBytes(seed[:])
	if err != nil {
		return err
	}
	return persist.SaveFile(seedMetadata, SeedFile{sfuid, encryptionVerification, cryptSeed}, seedFilename)
}

// createSeed creates a wallet seed and encrypts it using a key derived from
// the master key.
func (w *Wallet) createSeed(masterKey crypto.TwofishKey) (modules.Seed, error) {
	// Derive the key used to encrypt the seed file, and create the encryption
	// verification object.
	var sfuid SeedFileUID
	_, err := rand.Read(sfuid[:])
	if err != nil {
		return modules.Seed{}, err
	}
	sek := seedFileEncryptionKey(masterKey, sfuid)
	plaintextVerification := make([]byte, encryptionVerificationLen)
	encryptionVerification, err := sek.EncryptBytes(plaintextVerification)
	if err != nil {
		return modules.Seed{}, err
	}

	// Create the unencrypted seed and integrate it into the wallet.
	var seed modules.Seed
	_, err = rand.Read(seed[:])
	if err != nil {
		return modules.Seed{}, err
	}
	err = w.integrateSeed(seed)
	if err != nil {
		return modules.Seed{}, err
	}

	// Encrypt the seed and save the seed file.
	filename := filepath.Join(w.persistDir, seedFilePrefix+persist.RandomSuffix()+seedFileSuffix)
	cryptSeed, err := sek.EncryptBytes(seed[:])
	if err != nil {
		return modules.Seed{}, err
	}
	w.settings.PrimarySeedFile = SeedFile{sfuid, encryptionVerification, cryptSeed}
	w.settings.PrimarySeedProgress = 0
	w.settings.PrimarySeedFilename = filename
	err = persist.SaveFile(seedMetadata, &w.settings.PrimarySeedFile, filename)
	if err != nil {
		return modules.Seed{}, err
	}
	err = w.saveSettings()
	if err != nil {
		return modules.Seed{}, err
	}
	return seed, nil
}

// initAuxiliarySeeds scans the wallet folder for wallet seeds. Auxiliary seeds
// are not used to generate new addresses.
func (w *Wallet) initAuxiliarySeeds(masterKey crypto.TwofishKey) error {
	// Scan for existing wallet seed files.
	filesInfo, err := ioutil.ReadDir(w.persistDir)
	if err != nil {
		return err
	}
	for _, fileInfo := range filesInfo {
		if strings.HasSuffix(fileInfo.Name(), seedFileSuffix) {
			err = w.loadSeedFile(masterKey, fileInfo)
			if err != nil {
				w.log.Println("WARNING: loading seed", fileInfo.Name(), "returned an error:", err)
			}
		}
	}
	return nil
}

// nextPrimarySeedAddress fetches the next address from the primary seed.
func (w *Wallet) nextPrimarySeedAddress() (types.UnlockConditions, error) {
	// Check that the wallet has been unlocked.
	if !w.unlocked {
		return types.UnlockConditions{}, errLockedWallet
	}

	// Check that the seed has room for more addresses.
	if w.settings.PrimarySeedProgress == modules.PublicKeysPerSeed {
		return types.UnlockConditions{}, errAddressExhaustion
	}

	// Using the seed, determine the public key of the next address.
	entropy := crypto.HashAll(w.primarySeed, w.settings.PrimarySeedProgress)
	_, pk := crypto.DeterministicSignatureKeys(entropy)

	// Increase the address usage.
	w.settings.PrimarySeedProgress++
	err := w.saveSettings()
	if err != nil {
		return types.UnlockConditions{}, err
	}

	uc := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       pk[:],
		}},
		SignaturesRequired: 1,
	}
	return uc, nil
}

// NewPrimarySeed has the wallet create a new primary seed for the wallet,
// archiving the old seed. The new seed is returned.
func (w *Wallet) NewPrimarySeed(masterKey crypto.TwofishKey) (modules.Seed, error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	if !w.unlocked {
		return modules.Seed{}, errLockedWallet
	}
	err := w.checkMasterKey(masterKey)
	if err != nil {
		return modules.Seed{}, err
	}
	return w.createSeed(masterKey)
}

// PrimarySeed returns the decrypted primary seed of the wallet.
func (w *Wallet) PrimarySeed(masterKey crypto.TwofishKey) (seed modules.Seed, err error) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.primarySeed, nil
}

// RecoverSeed will track all of the addresses generated by the input seed,
// reclaiming any funds that were lost due to a deleted file or lost encryption
// key. An error will be returned if the seed has already been integrated with
// the wallet.
func (w *Wallet) RecoverSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	if !w.unlocked {
		return errLockedWallet
	}
	err := w.checkMasterKey(masterKey)
	if err != nil {
		return err
	}
	return w.recoverSeed(masterKey, seed)
}

// AllSeeds returns a list of all seeds known to the wallet.
func (w *Wallet) AllSeeds(masterKey crypto.TwofishKey) ([]modules.Seed, error) {
	// Scan for existing wallet seed files.
	var seeds []modules.Seed
	filesInfo, err := ioutil.ReadDir(w.persistDir)
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range filesInfo {
		if strings.HasSuffix(fileInfo.Name(), seedFileSuffix) {
			// Open the seed file.
			var seedFile SeedFile
			err := persist.LoadFile(seedMetadata, &seedFile, fileInfo.Name())
			if err != nil {
				return nil, err
			}
			seed, err := decryptSeedFile(masterKey, seedFile)
			if err != nil {
				continue
			}

			// Check that the seed is actively being used by the wallet.
			spendableKey := generateSpendableKey(seed, 0)
			_, exists := w.keys[spendableKey.unlockConditions.UnlockHash()]
			if !exists {
				continue
			}
			seeds = append(seeds, seed)
		}
	}
	return seeds, nil
}
