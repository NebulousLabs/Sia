package wallet

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/fastrand"

	"github.com/coreos/bbolt"
)

const (
	compatFile = modules.WalletDir + ".json"
	dbFile     = modules.WalletDir + ".db"
	logFile    = modules.WalletDir + ".log"
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
		// check whether we need to init bucketAddrTransactions
		buildAddrTxns := tx.Bucket(bucketAddrTransactions) == nil
		// ensure that all buckets exist
		for _, b := range dbBuckets {
			_, err := tx.CreateBucketIfNotExists(b)
			if err != nil {
				return fmt.Errorf("could not create bucket %v: %v", string(b), err)
			}
		}
		// if the wallet does not have a UID, create one
		if tx.Bucket(bucketWallet).Get(keyUID) == nil {
			uid := make([]byte, len(uniqueID{}))
			fastrand.Read(uid[:])
			tx.Bucket(bucketWallet).Put(keyUID, uid)
		}
		// if fields in bucketWallet are nil, set them to zero to prevent unmarshal errors
		wb := tx.Bucket(bucketWallet)
		if wb.Get(keyConsensusHeight) == nil {
			wb.Put(keyConsensusHeight, encoding.Marshal(uint64(0)))
		}
		if wb.Get(keyAuxiliarySeedFiles) == nil {
			wb.Put(keyAuxiliarySeedFiles, encoding.Marshal([]seedFile{}))
		}
		if wb.Get(keySpendableKeyFiles) == nil {
			wb.Put(keySpendableKeyFiles, encoding.Marshal([]spendableKeyFile{}))
		}
		if wb.Get(keySiafundPool) == nil {
			wb.Put(keySiafundPool, encoding.Marshal(types.ZeroCurrency))
		}

		// build the bucketAddrTransactions bucket if necessary
		if buildAddrTxns {
			it := dbProcessedTransactionsIterator(tx)
			for it.next() {
				index, pt := it.key(), it.value()
				if err := dbAddProcessedTransactionAddrs(tx, pt, index); err != nil {
					return err
				}
			}
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
	dbFilename := filepath.Join(w.persistDir, dbFile)
	compatFilename := filepath.Join(w.persistDir, compatFile)
	_, dbErr := os.Stat(dbFilename)
	_, compatErr := os.Stat(compatFilename)
	if dbErr != nil && compatErr == nil {
		// database does not exist, but old persist does; convert it
		err = w.convertPersistFrom112To120(dbFilename, compatFilename)
	} else {
		// either database exists or neither exists; open/create the database
		err = w.openDB(filepath.Join(w.persistDir, dbFile))
	}
	if err != nil {
		return err
	}
	err = w.tg.AfterStop(func() error {
		var err error
		if w.dbRollback {
			// rollback txn if necessry.
			err = errors.New("database unable to sync - rollback requested")
			err = errors.Compose(err, w.dbTx.Rollback())
		} else {
			// else commit the transaction.
			err = w.dbTx.Commit()
		}
		if err != nil {
			w.log.Severe("ERROR: failed to apply database update:", err)
			return errors.AddContext(err, "unable to commit dbTx in syncDB")
		}
		return w.db.Close()
	})
	if err != nil {
		return err
	}
	go w.threadedDBUpdate()
	return nil
}

// createBackup copies the wallet database to dst.
func (w *Wallet) createBackup(dst io.Writer) error {
	_, err := w.dbTx.WriteTo(dst)
	return err
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

// compat112Persist is the structure of the wallet.json file used in v1.1.2
type compat112Persist struct {
	UID                    uniqueID
	EncryptionVerification crypto.Ciphertext
	PrimarySeedFile        seedFile
	PrimarySeedProgress    uint64
	AuxiliarySeedFiles     []seedFile
	UnseededKeys           []spendableKeyFile
}

// compat112Meta is the metadata of the wallet.json file used in v1.1.2
var compat112Meta = persist.Metadata{
	Header:  "Wallet Settings",
	Version: "0.4.0",
}

// convertPersistFrom112To120 converts an old (pre-v1.2.0) wallet.json file to
// a wallet.db database.
func (w *Wallet) convertPersistFrom112To120(dbFilename, compatFilename string) error {
	var data compat112Persist
	err := persist.LoadJSON(compat112Meta, &data, compatFilename)
	if err != nil {
		return err
	}

	w.db, err = persist.OpenDatabase(dbMetadata, dbFilename)
	if err != nil {
		return err
	}
	// initialize the database
	err = w.db.Update(func(tx *bolt.Tx) error {
		for _, b := range dbBuckets {
			_, err := tx.CreateBucket(b)
			if err != nil {
				return fmt.Errorf("could not create bucket %v: %v", string(b), err)
			}
		}
		// set UID, verification, seeds, and seed progress
		tx.Bucket(bucketWallet).Put(keyUID, data.UID[:])
		tx.Bucket(bucketWallet).Put(keyEncryptionVerification, data.EncryptionVerification)
		tx.Bucket(bucketWallet).Put(keyPrimarySeedFile, encoding.Marshal(data.PrimarySeedFile))
		tx.Bucket(bucketWallet).Put(keyAuxiliarySeedFiles, encoding.Marshal(data.AuxiliarySeedFiles))
		tx.Bucket(bucketWallet).Put(keySpendableKeyFiles, encoding.Marshal(data.UnseededKeys))
		// old wallets had a "preload depth" of 25
		dbPutPrimarySeedProgress(tx, data.PrimarySeedProgress+25)

		// set consensus height and CCID to zero so that a full rescan is
		// triggered
		dbPutConsensusHeight(tx, 0)
		dbPutConsensusChangeID(tx, modules.ConsensusChangeBeginning)
		return nil
	})
	w.encrypted = true
	return err
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
