package explorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"

	"github.com/coreos/bbolt"
)

var (
	// database buckets
	bucketBlockFacts            = []byte("BlockFacts")
	bucketBlockIDs              = []byte("BlockIDs")
	bucketBlocksDifficulty      = []byte("BlocksDifficulty")
	bucketBlockTargets          = []byte("BlockTargets")
	bucketFileContractHistories = []byte("FileContractHistories")
	bucketFileContractIDs       = []byte("FileContractIDs")
	// bucketInternal is used to store values internal to the explorer
	bucketInternal         = []byte("Internal")
	bucketSiacoinOutputIDs = []byte("SiacoinOutputIDs")
	bucketSiacoinOutputs   = []byte("SiacoinOutputs")
	bucketSiafundOutputIDs = []byte("SiafundOutputIDs")
	bucketSiafundOutputs   = []byte("SiafundOutputs")
	bucketTransactionIDs   = []byte("TransactionIDs")
	bucketUnlockHashes     = []byte("UnlockHashes")

	errNotExist = errors.New("entry does not exist")

	// keys for bucketInternal
	internalBlockHeight  = []byte("BlockHeight")
	internalRecentChange = []byte("RecentChange")
)

// These functions all return a 'func(*bolt.Tx) error', which, allows them to
// be called concisely with the db.View and db.Update functions, e.g.:
//
//    var height types.BlockHeight
//    db.View(dbGetAndDecode(bucketBlockIDs, id, &height))
//
// Instead of:
//
//   var height types.BlockHeight
//   db.View(func(tx *bolt.Tx) error {
//       bytes := tx.Bucket(bucketBlockIDs).Get(encoding.Marshal(id))
//       return encoding.Unmarshal(bytes, &height)
//   })

// dbGetAndDecode returns a 'func(*bolt.Tx) error' that retrieves and decodes
// a value from the specified bucket. If the value does not exist,
// dbGetAndDecode returns errNotExist.
func dbGetAndDecode(bucket []byte, key, val interface{}) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		valBytes := tx.Bucket(bucket).Get(encoding.Marshal(key))
		if valBytes == nil {
			return errNotExist
		}
		return encoding.Unmarshal(valBytes, val)
	}
}

// dbGetTransactionIDSet returns a 'func(*bolt.Tx) error' that decodes a
// bucket of transaction IDs into a slice. If the bucket is nil,
// dbGetTransactionIDSet returns errNotExist.
func dbGetTransactionIDSet(bucket []byte, key interface{}, ids *[]types.TransactionID) func(*bolt.Tx) error {
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

// dbGetBlockFacts returns a 'func(*bolt.Tx) error' that decodes
// the block facts for `height` into blockfacts
func (e *Explorer) dbGetBlockFacts(height types.BlockHeight, bf *blockFacts) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		block, exists := e.cs.BlockAtHeight(height)
		if !exists {
			return errors.New("requested block facts for a block that does not exist")
		}
		return dbGetAndDecode(bucketBlockFacts, block.ID(), bf)(tx)
	}
}

// dbSetInternal sets the specified key of bucketInternal to the encoded value.
func dbSetInternal(key []byte, val interface{}) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return tx.Bucket(bucketInternal).Put(key, encoding.Marshal(val))
	}
}

// dbGetInternal decodes the specified key of bucketInternal into the supplied pointer.
func dbGetInternal(key []byte, val interface{}) func(*bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		return encoding.Unmarshal(tx.Bucket(bucketInternal).Get(key), val)
	}
}
