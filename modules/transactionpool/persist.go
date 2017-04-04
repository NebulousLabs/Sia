package transactionpool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

const tpoolSyncRate = time.Minute * 2

var (
	// bucketRecentConsensusChange holds the most recent consensus change seen
	// by the transaction pool.
	bucketRecentConsensusChange = []byte("RecentConsensusChange")

	// bucketConfirmedTransactions holds the ids of every transaction that has
	// been confirmed on the blockchain.
	bucketConfirmedTransactions = []byte("ConfirmedTransactions")

	// errNilConsensusChange is returned if there is no consensus change in the
	// database.
	errNilConsensusChange = errors.New("no consensus change found")

	// fieldRecentConsensusChange is the field in bucketRecentConsensusChange
	// that holds the value of the most recent consensus change.
	fieldRecentConsensusChange = []byte("RecentConsensusChange")
)

// threadedRegularSync will make sure that sync gets called on the database
// every once in a while.
func (tp *TransactionPool) threadedRegularSync() {
	for {
		select {
		case <-tp.tg.StopChan():
			// A queued AfterStop will close out the db properly.
			return
		case <-time.After(tpoolSyncRate):
			tp.mu.Lock()
			tp.syncDB()
			tp.mu.Unlock()
		}
	}
}

// syncDB commits the current global transaction and immediately begins a new
// one.
func (tp *TransactionPool) syncDB() {
	// Commit the existing tx.
	err := tp.dbTx.Commit()
	if err != nil {
		tp.log.Severe("ERROR: failed to apply database update:", err)
		tp.dbTx.Rollback()
	}
	tp.dbTx, err = tp.db.Begin(true)
	if err != nil {
		tp.log.Severe("ERROR: failed to initialize a db transaction:", err)
	}
}

// resetDB deletes all consensus related persistence from the transaction pool.
func (tp *TransactionPool) resetDB(tx *bolt.Tx) error {
	err := tx.DeleteBucket(bucketConfirmedTransactions)
	if err != nil {
		return err
	}
	err = tp.putRecentConsensusChange(tx, modules.ConsensusChangeBeginning)
	if err != nil {
		return err
	}
	_, err = tx.CreateBucket(bucketConfirmedTransactions)
	return err
}

// initPersist creates buckets in the database
func (tp *TransactionPool) initPersist() error {
	// Create the persist directory if it does not yet exist.
	err := os.MkdirAll(tp.persistDir, 0700)
	if err != nil {
		return err
	}

	// Create the tpool logger.
	tp.log, err = persist.NewFileLogger(filepath.Join(tp.persistDir, logFile))
	if err != nil {
		return build.ExtendErr("unable to initialize the transaction pool logger", err)
	}
	tp.tg.AfterStop(func() {
		err := tp.log.Close()
		if err != nil {
			fmt.Println("Unable to close the transaction pool logger:", err)
		}
	})

	// Open the database file.
	tp.db, err = persist.OpenDatabase(dbMetadata, filepath.Join(tp.persistDir, dbFilename))
	if err != nil {
		return err
	}
	tp.tg.AfterStop(func() {
		err := tp.db.Close()
		if err != nil {
			tp.log.Println("Error while closing transaction pool database:", err)
		}
	})
	// Create the global tpool tx that will be used for most persist actions.
	tp.dbTx, err = tp.db.Begin(true)
	if err != nil {
		return build.ExtendErr("unable to begin tpool dbTx", err)
	}
	tp.tg.AfterStop(func() {
		err := tp.dbTx.Commit()
		if err != nil {
			tp.log.Println("Unable to close transaction properly during shutdown:", err)
		}
	})
	// Spin up the thread that occasionally syncrhonizes the database.
	go tp.threadedRegularSync()

	// Create the database and get the most recent consensus change.
	var cc modules.ConsensusChangeID
	// Create the database buckets.
	buckets := [][]byte{
		bucketRecentConsensusChange,
		bucketConfirmedTransactions,
	}
	for _, bucket := range buckets {
		_, err := tp.dbTx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return build.ExtendErr("unable to create the tpool buckets", err)
		}
	}

	// Get the recent consensus change.
	cc, err = tp.getRecentConsensusChange(tp.dbTx)
	if err == errNilConsensusChange {
		err = tp.putRecentConsensusChange(tp.dbTx, modules.ConsensusChangeBeginning)
	}
	if err != nil {
		return build.ExtendErr("unable to initialize the recent consensus change in the tpool", err)
	}

	// Subscribe to the consensus set using the most recent consensus change.
	err = tp.consensusSet.ConsensusSetSubscribe(tp, cc)
	if err == modules.ErrInvalidConsensusChangeID {
		// Reset and rescan because the consensus set does not recognize the
		// provided consensus change id.
		resetErr := tp.resetDB(tp.dbTx)
		if resetErr != nil {
			return resetErr
		}
		freshScanErr := tp.consensusSet.ConsensusSetSubscribe(tp, modules.ConsensusChangeBeginning)
		if freshScanErr != nil {
			return freshScanErr
		}
		tp.tg.OnStop(func() {
			tp.consensusSet.Unsubscribe(tp)
		})
		return nil
	}
	if err != nil {
		return err
	}
	tp.tg.OnStop(func() {
		tp.consensusSet.Unsubscribe(tp)
	})
	return nil
}

// getRecentConsensusChange returns the most recent consensus change from the
// database.
func (tp *TransactionPool) getRecentConsensusChange(tx *bolt.Tx) (cc modules.ConsensusChangeID, err error) {
	ccBytes := tx.Bucket(bucketRecentConsensusChange).Get(fieldRecentConsensusChange)
	if ccBytes == nil {
		return modules.ConsensusChangeID{}, errNilConsensusChange
	}
	copy(cc[:], ccBytes)
	return cc, nil
}

// putRecentConsensusChange updates the most recent consensus change seen by
// the transaction pool.
func (tp *TransactionPool) putRecentConsensusChange(tx *bolt.Tx, cc modules.ConsensusChangeID) error {
	return tx.Bucket(bucketRecentConsensusChange).Put(fieldRecentConsensusChange, cc[:])
}

// transactionConfirmed returns true if the transaction has been confirmed on
// the blockchain and false if the transaction has not been confirmed on the
// blockchain.
func (tp *TransactionPool) transactionConfirmed(tx *bolt.Tx, id types.TransactionID) bool {
	confirmedBytes := tx.Bucket(bucketConfirmedTransactions).Get(id[:])
	if confirmedBytes == nil {
		return false
	}
	return true
}

// addTransaction adds a transaction to the list of confirmed transactions.
func (tp *TransactionPool) addTransaction(tx *bolt.Tx, id types.TransactionID) error {
	return tx.Bucket(bucketConfirmedTransactions).Put(id[:], []byte{})
}

// deleteTransaction deletes a transaction from the list of confirmed
// transactions.
func (tp *TransactionPool) deleteTransaction(tx *bolt.Tx, id types.TransactionID) error {
	return tx.Bucket(bucketConfirmedTransactions).Delete(id[:])
}
