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
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	publicKeysPerSeed = 100
)

var (
	seedModifier         = types.Specifier{'s', 'e', 'e', 'd'}
	generatedKeyModifier = types.Specifier{'g', 'e', 'n', 'k', 'e', 'y'}

	errAddressExhaustion = errors.New("current seed has used all available addresses")
)

type (
	seed [32]byte

	generatedSignatureKey struct {
		index        uint64
		encryptedKey crypto.Ciphertext
	}

	// SeedFile stores an encrypted wallet seed on disk.
	SeedFile struct {
		EncryptionVerification crypto.Ciphertext
		Seed                   crypto.Ciphertext
	}
)

// seedFileEncryptionKey creates an encryption key that is used to decrypt a
// specific key file.
func seedEncryptionKey(masterKey crypto.TwofishKey, seedFilename string) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, seedModifier, seedFilename))
}

// generatedKeyEncryptionKey creates the encryption key for a generated
// signature key.
func signatureKeyEncryptionKey(masterKey crypto.TwofishKey, seedFilename string, keyIndex uint64) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, generatedKeyModifier, seedFilename, keyIndex))
}

// generateAndTrackKey will create key 'index' from seed 's', tracking the
// public key. The secret key will be encrypted and stored.
func (w *Wallet) generateAndTrackKey(masterKey crypto.TwofishKey, s seed, seedFilename string, index uint64) error {
	// Generate the key.
	entropy := crypto.HashAll(s, index)
	sk, pk := crypto.DeterministicSignatureKeys(entropy)

	// Fetch the unlock hash.
	unlockHash := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       pk[:],
		}},
		SignaturesRequired: 1,
	}.UnlockHash()

	// Encrypt the secret key.
	skek := signatureKeyEncryptionKey(masterKey, seedFilename, index)
	encryptedSignatureKey, err := skek.EncryptBytes(sk[:])
	if err != nil {
		return err
	}

	// Add the key to the set of tracked keys.
	w.generatedKeys[unlockHash] = generatedSignatureKey{index: index, encryptedKey: encryptedSignatureKey}
	w.trackedKeys[unlockHash] = struct{}{}
	return nil
}

// integrateSeed takes an address seed as input and from that generates
// 'publicKeysPerSeed' addresses that the wallet is able to spend.
func (w *Wallet) integrateSeed(masterKey crypto.TwofishKey, s seed, seedFilename string) error {
	for i := uint64(0); i < publicKeysPerSeed; i++ {
		err := w.generateAndTrackKey(masterKey, s, seedFilename, i)
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
	// Load the seed.
	var seedFile SeedFile
	err := persist.LoadFile(seedMetadata, &seedFile, fileInfo.Name())
	if err != nil {
		return err
	}

	// Check that the master key is correct.
	key := seedEncryptionKey(masterKey, fileInfo.Name())
	expected := make([]byte, encryptionVerificationLen)
	decryptedBytes, err := key.DecryptBytes(seedFile.EncryptionVerification)
	if err != nil {
		return err
	}
	if !bytes.Equal(decryptedBytes, expected) {
		return errBadEncryptionKey
	}

	// Decrypt the seed and integrate it with the wallet.
	var s seed
	plainSeed, err := key.DecryptBytes(seedFile.Seed)
	if err != nil {
		return err
	}
	copy(s[:], plainSeed[:])
	return w.integrateSeed(masterKey, s, fileInfo.Name())
}

// createSeed creates a wallet seed and encrypts it using a key derived from
// the master key.
func (w *Wallet) createSeed(masterKey crypto.TwofishKey) error {
	// Derive the key used to encrypt the seed file, and create the encryption
	// verification object.
	filename := filepath.Join(w.persistDir, seedFilePrefix+persist.RandomSuffix()+seedFileSuffix)
	sek := seedEncryptionKey(masterKey, filename)
	plaintextVerification := make([]byte, encryptionVerificationLen)
	encryptionVerification, err := sek.EncryptBytes(plaintextVerification)
	if err != nil {
		return err
	}

	// Create the unencrypted seed and integrate it into the wallet.
	var s seed
	_, err = rand.Read(s[:])
	if err != nil {
		return err
	}
	err = w.integrateSeed(masterKey, s, filename)
	if err != nil {
		return err
	}

	// Encrypt the seed and save the seed file.
	cryptSeed, err := sek.EncryptBytes(s[:])
	if err != nil {
		return err
	}
	w.settings.PrimarySeedFile = SeedFile{encryptionVerification, cryptSeed}
	w.settings.PrimarySeedFilename = filename
	w.settings.AddressProgress = 0
	return persist.SaveFile(seedMetadata, &w.settings.PrimarySeedFile, filename)
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
				w.log.Println("WARNING: loading a seed", fileInfo.Name(), "returned an error:", err)
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
