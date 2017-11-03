package explorer

import (
	"errors"
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
	"log"
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
	bucketHashType         = []byte("HashType")

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

// helper functions
func assertNil(err error) {
	if err != nil && build.DEBUG {
		panic(err)
	} else if err != nil {
		log.Printf("Error asserting.  Found non-nil error: %s", err)
	}
}
func mustPut(bucket *bolt.Bucket, key, val interface{}) {
	assertNil(bucket.Put(encoding.Marshal(key), encoding.Marshal(val)))
}
func mustPutSet(bucket *bolt.Bucket, key interface{}) {
	assertNil(bucket.Put(encoding.Marshal(key), nil))
}
func mustDelete(bucket *bolt.Bucket, key interface{}) {
	assertNil(bucket.Delete(encoding.Marshal(key)))
}
func bucketIsEmpty(bucket *bolt.Bucket) bool {
	k, _ := bucket.Cursor().First()
	return k == nil
}

// These functions panic on error in debug mode.

// Add/Remove block ID
func dbAddBlockID(tx *bolt.Tx, id types.BlockID, height types.BlockHeight) {
	mustPut(tx.Bucket(bucketHashType), id, modules.BlockHash)
	mustPut(tx.Bucket(bucketBlockIDs), id, height)
}
func dbRemoveBlockID(tx *bolt.Tx, id types.BlockID) {
	mustDelete(tx.Bucket(bucketHashType), id)
	mustDelete(tx.Bucket(bucketBlockIDs), id)
}

// Add/Remove block facts
func dbAddBlockFacts(tx *bolt.Tx, facts blockFacts) {
	mustPut(tx.Bucket(bucketBlockFacts), facts.BlockID, facts)
}
func dbRemoveBlockFacts(tx *bolt.Tx, id types.BlockID) {
	mustDelete(tx.Bucket(bucketBlockFacts), id)
}

// Add/Remove block target
func dbAddBlockTarget(tx *bolt.Tx, id types.BlockID, target types.Target) {
	mustPut(tx.Bucket(bucketBlockTargets), id, target)
}
func dbRemoveBlockTarget(tx *bolt.Tx, id types.BlockID, target types.Target) {
	mustDelete(tx.Bucket(bucketBlockTargets), id)
}

// Add/Remove file contract
func dbAddFileContract(tx *bolt.Tx, id types.FileContractID, fc types.FileContract) {
	mustPut(tx.Bucket(bucketHashType), id, modules.FileContractId)
	history := fileContractHistory{Contract: fc}
	mustPut(tx.Bucket(bucketFileContractHistories), id, history)
}
func dbRemoveFileContract(tx *bolt.Tx, id types.FileContractID) {
	mustDelete(tx.Bucket(bucketHashType), id)
	mustDelete(tx.Bucket(bucketFileContractHistories), id)
}

// Add/Remove txid from file contract ID bucket
func dbAddFileContractID(tx *bolt.Tx, id types.FileContractID, txid types.TransactionID) {
	b, err := tx.Bucket(bucketFileContractIDs).CreateBucketIfNotExists(encoding.Marshal(id))
	assertNil(err)
	mustPutSet(b, txid)
}

func dbRemoveFileContractID(tx *bolt.Tx, id types.FileContractID, txid types.TransactionID) {
	bucket := tx.Bucket(bucketFileContractIDs).Bucket(encoding.Marshal(id))
	mustDelete(bucket, txid)
	if bucketIsEmpty(bucket) {
		tx.Bucket(bucketFileContractIDs).DeleteBucket(encoding.Marshal(id))
	}
}

func dbAddFileContractRevision(tx *bolt.Tx, fcid types.FileContractID, fcr types.FileContractRevision) {
	var history fileContractHistory
	assertNil(dbGetAndDecode(bucketFileContractHistories, fcid, &history)(tx))
	history.Revisions = append(history.Revisions, fcr)
	mustPut(tx.Bucket(bucketFileContractHistories), fcid, history)
}
func dbRemoveFileContractRevision(tx *bolt.Tx, fcid types.FileContractID) {
	var history fileContractHistory
	assertNil(dbGetAndDecode(bucketFileContractHistories, fcid, &history)(tx))
	// TODO: could be more rigorous
	history.Revisions = history.Revisions[:len(history.Revisions)-1]
	mustPut(tx.Bucket(bucketFileContractHistories), fcid, history)
}

// Add/Remove siacoin output
func dbAddSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID, output types.SiacoinOutput) {
	mustPut(tx.Bucket(bucketHashType), id, modules.SiacoinOutputId)
	mustPut(tx.Bucket(bucketSiacoinOutputs), id, output)
}
func dbRemoveSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) {
	mustDelete(tx.Bucket(bucketHashType), id)
	mustDelete(tx.Bucket(bucketSiacoinOutputs), id)
}

