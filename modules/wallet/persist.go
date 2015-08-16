package wallet

import (
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	logFile        = modules.WalletDir + ".log"
	settingsFile   = modules.WalletDir + ".json"
	seedFilePrefix = "Sia Wallet Seed - "
	seedFileSuffix = ".seed"

	encryptionVerificationLen = 32
)

var (
	settingsMetadata = persist.Metadata{"Wallet Settings", "0.4.0"}
	seedMetadata     = persist.Metadata{"Wallet Seed", "0.4.0"}
)

type WalletSettings struct {
	// EncryptionVerification is an encrypted string that, when decrypted, is
	// 32 '0' bytes.
	EncryptionVerification crypto.Ciphertext

	// The primary seed is used to generate new addresses as they are required.
	// All addresses are tracked and spendable. Only modules.PublicKeysPerSeed
	// keys/addresses can be created per seed, after which a new seed will need
	// to be generated.
	PrimarySeedFile     SeedFile
	PrimarySeedProgress uint64
	PrimarySeedFilename string
}

// unlockKey creates a wallet unlocking key from the input master key.
func unlockKey(masterKey crypto.TwofishKey) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, unlockModifier))
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
	return persist.LoadFile(settingsMetadata, &w.settings, filepath.Join(w.persistDir, settingsFile))
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
