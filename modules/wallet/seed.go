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
	SeedFileUID [crypto.EntropySize]byte

	// generatedSignatureKey is a key that can be used to sign outputs. All of
	// the generated keys are encrypted and kept in memory, to avoid needing to
	// keep track of different seed files and passwords.
	generatedSignatureKey struct {
		seedFileUID SeedFileUID
		keyIndex     uint64
		encryptedKey crypto.Ciphertext
	}

	// SeedFile stores an encrypted wallet seed on disk.
	SeedFile struct {
		SeedFileUID SeedFileUID
		EncryptionVerification crypto.Ciphertext
		Seed                   crypto.Ciphertext
	}
)

// seedFileEncryptionKey creates an encryption key that is used to decrypt a
// specific key file.
func seedFileEncryptionKey(masterKey crypto.TwofishKey, sfuid SeedFileUID) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, seedModifier, sfuid))
}

// signatureKeyEncryptionKey creates the encryption key for a generated
// signature key.
func signatureKeyEncryptionKey(masterKey crypto.TwofishKey, sfuid SeedFileUID, keyIndex uint64) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, generatedKeyModifier, sfuid, keyIndex))
}

// generateAddress creates the keys and unlock conditions for key 'index' of
// seed 's'.
func generateAddress(seed modules.Seed, index uint64) (crypto.SecretKey, crypto.PublicKey, types.UnlockConditions) {
	// Generate the keys and unlock conditions.
	entropy := crypto.HashAll(seed, index)
	sk, pk := crypto.DeterministicSignatureKeys(entropy)
	uc := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       pk[:],
		}},
		SignaturesRequired: 1,
	}
	return sk, pk, uc
}

// decryptSeedFile decrypts a seed file using the encryption key.
func decryptSeedFile(masterKey crypto.TwofishKey, sf SeedFile) (seed modules.Seed, error) {
	// Verify that the provided master key is the correct key.
	decryptionKey := seedFileEncryptionKey(maskerKey, sf.SeedFileUID)
	expectedDecryptedVerification := make([]byte, 32)
	decryptedVerification, err := decryptionKey.DecryptBytes(seedfile.EncryptionVerification)
	if err != nil {
		return seed, err
	}
	if !bytes.Equal(expectedDecryptedVerification, decryptedVerification) {
		return sseed, errBadEncryptionKey
	}

	// Decrypt and return the seed.
	plainSeed, err := key.DecryptBytes(seedFile.Seed)
	if err != nil {
		return seed, err
	}
	copy(seed[:], plainSeed)
	return seed, nil
}

// generateAndTrackKey will create key 'index' from seed 's', tracking the
// public key. The secret key will be encrypted and stored.
func (w *Wallet) generateAndTrackKey(masterKey crypto.TwofishKey, seed modules.Seed, sfuid SeedFileUID, index uint64) error {
	// Generate the key and check it is new to the wallet.
	sk, pk, uc := generateAddress(seed, index)
	_, exists := w.generatedKeys[uc.UnlockHash()]
	if exists {
		return errors.New("key is already being tracked")
	}

	// Encrypt the secret key.
	skek := signatureKeyEncryptionKey(masterKey, sfuid, index)
	encryptedSignatureKey, err := skek.EncryptBytes(sk[:])
	if err != nil {
		return err
	}

	// Add the key to the set of tracked keys.
	w.generatedKeys[uc.unlockHash] = generatedSignatureKey{sfuid, index, encryptedSignatureKey}
	w.trackedKeys[uc.unlockHash] = struct{}{}
	return nil
}

