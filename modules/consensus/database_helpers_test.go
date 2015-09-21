package consensus

// database_test.go contains a bunch of legacy functions to preserve
// compatibility with the test suite.

import (
	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// dbBlockHeight is a convenience function allowing blockHeight to be called
// without a bolt.Tx.
func (cs *ConsensusSet) dbBlockHeight() (bh types.BlockHeight) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		bh = blockHeight(tx)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return bh
}

// dbCurrentBlockID is a convenience function allowing currentBlockID to be
// called without a bolt.Tx.
func (cs *ConsensusSet) dbCurrentBlockID() (id types.BlockID) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		id = currentBlockID(tx)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return id
}

// dbCurrentProcessedBlock is a convenience function allowing
// currentProcessedBlock to be called without a bolt.Tx.
func (cs *ConsensusSet) dbCurrentProcessedBlock() (pb *processedBlock) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		pb = currentProcessedBlock(tx)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return pb
}

// dbGetPath is a convenience function allowing getPath to be called without a
// bolt.Tx.
func (cs *ConsensusSet) dbGetPath(bh types.BlockHeight) (id types.BlockID, err error) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		id, err = getPath(tx, bh)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return id, err
}

// dbGetBlockMap is a convenience function allowing getBlockMap to be called
// without a bolt.Tx.
func (cs *ConsensusSet) dbGetBlockMap(id types.BlockID) (pb *processedBlock, err error) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		pb, err = getBlockMap(tx, id)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return pb, err
}

// dbGetSiacoinOutput is a convenience function allowing getSiacoinOutput to be
// called without a bolt.Tx.
func (cs *ConsensusSet) dbGetSiacoinOutput(id types.SiacoinOutputID) (sco types.SiacoinOutput, err error) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		sco, err = getSiacoinOutput(tx, id)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return sco, err
}

// dbGetFileContract is a convenience function allowing getFileContract to be
// called without a bolt.Tx.
func (cs *ConsensusSet) dbGetFileContract(id types.FileContractID) (fc types.FileContract, err error) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		fc, err = getFileContract(tx, id)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return fc, err
}

// dbGetSiafundPool is a convenience function allowing getSiafundPool to be
// called without a bolt.Tx.
func (cs *ConsensusSet) dbGetSiafundPool() (siafundPool types.Currency) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		siafundPool = getSiafundPool(tx)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return siafundPool
}

// dbGetDSCO is a convenience function allowing a delayed siacoin output to be
// fetched without a bolt.Tx. An error is returned if the delayed output is not
// found at the maturity height indicated by the input.
func (cs *ConsensusSet) dbGetDSCO(height types.BlockHeight, id types.SiacoinOutputID) (dsco types.SiacoinOutput, err error) {
	dbErr := cs.db.View(func(tx *bolt.Tx) error {
		dscoBucketID := append(prefixDSCO, encoding.Marshal(height)...)
		dscoBucket := tx.Bucket(dscoBucketID)
		if dscoBucket == nil {
			err = errNilItem
			return nil
		}
		dscoBytes := dscoBucket.Get(id[:])
		if dscoBytes == nil {
			err = errNilItem
			return nil
		}
		err = encoding.Unmarshal(dscoBytes, &dsco)
		if err != nil {
			panic(err)
		}
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return dsco, err
}

/// BREAK ///

// addDelayedSiacoinOutputsHeight inserts a siacoin output to the bucket at a particular height
func (db *setDB) addDelayedSiacoinOutputsHeight(h types.BlockHeight, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	bucketID := append(prefixDSCO, encoding.Marshal(h)...)
	err := db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, bucketID, id, sco)
	})
	if err != nil {
		panic(err)
	}
}

// rmDelayedSiacoinOutputsHeight removes a siacoin output with a given ID at the given height
func (db *setDB) rmDelayedSiacoinOutputsHeight(h types.BlockHeight, id types.SiacoinOutputID) error {
	bucketID := append(prefixDSCO, encoding.Marshal(h)...)
	return db.rmItem(bucketID, id)
}

// lenSiacoinOutputs returns the size of the siacoin outputs bucket
func (db *setDB) lenSiacoinOutputs() uint64 {
	return db.lenBucket(SiacoinOutputs)
}

// lenFileContracts returns the number of file contracts in the consensus set
func (db *setDB) lenFileContracts() uint64 {
	return db.lenBucket(FileContracts)
}

// lenFCExpirationsHeight returns the number of file contracts which expire at a given height
func (db *setDB) lenFCExpirationsHeight(h types.BlockHeight) uint64 {
	bucketID := append(prefixFCEX, encoding.Marshal(h)...)
	return db.lenBucket(bucketID)
}

