package transactionpool

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

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

	// Open the database file.
	tp.db, err = persist.OpenDatabase(dbMetadata, filepath.Join(tp.persistDir, dbFilename))
	if err != nil {
		return err
	}

	// Create the database and get the most recent consensus change.
	var cc modules.ConsensusChangeID
	err = tp.db.Update(func(tx *bolt.Tx) error {
		// Create the database buckets.
		buckets := [][]byte{
			bucketRecentConsensusChange,
			bucketConfirmedTransactions,
		}
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}

		// Get the recent consensus change.
		cc, err = tp.getRecentConsensusChange(tx)
		if err == errNilConsensusChange {
			return tp.putRecentConsensusChange(tx, modules.ConsensusChangeBeginning)
		}
		return err
	})
	if err != nil {
		return err
	}

	// Subscribe to the consensus set using the most recent consensus change.
	err = tp.consensusSet.ConsensusSetSubscribe(tp, cc)
	if err == modules.ErrInvalidConsensusChangeID {
		// Reset and rescan because the consensus set does not recognize the
		// provided consensus change id.
		resetErr := tp.db.Update(func(tx *bolt.Tx) error {
			return tp.resetDB(tx)
		})
		if resetErr != nil {
			return resetErr
		}
		return tp.consensusSet.ConsensusSetSubscribe(tp, modules.ConsensusChangeBeginning)
	}
	return err
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
