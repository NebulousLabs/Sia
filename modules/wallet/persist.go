package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	logFile        = modules.WalletDir + ".log"
	settingsFile   = modules.WalletDir + ".json"
	seedFileSuffix = ".seed"

	encryptionVerificationLen = 32
)

var (
	errAlreadyUnlocked  = errors.New("wallet has already been unlocked")
	errBadEncryptionKey = errors.New("provided encryption key is incorrect")

	settingsMetadata = persist.Metadata{"Wallet Settings", "0.4.0"}
	seedMetadata     = persist.Metadata{"Wallet Seed", "0.4.0"}

	encryptionTestModifier = types.Specifier{'e', 'n', 'c', 't', 'e', 's', 't'}
	seedModifier           = types.Specifier{'k', 'e', 'y', 's', 'e', 'e', 'd'}
)

type (
	WalletSettings struct {
		// EncryptionVerification is an encrypted string that, when decrypted, is
		// 32 '0' bytes.
		EncryptionVerification crypto.Ciphertext
	}

	SeedFile struct {
		EncryptionVerification crypto.Ciphertext
		Seed                   [32]byte
	}
)

func (w *Wallet) saveSettings() error {
	return persist.SaveFile(settingsMetadata, &w.settings, filepath.Join(w.persistDir, settingsFile))
}

func (w *Wallet) loadSettings() error {
	return persist.LoadFile(settingsMetadata, &w.settings, settingsFile)
}

func (w *Wallet) initLog() error {
	logFile, err := os.OpenFile(filepath.Join(w.persistDir, logFile), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	w.log = log.New(logFile, "", modules.LogSettings)
	w.log.Println("STARTUP: Wallet logging has started.")
	return nil
}

func (w *Wallet) initSettings() error {
	// Check if the settings file exists, if not create it.
	settingsFilename := filepath.Join(w.persistDir, settingsFile)
	_, err := os.Stat(settingsFilename)
	if os.IsNotExist(err) {
		return w.saveSettings()
	} else if err != nil {
		return err
	}

	// Load the settings file if it does exist.
	return w.loadSettings()
}

func (w *Wallet) initPersist() error {
	// Create a directory for the wallet without overwriting an existing
	// directory.
	err := os.MkdirAll(w.persistDir, 0700)
	if err != nil {
		return err
	}

	// Start logging.
	err = w.initLog()
	if err != nil {
		return err
	}

	err = w.initSettings()
	if err != nil {
		return err
	}
	return nil
}

// encryptionVerificationKey turns the master key in to a verification key.
func encryptionVerificationKey(masterKey crypto.TwofishKey) crypto.TwofishKey {
	keyBase := append(masterKey[:], encryptionTestModifier[:]...)
	return crypto.TwofishKey(crypto.HashObject(keyBase))
}

// seedKey turns the master key into a seed key.
func seedKey(masterKey crypto.TwofishKey, seedFilename string) crypto.TwofishKey {
	modifier := append(seedModifier[:], []byte(seedFilename)...)
	keyBase := append(masterKey[:], modifier[:]...)
	return crypto.TwofishKey(crypto.HashObject(keyBase))
}

// checkEncryptionKey checks that the correct encryption key was used.
func (w *Wallet) checkEncryptionKey(masterKey crypto.TwofishKey) error {
	verificationKey := encryptionVerificationKey(masterKey)
	decryptedBytes, err := verificationKey.DecryptBytes(w.settings.EncryptionVerification)
	if err != nil {
		return err
	}
	expected := make([]byte, encryptionVerificationLen)
	if bytes.Equal(expected, decryptedBytes) {
		return errBadEncryptionKey
	}
	return nil
}

func (w *Wallet) initEncryption(masterKey crypto.TwofishKey) error {
	// Check if the wallet encryption key has already been set.
	encryptionBase := make([]byte, encryptionVerificationLen)
	if !bytes.Equal(w.settings.EncryptionVerification, encryptionBase) {
		return w.checkEncryptionKey(masterKey)
	}

	// Encryption key has not been created yet - create it.
	var err error
	verificationKey := encryptionVerificationKey(masterKey)
	w.settings.EncryptionVerification, err = verificationKey.EncryptBytes(encryptionBase)
	if err != nil {
		return err
	}
	return w.saveSettings()
}

func (w *Wallet) loadSeed(fileInfo os.FileInfo, masterKey crypto.TwofishKey) error {
	// Load the seed.
	var seedFile SeedFile
	err := persist.LoadFile(seedMetadata, &seedFile, fileInfo.Name())
	if err != nil {
		return err
	}

	// Check that the master key is correct.
	key := seedKey(masterKey, fileInfo.Name())
	expected := make([]byte, encryptionVerificationLen)
	decryptedBytes, err := key.DecryptBytes(seedFile.EncryptionVerification)
	if err != nil {
		return err
	}
	if !bytes.Equal(decryptedBytes, expected) {
		return errBadEncryptionKey
	}

	// TODO: Generate the addresses and move them into memory.
	return nil
}

func (w *Wallet) createSeed(masterKey crypto.TwofishKey) error {
	var seedFile SeedFile
	filename := persist.RandomSuffix() + seedFileSuffix
	key := seedKey(masterKey, filename)
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
			err = w.loadSeed(fileInfo, masterKey)
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

// unlock loads all of the encrypted file structures into wallet memory. Even
// after loading, the structures are kept encrypted, but some data such as
// addresses are decrypted so that the wallet knows what to track.
func (w *Wallet) unlock(masterKey crypto.TwofishKey) error {
	// Wallet only needs to be unlocked once.
	if w.unlocked {
		return errAlreadyUnlocked
	}

	// Initialize the encryption of the wallet.
	err := w.initEncryption(masterKey)
	if err != nil {
		return err
	}

	// Handle scanning and creating wallet seeds.
	err = w.initWalletSeeds(masterKey)
	if err != nil {
		return err
	}

	// Load all special files.

	w.unlocked = true
	return nil
}