// lenSiafundOutputs returns the size of the SiafundOutputs bucket
func (db *setDB) lenSiafundOutputs() uint64 {
	return db.lenBucket(SiafundOutputs)
}

// inFileContracts is a wrapper around inBucket which returns true if
// a file contract is in the consensus set
func (db *setDB) inFileContracts(id types.FileContractID) bool {
	return db.inBucket(FileContracts, id)
}

// rmFileContracts removes a file contract from the consensus set
func (db *setDB) rmFileContracts(id types.FileContractID) error {
	return db.rmItem(FileContracts, id)
}

// addSiacoinOutputs adds a given siacoin output to the SiacoinOutputs bucket
func (db *setDB) addSiacoinOutputs(id types.SiacoinOutputID, sco types.SiacoinOutput) error {
	return db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, SiacoinOutputs, id, sco)
	})
}

// addBlockMap adds a processedBlock to the block map
// This will eventually take a processed block as an argument
func (db *setDB) addBlockMap(pb *processedBlock) error {
	return db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, BlockMap, pb.Block.ID(), *pb)
	})
}

// addFileContracts is a wrapper around addItem for adding a file
// contract to the consensusset
func (db *setDB) addFileContracts(id types.FileContractID, fc types.FileContract) error {
	return db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, FileContracts, id, fc)
	})
}

// insertItem inserts an item to a bucket. In debug mode, a panic is thrown if
// the bucket does not exist or if the item is already in the bucket.
func insertItem(tx *bolt.Tx, bucket []byte, key, value interface{}) error {
	b := tx.Bucket(bucket)
	if build.DEBUG && b == nil {
		panic(errNilBucket)
	}
	k := encoding.Marshal(key)
	v := encoding.Marshal(value)
	if build.DEBUG && b.Get(k) != nil {
		panic(errRepeatInsert)
	}
	return b.Put(k, v)
}

// pathHeight returns the size of the current path
func (db *setDB) pathHeight() types.BlockHeight {
	return types.BlockHeight(db.lenBucket(BlockPath))
}

// forEachDelayedSiacoinOutputsHeight applies a function to every siacoin output at a given height
func (db *setDB) forEachDelayedSiacoinOutputsHeight(h types.BlockHeight, fn func(k types.SiacoinOutputID, v types.SiacoinOutput)) {
	bucketID := append(prefixDSCO, encoding.Marshal(h)...)
	db.forEachItem(bucketID, func(kb, vb []byte) error {
		var key types.SiacoinOutputID
		var value types.SiacoinOutput
		err := encoding.Unmarshal(kb, &key)
		if err != nil {
			return err
		}
		err = encoding.Unmarshal(vb, &value)
		if err != nil {
			return err
		}
		fn(key, value)
		return nil
	})
}

// lenDelayedSiacoinOutputsHeight returns the number of outputs stored at one height
func (db *setDB) lenDelayedSiacoinOutputsHeight(h types.BlockHeight) uint64 {
	bucketID := append(prefixDSCO, encoding.Marshal(h)...)
	return db.lenBucket(bucketID)
}

// inDelayedSiacoinOutputsHeight returns a boolean showing if a siacoin output exists at a given height
func (db *setDB) inDelayedSiacoinOutputsHeight(h types.BlockHeight, id types.SiacoinOutputID) bool {
	bucketID := append(prefixDSCO, encoding.Marshal(h)...)
	return db.inBucket(bucketID, id)
}

// getDelayedSiacoinOutputs returns a particular siacoin output given a height and an ID
func (db *setDB) getDelayedSiacoinOutputs(h types.BlockHeight, id types.SiacoinOutputID) types.SiacoinOutput {
	bucketID := append(prefixDSCO, encoding.Marshal(h)...)
	scoBytes, err := db.getItem(bucketID, id)
	if build.DEBUG && err != nil {
		panic(err)
	}
	var sco types.SiacoinOutput
	err = encoding.Unmarshal(scoBytes, &sco)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return sco
}

// forEachSiacoinOutputs applies a function to every siacoin output and ID
func (db *setDB) forEachSiacoinOutputs(fn func(k types.SiacoinOutputID, v types.SiacoinOutput)) {
	db.forEachItem(SiacoinOutputs, func(kb, vb []byte) error {
		var key types.SiacoinOutputID
		var value types.SiacoinOutput
		err := encoding.Unmarshal(kb, &key)
		if err != nil {
			return err
		}
		err = encoding.Unmarshal(vb, &value)
		if err != nil {
			return err
		}
		fn(key, value)
		return nil
	})
}

// inSiacoinOutputs returns a bool showing if a soacoin output ID is
// in the siacoin outputs bucket
func (db *setDB) inSiacoinOutputs(id types.SiacoinOutputID) bool {
	return db.inBucket(SiacoinOutputs, id)
}

