package transactionpool

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/coreos/bbolt"
)

const tpoolSyncRate = time.Minute * 2

// threadedRegularSync will make sure that sync gets called on the database
// every once in a while.
func (tp *TransactionPool) threadedRegularSync() {
	if err := tp.tg.Add(); err != nil {
		return
	}
	defer tp.tg.Done()
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
	// Begin a new tx
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
	err = tp.putRecentBlockID(tx, types.BlockID{})
	if err != nil {
		return err
	}
	err = tp.putRecentConsensusChange(tx, modules.ConsensusChangeBeginning)
	if err != nil {
		return err
	}
	err = tp.putBlockHeight(tx, types.BlockHeight(0))
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
		tp.mu.Lock()
		err := tp.dbTx.Commit()
		tp.mu.Unlock()
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
		bucketBlockHeight,
		bucketRecentConsensusChange,
		bucketConfirmedTransactions,
		bucketFeeMedian,
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

	// Get the most recent block height
	bh, err := tp.getBlockHeight(tp.dbTx)
	if err != nil {
		tp.log.Println("Block height is reporting as zero, setting up to subscribe from the beginning.")
		err = tp.putBlockHeight(tp.dbTx, types.BlockHeight(0))
		if err != nil {
			return build.ExtendErr("unable to initialize the block height in the tpool", err)
		}
		err = tp.putRecentConsensusChange(tp.dbTx, modules.ConsensusChangeBeginning)
	} else {
		tp.log.Debugln("Transaction pool is loading from height:", bh)
		tp.blockHeight = bh
	}
	if err != nil {
		return build.ExtendErr("unable to initialize the block height in the tpool", err)
	}

	// Get the fee median data.
	mp, err := tp.getFeeMedian(tp.dbTx)
	if err != nil && err != errNilFeeMedian {
		return build.ExtendErr("unable to load the fee median", err)
	}
	// Just leave the fields empty if no fee median was found. They will be
	// filled out.
	if err != errNilFeeMedian {
		tp.recentMedians = mp.RecentMedians
		tp.recentMedianFee = mp.RecentMedianFee
	}

	// Subscribe to the consensus set using the most recent consensus change.
	err = tp.consensusSet.ConsensusSetSubscribe(tp, cc, tp.tg.StopChan())
	if err == modules.ErrInvalidConsensusChangeID {
		tp.log.Println("Invalid consensus change loaded; resetting. This can take a while.")
		// Reset and rescan because the consensus set does not recognize the
		// provided consensus change id.
		resetErr := tp.resetDB(tp.dbTx)
		if resetErr != nil {
			return resetErr
		}
		freshScanErr := tp.consensusSet.ConsensusSetSubscribe(tp, modules.ConsensusChangeBeginning, tp.tg.StopChan())
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

// TransactionConfirmed returns true if the transaction has been seen on the
// blockchain. Note, however, that the block containing the transaction may
// later be invalidated by a reorg.
func (tp *TransactionPool) TransactionConfirmed(id types.TransactionID) bool {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.transactionConfirmed(tp.dbTx, id)
}

func (tp *TransactionPool) transactionConfirmed(tx *bolt.Tx, id types.TransactionID) bool {
	return tx.Bucket(bucketConfirmedTransactions).Get(id[:]) != nil
}
