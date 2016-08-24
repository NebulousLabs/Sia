package wallet

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"

	"github.com/NebulousLabs/bolt"
)

const (
	logFile = modules.WalletDir + ".log"
	dbFile  = modules.WalletDir + ".db"

	encryptionVerificationLen = 32
)

var (
	dbMetadata = persist.Metadata{
		Header:  "Wallet Database",
		Version: "1.1.0",
	}
)

// spendableKeyFile stores an encrypted spendable key on disk.
type spendableKeyFile struct {
	UID                    uniqueID
	EncryptionVerification crypto.Ciphertext
	SpendableKey           crypto.Ciphertext
}

// openDB loads the set database and populates it with the necessary buckets.
func (w *Wallet) openDB(filename string) (err error) {
	w.db, err = persist.OpenDatabase(dbMetadata, filename)
	if err != nil {
		return err
	}
	// initialize the database
	err = w.db.Update(func(tx *bolt.Tx) error {
		for _, b := range dbBuckets {
			_, err := tx.CreateBucketIfNotExists(b)
			if err != nil {
				return fmt.Errorf("could not create bucket %v: %v", string(b), err)
			}
		}
		// if the wallet does not have a UID, create one
		if tx.Bucket(bucketWallet).Get(keyUID) == nil {
			uid := make([]byte, len(uniqueID{}))
			_, err = rand.Read(uid[:])
			if err != nil {
				return fmt.Errorf("could not generate UID: %v", err)
			}
			tx.Bucket(bucketWallet).Put(keyUID, uid)
		}
		// check whether wallet is encrypted
		w.encrypted = tx.Bucket(bucketWallet).Get(keyEncryptionVerification) != nil
		return nil
	})
	return err
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
	w.log, err = persist.NewFileLogger(filepath.Join(w.persistDir, logFile))
	if err != nil {
		return err
	}

	// Open the database.
	err = w.openDB(filepath.Join(w.persistDir, dbFile))
	if err != nil {
		return err
	}
	w.tg.AfterStop(func() { w.db.Close() })

	return nil
}

// createBackup copies the wallet database to dst.
func (w *Wallet) createBackup(dst io.Writer) error {
	return w.db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(dst)
		return err
	})
}

// CreateBackup creates a backup file at the desired filepath.
func (w *Wallet) CreateBackup(backupFilepath string) error {
	if err := w.tg.Add(); err != nil {
		return err
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.Create(backupFilepath)
	if err != nil {
		return err
	}
	defer f.Close()
	return w.createBackup(f)
}

/*
// LoadBackup loads a backup file from the provided filepath. The backup file
// primary seed is loaded as an auxiliary seed.
func (w *Wallet) LoadBackup(masterKey, backupMasterKey crypto.TwofishKey, backupFilepath string) error {
	if err := w.tg.Add(); err != nil {
		return err
	}
	defer w.tg.Done()

	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	// Load all of the seed files, check for duplicates, re-encrypt them (but
	// keep the UID), and add them to the walletPersist object)
	var backupPersist walletPersist
	err := persist.LoadFile(settingsMetadata, &backupPersist, backupFilepath)
	if err != nil {
		return err
	}
	backupSeeds := append(backupPersist.AuxiliarySeedFiles, backupPersist.PrimarySeedFile)
	TODO: more
}
*/
