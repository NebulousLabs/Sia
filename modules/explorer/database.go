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
	bucketBlockIDs              = []byte("BlockIDs")
	bucketBlocksDifficulty      = []byte("BlocksDifficulty")
	bucketBlockTargets          = []byte("BlockTargets")
	bucketFileContractHistories = []byte("FileContractHistories")
	bucketFileContractIDs       = []byte("FileContractIDs")
	bucketSiacoinOutputIDs      = []byte("SiacoinOutputIDs")
	bucketSiacoinOutputs        = []byte("SiacoinOutputs")
	bucketSiafundOutputIDs      = []byte("SiafundOutputIDs")
	bucketSiafundOutputs        = []byte("SiafundOutputs")
	bucketTransactionIDs        = []byte("TransactionIDs")
	bucketUnlockHashes          = []byte("UnlockHashes")

	// bucketInternal is used to store values internal to the explorer
	bucketInternal = []byte("Internal")

	// keys for bucketInternal
	internalBlockHeight  = []byte("BlockHeight")
	internalDifficulty   = []byte("Difficulty")
	internalRecentChange = []byte("RecentChange")
)

// getAndDecode returns a 'func(*bolt.Tx) error' that retrieves and decodes a
// value from the specified bucket. If the value does not exist, getAndDecode
// returns errNotExist.
func getAndDecode(bucket []byte, key, val interface{}) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		valBytes := tx.Bucket(bucket).Get(encoding.Marshal(key))
		if valBytes == nil {
			return errNotExist
		}
		return encoding.Unmarshal(valBytes, val)
	}
}

// getTransactionIDSet returns a 'func(*bolt.Tx) error' that decodes a bucket
// of transaction IDs into a slice. If the bucket is nil, getTransactionIDSet
// returns errNotExist.
func getTransactionIDSet(bucket []byte, key interface{}, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket).Bucket(encoding.Marshal(key))
		if b == nil {
			return errNotExist
		}
		// decode into a local slice
		var txids []types.TransactionID
		err := b.ForEach(func(txid, _ []byte) error {
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
	return getAndDecode(bucketBlockIDs, id, height)
}

// dbGetBlockFacts retrieves the blockFacts of the specified block ID.
func dbGetBlockFacts(id types.BlockID, facts *blockFacts) func(*bolt.Tx) error {
	return getAndDecode(bucketBlockFacts, id, facts)
}

// dbGetTransactionHeight retrieves the height at which a specified
// transaction appeared.
func dbGetTransactionHeight(id types.TransactionID, height *types.BlockHeight) func(*bolt.Tx) error {
	return getAndDecode(bucketTransactionIDs, id, height)
}

// dbGetUnlockHashTxnIDs retrieves the IDs of all the transactions that
// contain the specified unlock hash.
func dbGetUnlockHashTxnIDs(uh types.UnlockHash, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return getTransactionIDSet(bucketUnlockHashes, uh, ids)
}

// dbGetSiacoinOutput will return the siacoin output associated with the specified ID.
func dbGetSiacoinOutput(id types.SiacoinOutputID, sco *types.SiacoinOutput) func(*bolt.Tx) error {
	return getAndDecode(bucketSiacoinOutputs, id, &sco)
}

// dbGetSiacoinOutputTxnIDs returns all of the transactions that contain the
// specified siacoin output ID.
func dbGetSiacoinOutputTxnIDs(id types.SiacoinOutputID, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return getTransactionIDSet(bucketSiacoinOutputIDs, id, ids)
}

// dbGetFileContractHistory returns the fileContractHistory associated with
// the specified ID.
func dbGetFileContractHistory(id types.FileContractID, history *fileContractHistory) func(*bolt.Tx) error {
	return getAndDecode(bucketFileContractHistories, id, &history)
}

// dbGetFileContractTxnIDs returns all of the transactions that contain the
// specified file contract ID.
func dbGetFileContractTxnIDs(id types.FileContractID, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return getTransactionIDSet(bucketFileContractIDs, id, ids)
}

// dbGetSiafundOutput will return the siafund output associated with the specified ID.
func dbGetSiafundOutput(id types.SiafundOutputID, sco *types.SiafundOutput) func(*bolt.Tx) error {
	return getAndDecode(bucketSiafundOutputs, id, &sco)
}

// dbGetSiafundOutputTxnIDs returns all of the transactions that contain the
// specified siafund output ID.
func dbGetSiafundOutputTxnIDs(id types.SiafundOutputID, ids *[]types.TransactionID) func(*bolt.Tx) error {
	return getTransactionIDSet(bucketSiafundOutputIDs, id, ids)
}

// Internal bucket getters/setters

func dbSetInternal(key []byte, val interface{}) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return tx.Bucket(bucketInternal).Put(key, encoding.Marshal(val))
	}
}
func dbGetInternal(key []byte, val interface{}) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return encoding.Unmarshal(tx.Bucket(bucketInternal).Get(key), val)
	}
}