// integrateSeed takes an address seed as input and from that generates
// 'publicKeysPerSeed' addresses that the wallet is able to spend.
func (w *Wallet) integrateSeed(masterKey crypto.TwofishKey, seed modules.Seed, sfuid SeedFileUID) error {
	for i := uint64(0); i < publicKeysPerSeed; i++ {
		err := w.generateAndTrackKey(masterKey, seed, sfuid, i)
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
	seed, err := decryptSeedFile(masterKey, fileInfo.Name(), seedFile)
	if err != nil {
		return err
	}
	return w.integrateSeed(masterKey, seed, seedFile.seedFileUID)
}

// recoverSeed integrates a recovery seed into the wallet.
func recoverSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	// Integrate the seed with the wallet.
	seedFilename := filepath.Join(w.persistDir, seedFilePrefix+persist.RandomSuffix()+seedFileSuffix)
	err = w.integrateSeed(masterKey, seed, filename)
	if err != nil {
		return err
	}

	// Encrypt the seed and save the seed file.
	var sfuid SeedFileUID
	_, err = rand.Read(sfuid)
	if err != nil {
		return err
	}
	sek := seedEncryptionKey(masterKey, sfuid)
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
	_, err = rand.Read(sfuid)
	if err != nil {
		return modules.Seed{}, err
	}
	sek := seedEncryptionKey(masterKey, sfuid)
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
	err = w.integrateSeed(masterKey, seed, sfuid)
	if err != nil {
		return modules.Seed{}, err
	}

	// Encrypt the seed and save the seed file.
	filename := filepath.Join(w.persistDir, seedFilePrefix+persist.RandomSuffix()+seedFileSuffix)
	cryptSeed, err := sek.EncryptBytes(seed[:])
	if err != nil {
		return modules.Seed{}, err
	}
	w.settings.PrimarySeedFilename = filename
	w.settings.PrimarySeedFile = SeedFile{sfuid, encryptionVerification, cryptSeed}
	w.settings.AddressProgress = 0
	err = persist.SaveFile(seedMetadata, &w.settings.PrimarySeedFile, filename)
	if err != nil {
		return modules.Seed{}, err
	}
	err = w.saveSettings()
	if err != nil {
		return err
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
func (w *Wallet) nextPrimarySeedAddress(masterKey crypto.TwofishKey) (types.UnlockHash, error) {
	// Check that the wallet has been unlocked.
	if !w.unlocked {
		return types.UnlockHash{}, errLockedWallet
	}

	// Check that the seed has room for more addresses.
	if w.settings.AddressProgress == publicKeysPerSeed {
		return types.UnlockHash{}, errAddressExhaustion
	}

	// Check that the masterKey is correct.
	sek := seedEncryptionKey(masterKey, w.settings.PrimarySeedFilename)
	expected := make([]byte, encryptionVerificationLen)
	decryptedBytes, err := sek.DecryptBytes(w.settings.PrimarySeedFile.EncryptionVerification)
	if err != nil {
		return types.UnlockHash{}, err
	}
	if !bytes.Equal(decryptedBytes, expected) {
		return types.UnlockHash{}, errBadEncryptionKey
	}

	// Decrypt the seed.
	var s seed
	plainSeed, err := sek.DecryptBytes(w.settings.PrimarySeedFile.Seed)
	if err != nil {
		return types.UnlockHash{}, err
	}
	copy(s[:], plainSeed[:])

	// Using the seed, determine the public key of the next address.
	entropy := crypto.HashAll(s, w.settings.AddressProgress)
	_, pk := crypto.DeterministicSignatureKeys(entropy)

	// Increase the address usage.
	w.settings.AddressProgress++
	err = w.saveSettings()
	if err != nil {
		return types.UnlockHash{}, err
	}

	return types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       pk[:],
		}},
		SignaturesRequired: 1,
	}.UnlockHash(), nil
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
	if !w.unlocked {
		return seed, errLockedWallet
	}
	err := w.checkMasterKey()
	if err != nil {
		return seed, err
	}
	return decryptSeedFile(masterKey, w.settings.PrimarySeedFilename, w.settings.PrimarySeedFile)
}

// RecoverSeed will track all of the addresses generated by the input seed,
// reclaiming any funds that were lost due to a deleted file or lost encryption
// key. An error will be returned if the seed has already been integrated with
// the wallet.
func (w *Wallet) RecoverSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	if !w.unlocked {
		return seed, errLockedWallet
	}
	err := w.checkMasterKey()
	if err != nil {
		return seed, err
	}
	return w.recoverSeed(masterKey, seed)
}

// AllSeeds returns a list of all seeds known to the wallet.
func (w *Wallet) AllSeeds(masterKey crypto.TwofishKey) ([]modules.Seed, error) {
	// Scan for existing wallet seed files.
	var seeds []modules.Seed
	filesInfo, err := ioutil.ReadDir(w.persistDir)
	if err != nil {
		return err
	}
	for _, fileInfo := range filesInfo {
		if strings.HasSuffix(fileInfo.Name(), seedFileSuffix) {
			// Open the seed file.
			var seedFile SeedFile
			err := persist.LoadFile(seedMetadata, &seedFile, fileInfo.Name())
			if err != nil {
				return err
			}
			seed, err := decryptSeedFile(masterKey, seedFile)
			if err != nil {
				return err
			}

			// Check that the seed is actively being used by the wallet.
			_, _, unlockConditions := generateAddress(seed, 0)
			_, exists := w.generatedAddresses[unlockConditions.UnlockHash()]
			if !exists {
				continue
			}
			seeds = append(seeds, seed)
		}
	}
	return seeds, nil
}