// Add/Remove txid from siacoin output ID bucket
func dbAddSiacoinOutputID(tx *bolt.Tx, id types.SiacoinOutputID, txid types.TransactionID) {
	b, err := tx.Bucket(bucketSiacoinOutputIDs).CreateBucketIfNotExists(encoding.Marshal(id))
	assertNil(err)
	mustPutSet(b, txid)
}
func dbRemoveSiacoinOutputID(tx *bolt.Tx, id types.SiacoinOutputID, txid types.TransactionID) {
	bucket := tx.Bucket(bucketSiacoinOutputIDs).Bucket(encoding.Marshal(id))
	mustDelete(bucket, txid)
	if bucketIsEmpty(bucket) {
		tx.Bucket(bucketSiacoinOutputIDs).DeleteBucket(encoding.Marshal(id))
	}
}

// Add/Remove siafund output
func dbAddSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID, output types.SiafundOutput) {
	mustPut(tx.Bucket(bucketHashType), id, modules.SiafundOutputId)
	mustPut(tx.Bucket(bucketSiafundOutputs), id, output)
}
func dbRemoveSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) {
	mustDelete(tx.Bucket(bucketHashType), id)
	mustDelete(tx.Bucket(bucketSiafundOutputs), id)
}

// Add/Remove txid from siafund output ID bucket
func dbAddSiafundOutputID(tx *bolt.Tx, id types.SiafundOutputID, txid types.TransactionID) {
	b, err := tx.Bucket(bucketSiafundOutputIDs).CreateBucketIfNotExists(encoding.Marshal(id))
	assertNil(err)
	mustPutSet(b, txid)
}
func dbRemoveSiafundOutputID(tx *bolt.Tx, id types.SiafundOutputID, txid types.TransactionID) {
	bucket := tx.Bucket(bucketSiafundOutputIDs).Bucket(encoding.Marshal(id))
	mustDelete(bucket, txid)
	if bucketIsEmpty(bucket) {
		tx.Bucket(bucketSiafundOutputIDs).DeleteBucket(encoding.Marshal(id))
	}
}

// Add/Remove storage proof
func dbAddStorageProof(tx *bolt.Tx, fcid types.FileContractID, sp types.StorageProof) {
	var history fileContractHistory
	assertNil(dbGetAndDecode(bucketFileContractHistories, fcid, &history)(tx))
	history.StorageProof = sp
	mustPut(tx.Bucket(bucketFileContractHistories), fcid, history)
}
func dbRemoveStorageProof(tx *bolt.Tx, fcid types.FileContractID) {
	dbAddStorageProof(tx, fcid, types.StorageProof{})
}

// Add/Remove transaction ID
func dbAddTransactionID(tx *bolt.Tx, id types.TransactionID, height types.BlockHeight) {
	mustPut(tx.Bucket(bucketTransactionIDs), id, height)
}
func dbRemoveTransactionID(tx *bolt.Tx, id types.TransactionID) {
	mustDelete(tx.Bucket(bucketTransactionIDs), id)
}

// Add/Remove txid from unlock hash bucket
func dbAddUnlockHash(tx *bolt.Tx, uh types.UnlockHash, txid types.TransactionID) {
	b, err := tx.Bucket(bucketUnlockHashes).CreateBucketIfNotExists(encoding.Marshal(uh))
	assertNil(err)
	mustPutSet(b, txid)
}
func dbRemoveUnlockHash(tx *bolt.Tx, uh types.UnlockHash, txid types.TransactionID) {
	bucket := tx.Bucket(bucketUnlockHashes).Bucket(encoding.Marshal(uh))
	mustDelete(bucket, txid)
	if bucketIsEmpty(bucket) {
		tx.Bucket(bucketUnlockHashes).DeleteBucket(encoding.Marshal(uh))
	}
}