// getSiacoinOutputs retrieves a saicoin output by ID
func (db *setDB) getSiacoinOutputs(id types.SiacoinOutputID) types.SiacoinOutput {
	scoBytes, err := db.getItem(SiacoinOutputs, id)
	if err != nil {
		panic(err)
	}
	var sco types.SiacoinOutput
	err = encoding.Unmarshal(scoBytes, &sco)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return sco
}

// forEachFileContracts applies a function to each (file contract id, filecontract)
// pair in the consensus set
func (db *setDB) forEachFileContracts(fn func(k types.FileContractID, v types.FileContract)) {
	db.forEachItem(FileContracts, func(kb, vb []byte) error {
		var key types.FileContractID
		var value types.FileContract
		err := encoding.Unmarshal(kb, &key)
		if err != nil {
			return err
		}
		err = encoding.Unmarshal(vb, &value)
		if err != nil {
			return err
		}
		fn(key, value)
		return nil
	})
}

// rmSiafundOutputs removes a siafund output from the database
func (db *setDB) rmSiafundOutputs(id types.SiafundOutputID) error {
	return db.rmItem(SiafundOutputs, id)
}

// inSiafundOutputs is a wrapper around inBucket which returns a true
// if an output with the given id is in the database
func (db *setDB) inSiafundOutputs(id types.SiafundOutputID) bool {
	return db.inBucket(SiafundOutputs, id)
}

// getSiafundOutputs is a wrapper around getItem which decodes the
// result into a siafundOutput
func (db *setDB) getSiafundOutputs(id types.SiafundOutputID) types.SiafundOutput {
	sfoBytes, err := db.getItem(SiafundOutputs, id)
	if build.DEBUG && err != nil {
		panic(err)
	}
	var sfo types.SiafundOutput
	err = encoding.Unmarshal(sfoBytes, &sfo)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return sfo
}

// rmItem removes an item from a bucket
func (db *setDB) rmItem(bucket []byte, key interface{}) error {
	k := encoding.Marshal(key)
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if build.DEBUG {
			// Sanity check to make sure the bucket exists.
			if b == nil {
				panic(errNilBucket)
			}
			// Sanity check to make sure you are deleting an item that exists
			item := b.Get(k)
			if item == nil {
				panic(errNilItem)
			}
		}
		return b.Delete(k)
	})
}

// inBucket checks if an item with the given key is in the bucket
func (db *setDB) inBucket(bucket []byte, key interface{}) bool {
	exists, err := db.Exists(bucket, encoding.Marshal(key))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return exists
}

// lenBucket is a simple wrapper for bucketSize that panics on error
func (db *setDB) lenBucket(bucket []byte) uint64 {
	s, err := db.BucketSize(bucket)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return s
}

// forEachItem runs a given function on every element in a given
// bucket name, and will panic on any error
func (db *setDB) forEachItem(bucket []byte, fn func(k, v []byte) error) {
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if build.DEBUG && b == nil {
			panic(errNilBucket)
		}
		return b.ForEach(fn)
	})
	if err != nil {
		panic(err)
	}
}

// inBlockMap checks for the existance of a block with a given ID in
// the consensus set
func (db *setDB) inBlockMap(id types.BlockID) bool {
	return db.inBucket(BlockMap, id)
}

func (db *setDB) forEachSiafundOutputs(fn func(k types.SiafundOutputID, v types.SiafundOutput)) {
	db.forEachItem(SiafundOutputs, func(kb, vb []byte) error {
		var key types.SiafundOutputID
		var value types.SiafundOutput
		err := encoding.Unmarshal(kb, &key)
		if err != nil {
			return err
		}
		err = encoding.Unmarshal(vb, &value)
		if err != nil {
			return err
		}
		fn(key, value)
		return nil
	})
}

// getFileContracts is a wrapper around getItem for retrieving a file contract
func (db *setDB) getFileContracts(id types.FileContractID) types.FileContract {
	fcBytes, err := db.getItem(FileContracts, id)
	if build.DEBUG && err != nil {
		panic(err)
	}
	var fc types.FileContract
	err = encoding.Unmarshal(fcBytes, &fc)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return fc
}

// getItem is a generic function to insert an item into the set database
func (db *setDB) getItem(bucket []byte, key interface{}) (item []byte, err error) {
	k := encoding.Marshal(key)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		// Sanity check to make sure the bucket exists.
		if build.DEBUG && b == nil {
			panic(errNilBucket)
		}
		item = b.Get(k)
		// Sanity check to make sure the item requested exists
		if item == nil {
			return errNilItem
		}
		return nil
	})
	return item, err
}
