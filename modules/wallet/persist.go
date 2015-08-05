package wallet

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	logFile        = modules.WalletDir + ".log"
	settingsFile   = modules.WalletDir + ".json"
	seedFilePrefix = "Sia Wallet Seed - "
	seedFileSuffix = ".seed"

	encryptionVerificationLen = 32
)

var (
	errAlreadyUnlocked  = errors.New("wallet has already been unlocked")
	errBadEncryptionKey = errors.New("provided encryption key is incorrect")

	settingsMetadata = persist.Metadata{"Wallet Settings", "0.4.0"}
	seedMetadata     = persist.Metadata{"Wallet Seed", "0.4.0"}

	unlockModifier = types.Specifier{'u', 'n', 'l', 'o', 'c', 'k'}
)

type WalletSettings struct {
	// EncryptionVerification is an encrypted string that, when decrypted, is
	// 32 '0' bytes.
	EncryptionVerification crypto.Ciphertext

	PrimarySeed     string // Name of the primary seed for the wallet.
	AddressProgress uint64 // Number of addresses used in the primary seed.
}

// saveSettings writes the wallet's settings to the wallet's settings file,
// replacing the existing file.
func (w *Wallet) saveSettings() error {
	return persist.SaveFile(settingsMetadata, w.settings, filepath.Join(w.persistDir, settingsFile))
}

// loadSettings reads the wallet's settings from the wallet's settings file,
// overwriting the settings object in memory. loadSettings should only be
// called at startup.
func (w *Wallet) loadSettings() error {
	return persist.LoadFile(settingsMetadata, w.settings, filepath.Join(w.persistDir, settingsFile))
}

// initLog begins logging the wallet, appending to any existing wallet file and
// writing a startup message to indicate that a new logging instance has been
// created.
func (w *Wallet) initLog() error {
	logFile, err := os.OpenFile(filepath.Join(w.persistDir, logFile), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	w.log = log.New(logFile, "", modules.LogSettings)
	w.log.Println("STARTUP: Wallet logging has started.")
	return nil
}

// initSettings creates the settings object at startup. If a settings file
// exists, the settings file will be loaded into memory. If the settings file
// does not exist, a new settings file will be created.
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

// initPersist loads all of the wallet's persistence files into memory,
// creating them if they do not exist.
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

	// Load the settings file.
	err = w.initSettings()
	if err != nil {
		return err
	}
	return nil
}

// unlockingKey creates a wallet unlocking key from the input master key.
func unlockingKey(masterKey crypto.TwofishKey) crypto.TwofishKey {
	keyBase := append(masterKey[:], unlockModifier[:]...)
	return crypto.TwofishKey(crypto.HashObject(keyBase))
}

// checkUnlockingKey verifies that the unlocking key provided to unlock the
// wallet matches the unlocking key given to the wallet.
func (w *Wallet) checkUnlockingKey(masterKey crypto.TwofishKey) error {
	verificationKey := unlockingKey(masterKey)
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

// initEncryption checks that the provided encryption key is the valid
// encryption key for the wallet. If encryption has not yet been established
// for the wallet, an encryption key is created.
func (w *Wallet) initEncryption(masterKey crypto.TwofishKey) error {
	// Check if the wallet encryption key has already been set.
	if len(w.settings.EncryptionVerification) != 0 {
		return w.checkUnlockingKey(masterKey)
	}

	// Encryption key has not been created yet - create it.
	var err error
	unlockingKey := unlockingKey(masterKey)
	encryptionBase := make([]byte, encryptionVerificationLen)
	w.settings.EncryptionVerification, err = unlockingKey.EncryptBytes(encryptionBase)
	if err != nil {
		return err
	}
	return w.saveSettings()
}

// initPrimarySeed loads the primary seed into the wallet, creating a new one
// if the primary seed does not exist. The primary seed is used to generate new
// addresses.
func (w *Wallet) initPrimarySeed(masterKey crypto.TwofishKey) error {
	if w.settings.PrimarySeed == "" {
		w.log.Println("UNLOCK: Primary seed undefined, creating a new seed.")
		return w.createSeed(masterKey)
	}
	fileInfo, err := os.Stat(filepath.Join(w.persistDir, w.settings.PrimarySeed))
	if err != nil {
		w.log.Println("UNLOCK: Issue loading primary seed file:", err)
		return err
	}
	err = w.loadSeedFile(masterKey, fileInfo)
	if err != nil {
		w.log.Println("UNLOCK: Issue loading primary seed:", err)
		return err
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

	// Load all special files.

	w.unlocked = true
	return nil
}

// Unlock will decrypt the wallet seed and load all of the addresses into
// memory.
func (w *Wallet) Unlock(masterKey crypto.TwofishKey) error {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)
	return w.unlock(masterKey)
}
