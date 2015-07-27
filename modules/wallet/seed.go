package wallet

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"os"
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
)

type (
	seed [32]byte

	generatedSignatureKey struct {
		index        uint64
		encryptedKey crypto.Ciphertext
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

func (w *Wallet) generateAndTrackKey(masterKey crypto.TwofishKey, s seed, seedFilename string, index uint64) error {
	// Generate the key.
	entropy := crypto.HashAll(s, index)
	_, pk, err := crypto.DeterministicSignatureKeys(entropy)
	if err != nil {
		return err
	}

	// Fetch the unlock hash.
	unlockHash := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       pk[:],
		}},
		SignaturesRequired: 1,
	}.UnlockHash()

	// Encrypt the public key.
	skek := signatureKeyEncryptionKey(masterKey, seedFilename, index)
	encryptedSignatureKey, err := skek.EncryptBytes(pk[:])
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
func (w *Wallet) integrateSeed(s seed) error {
	for i := uint64(0); i < publicKeysPerSeed; i++ {
	}
	return nil
}

func (w *Wallet) loadSeedFile(fileInfo os.FileInfo, masterKey crypto.TwofishKey) error {
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
	return w.integrateSeed(seedFile.Seed)
}

func (w *Wallet) createSeed(masterKey crypto.TwofishKey) error {
	var seedFile SeedFile
	filename := seedFilePrefix + persist.RandomSuffix() + seedFileSuffix
	key := seedEncryptionKey(masterKey, filename)
	encTest := make([]byte, encryptionVerificationLen)
	encVerification, err := key.EncryptBytes(encTest)
	if err != nil {
		return err
	}
	seedFile.EncryptionVerification = encVerification
	_, err = rand.Read(seedFile.Seed[:])
	if err != nil {
		return err
	}
	err = persist.SaveFile(seedMetadata, seedFile, filename)
	if err != nil {
		return err
	}

	// TODO: Generate the addresses and move them into memory.
	return nil
}

func (w *Wallet) initWalletSeeds(masterKey crypto.TwofishKey) error {
	// Scan for existing wallet seed files.
	foundSeed := false
	filesInfo, err := ioutil.ReadDir(w.persistDir)
	if err != nil {
		return err
	}
	for _, fileInfo := range filesInfo {
		if strings.HasSuffix(fileInfo.Name(), seedFileSuffix) {
			err = w.loadSeedFile(fileInfo, masterKey)
			if err != nil {
				w.log.Println("WARNING: loading a seed", fileInfo.Name(), "returned an error:", err)
			} else {
				foundSeed = true
			}
		}
	}

	// If no seed was found, create a new seed.
	if !foundSeed {
		err = w.createSeed(masterKey)
		if err != nil {
			return err
		}
	}
	return nil
}
