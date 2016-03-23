package explorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	errNotExist = errors.New("entry does not exist")

	// database buckets
	bucketBlockFacts            = []byte("BlockFacts")
	bucketBlockHashes           = []byte("BlockHashes")
	bucketBlocksDifficulty      = []byte("BlocksDifficulty")
	bucketBlockTargets          = []byte("BlockTargets")
	bucketFileContractHistories = []byte("FileContractHistories")
	bucketFileContractIDs       = []byte("FileContractIDs")
	bucketRecentChange          = []byte("RecentChange")
	bucketSiacoinOutputIDs      = []byte("SiacoinOutputIDs")
	bucketSiacoinOutputs        = []byte("SiacoinOutputs")
	bucketSiafundOutputIDs      = []byte("SiafundOutputIDs")
	bucketSiafundOutputs        = []byte("SiafundOutputs")
	bucketTransactionHashes     = []byte("TransactionHashes")
	bucketUnlockHashes          = []byte("UnlockHashes")
)

// getAndDecode is a helper function that retrieves and decodes a value from
// the specified bucket. If the value does not exist, getAndDecode returns
// errNotExist.
func getAndDecode(bucket *bolt.Bucket, key, val interface{}) error {
	valBytes := bucket.Get(encoding.Marshal(key))
	if valBytes == nil {
		return errNotExist
	}
	return encoding.Unmarshal(valBytes, val)
}

// getTransactionIDSet is a helper function that decodes a bucket of
// transaction IDs into a slice. If the bucket is nil, getTransactionIDSet
// returns errNotExist.
func getTransactionIDSet(bucket *bolt.Bucket, ids *[]types.TransactionID) error {
	if bucket == nil {
		return errNotExist
	}
	// decode into a local slice
	var txids []types.TransactionID
	err := bucket.ForEach(func(txid, _ []byte) error {
		var id types.TransactionID
		err := encoding.Unmarshal(txid, &id)
		if err != nil {
			return err
		}
		txids = append(txids, id)
		return nil
	})
	if err != nil {
		return err
	}
	// set pointer
	*ids = txids
	return nil
}

// These functions all return a 'func(*bolt.Tx) error', which, when called,
// decodes a value into a supplied pointer. This allows them to be called
// concisely with the db.View and db.Update functions, e.g.:
//
//    var height types.BlockHeight
//    db.View(dbGetBlockHeight(id, &height))
//
// Instead of:
//
//   var height types.BlockHeight
//   db.View(func(tx *bolt.Tx) error {
//       var err error
//       height, err = dbGetBlockHeight(tx, id)
//       return err
//   })

// dbGetBlockHeight retrieves the block height of the specified block ID.
func dbGetBlockHeight(id types.BlockID, height *types.BlockHeight) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getAndDecode(tx.Bucket(bucketBlockHashes), id, height)
	}
}

// dbGetBlockFacts retrieves the blockFacts at a specified height.
func dbGetBlockFacts(height types.BlockHeight, facts *blockFacts) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketBlockFacts)
		err := getAndDecode(b, height, facts)
		if err != nil {
			return err
		}
		// also look up the maturity timestamp, if possible
		// TODO: does this make sense? Why not set maturityTimestamp elsewhere, like in update.go?
		if height > types.MaturityDelay {
			var bf2 blockFacts
			err = getAndDecode(b, height, &bf2)
			if err != nil {
				return err
			}
			facts.maturityTimestamp = bf2.timestamp
		}
		return nil
	}
}

// dbGetTransactionHeight retrieves the height at which a specified
// transaction appeared.
func dbGetTransactionHeight(id types.TransactionID, height *types.BlockHeight) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getAndDecode(tx.Bucket(bucketTransactionHashes), id, height)
	}
}

// dbGetUnlockHashTxnIDs retrieves the IDs of all the transactions that
// contain the specified unlock hash.
func dbGetUnlockHashTxnIDs(uh types.UnlockHash, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getTransactionIDSet(tx.Bucket(bucketUnlockHashes).Bucket(encoding.Marshal(uh)), ids)
	}
}

// dbGetSiacoinOutput will return the siacoin output associated with the specified ID.
func dbGetSiacoinOutput(id types.SiacoinOutputID, sco *types.SiacoinOutput) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getAndDecode(tx.Bucket(bucketSiacoinOutputs), id, &sco)
	}
}

// dbGetSiacoinOutputTxnIDs returns all of the transactions that contain the
// specified siacoin output ID.
func dbGetSiacoinOutputTxnIDs(id types.SiacoinOutputID, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getTransactionIDSet(tx.Bucket(bucketSiacoinOutputIDs).Bucket(encoding.Marshal(id)), ids)
	}
}

// dbGetFileContractHistory returns the fileContractHistory associated with
// the specified ID.
func dbGetFileContractHistory(id types.FileContractID, history *fileContractHistory) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getAndDecode(tx.Bucket(bucketFileContractHistories), id, &history)
	}
}

// dbGetFileContractTxnIDs returns all of the transactions that contain the
// specified file contract ID.
func dbGetFileContractTxnIDs(id types.FileContractID, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getTransactionIDSet(tx.Bucket(bucketFileContractIDs).Bucket(encoding.Marshal(id)), ids)
	}
}

// dbGetSiafundOutput will return the siafund output associated with the specified ID.
func dbGetSiafundOutput(id types.SiafundOutputID, sco *types.SiafundOutput) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getAndDecode(tx.Bucket(bucketSiafundOutputs), id, &sco)
	}
}

// dbGetSiafundOutputTxnIDs returns all of the transactions that contain the
// specified siafund output ID.
func dbGetSiafundOutputTxnIDs(id types.SiafundOutputID, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return getTransactionIDSet(tx.Bucket(bucketSiafundOutputIDs).Bucket(encoding.Marshal(id)), ids)
	}
}
