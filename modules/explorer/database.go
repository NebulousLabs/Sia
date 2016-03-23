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
// transaction IDs into a slice.
func getTransactionIDSet(bucket *bolt.Bucket) (ids []types.TransactionID, err error) {
	err = bucket.ForEach(func(txid, _ []byte) error {
		var id types.TransactionID
		err := encoding.Unmarshal(txid, &id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	return
}

// dbGetBlockHeight retrieves the block height of the specified block ID.
func dbGetBlockHeight(tx *bolt.Tx, id types.BlockID) (height types.BlockHeight, err error) {
	err = getAndDecode(tx.Bucket(bucketBlockHashes), id, &height)
	return
}

// dbGetBlockFacts returns the blockFacts at a specified height.
func dbGetBlockFacts(tx *bolt.Tx, height types.BlockHeight) (facts blockFacts, err error) {
	b := tx.Bucket(bucketBlockFacts)
	err = getAndDecode(b, height, &facts)
	if err != nil {
		return
	}
	// also look up the maturity timestamp, if possible
	// TODO: does this make sense? Why not set maturityTimestamp elsewhere, like in update.go?
	if height > types.MaturityDelay {
		var bf2 blockFacts
		err = getAndDecode(b, height, &bf2)
		if err != nil {
			return
		}
		facts.maturityTimestamp = bf2.timestamp
	}
	return
}

// dbGetTransactionHeight returns the height at which a specified transaction
// appeared.
func dbGetTransactionHeight(tx *bolt.Tx, id types.TransactionID) (height types.BlockHeight, err error) {
	err = getAndDecode(tx.Bucket(bucketTransactionHashes), id, &height)
	return
}

// dbGetUnlockHashTxnIDs returns the IDs of all the transactions that contain
// the specified unlock hash.
func dbGetUnlockHashTxnIDs(tx *bolt.Tx, uh types.UnlockHash) (ids []types.TransactionID, err error) {
	hashBucket := tx.Bucket(bucketUnlockHashes).Bucket(encoding.Marshal(uh))
	if hashBucket == nil {
		return nil, errNotExist
	}
	err = hashBucket.ForEach(func(txid, _ []byte) error {
		var id types.TransactionID
		err := encoding.Unmarshal(txid, &id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	return
}

// dbGetSiacoinOutput will return the siacoin output associated with the specified ID.
func dbGetSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) (sco types.SiacoinOutput, err error) {
	err = getAndDecode(tx.Bucket(bucketSiacoinOutputs), id, &sco)
	return
}

// dbGetSiacoinOutputTxnIDs returns all of the transactions that contain the
// specified siacoin output ID.
func dbGetSiacoinOutputTxnIDs(tx *bolt.Tx, id types.SiacoinOutputID) (ids []types.TransactionID, err error) {
	scoBucket := tx.Bucket(bucketSiacoinOutputIDs).Bucket(encoding.Marshal(id))
	if scoBucket == nil {
		return nil, errNotExist
	}
	err = scoBucket.ForEach(func(txid, _ []byte) error {
		var id types.TransactionID
		err := encoding.Unmarshal(txid, &id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	return
}

// dbGetFileContractHistory returns the fileContractHistory associated with
// the specified ID.
func dbGetFileContractHistory(tx *bolt.Tx, id types.FileContractID) (history fileContractHistory, err error) {
	err = getAndDecode(tx.Bucket(bucketFileContractHistories), id, &history)
	return
}

// dbGetFileContractTxnIDs returns all of the transactions that contain the
// specified file contract ID.
func dbGetFileContractTxnIDs(tx *bolt.Tx, id types.FileContractID) (ids []types.TransactionID, err error) {
	fcBucket := tx.Bucket(bucketFileContractIDs).Bucket(encoding.Marshal(id))
	if fcBucket == nil {
		return nil, errNotExist
	}
	err = fcBucket.ForEach(func(txid, _ []byte) error {
		var id types.TransactionID
		err := encoding.Unmarshal(txid, &id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	return
}

// dbGetSiafundOutput will return the siafund output associated with the specified ID.
func dbGetSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) (sco types.SiafundOutput, err error) {
	err = getAndDecode(tx.Bucket(bucketSiafundOutputs), id, &sco)
	return
}

// dbGetSiafundOutputTxnIDs returns all of the transactions that contain the
// specified siafund output ID.
func dbGetSiafundOutputTxnIDs(tx *bolt.Tx, id types.SiafundOutputID) (ids []types.TransactionID, err error) {
	scoBucket := tx.Bucket(bucketSiafundOutputIDs).Bucket(encoding.Marshal(id))
	if scoBucket == nil {
		return nil, errNotExist
	}
	err = scoBucket.ForEach(func(txid, _ []byte) error {
		var id types.TransactionID
		err := encoding.Unmarshal(txid, &id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	})
	return
}
