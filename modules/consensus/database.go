package consensus

// database.go contains initialization functions for the database, and helper
// functions for accessing the database. All database access functions take a
// bolt.Tx as input because consensus manipulations should always be made as a
// single atomic transaction. Any function for changing the database will not
// return errors, but instead will panic as a sanity check. No item should ever
// be inserted into the database that is already in the database, and no item
// should ever be removed from the database that is not currently in the
// database. Attempting such with the debug flags enabled indicate developer
// error and will cause a panic.

import (
	"errors"
	"fmt"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errRepeatInsert   = errors.New("attempting to add an already existing item to the consensus set")
	errNilBucket      = errors.New("using a bucket that does not exist")
	errNilItem        = errors.New("requested item does not exist")
	errDBInconsistent = errors.New("database guard indicates inconsistency within database")
	errNonEmptyBucket = errors.New("cannot remove a map with objects still in it")

	prefix_dsco = []byte("dsco_")
	prefix_fcex = []byte("fcex_")

	meta = persist.Metadata{
		Version: "0.4.3",
		Header:  "Consensus Set Database",
	}

	// Generally we would just look at BlockPath.Stats(), but there is an error
	// in boltdb that prevents the bucket stats from updating until a tx is
	// committed. Wasn't a problem until we started doing the entire block as
	// one tx.
	//
	// DEPRECATED.
	BlockHeight = []byte("BlockHeight")

	// BlockPath is a database bucket containing a mapping from the height of a
	// block to the id of the block at that height. BlockPath only includes
	// blocks in the current path.
	BlockPath               = []byte("BlockPath")
	BlockMap                = []byte("BlockMap")
	SiacoinOutputs          = []byte("SiacoinOutputs")
	FileContracts           = []byte("FileContracts")
	FileContractExpirations = []byte("FileContractExpirations") // TODO: Unneeded data structure
	SiafundOutputs          = []byte("SiafundOutputs")
	SiafundPool             = []byte("SiafundPool")
)

// setDB is a wrapper around the persist bolt db which backs the
// consensus set
type setDB struct {
	*persist.BoltDatabase
	// The open flag is used to prevent reading from the database
	// after closing sia when the loading loop is still running
	open bool // DEPRECATED
}

// openDB loads the set database and populates it with the necessary buckets
func openDB(filename string) (*setDB, error) {
	db, err := persist.OpenDatabase(meta, filename)
	if err != nil {
		return nil, err
	}

	// Enumerate the database buckets.
	buckets := [][]byte{
		BlockPath,
		BlockMap,
		SiacoinOutputs,
		FileContracts,
		FileContractExpirations,
		SiafundOutputs,
		SiafundPool,
		BlockHeight,
	}

	// Initialize the database.
	err = db.Update(func(tx *bolt.Tx) error {
		// Create the database buckets.
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return &setDB{db, true}, err
}

// blockHeight returns the height of the blockchain.
func blockHeight(tx *bolt.Tx) types.BlockHeight {
	var height int
	bh := tx.Bucket(BlockHeight)
	err := encoding.Unmarshal(bh.Get(BlockHeight), &height)
	if build.DEBUG && err != nil {
		panic(err)
	}
	if height < 0 {
		panic(height)
	}
	return types.BlockHeight(height)
}

// currentBlockID returns the id of the most recent block in the consensus set.
func currentBlockID(tx *bolt.Tx) types.BlockID {
	return getPath(tx, blockHeight(tx))
}

// currentProcessedBlock returns the most recent block in the consensus set.
func currentProcessedBlock(tx *bolt.Tx) *processedBlock {
	pb, err := getBlockMap(tx, currentBlockID(tx))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return pb
}

// getBlockMap returns a processed block with the input id.
func getBlockMap(tx *bolt.Tx, id types.BlockID) (*processedBlock, error) {
	// Look up the encoded block.
	pbBytes := tx.Bucket(BlockMap).Get(id[:])
	if pbBytes == nil {
		return nil, errNilItem
	}

	// Decode the block - should never fail.
	var pb processedBlock
	err := encoding.Unmarshal(pbBytes, &pb)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return &pb, nil
}

// getPath returns the block id at 'height' in the block path.
func getPath(tx *bolt.Tx, height types.BlockHeight) (id types.BlockID) {
	idBytes := tx.Bucket(BlockPath).Get(encoding.Marshal(height))
	err := encoding.Unmarshal(idBytes, &id)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return id
}

// pushPath adds a block to the BlockPath at current height + 1.
func pushPath(tx *bolt.Tx, bid types.BlockID) {
	// Fetch and update the block height.
	bh := tx.Bucket(BlockHeight)
	heightBytes := bh.Get(BlockHeight)
	var oldHeight types.BlockHeight
	err := encoding.Unmarshal(heightBytes, &oldHeight)
	if build.DEBUG && err != nil {
		panic(err)
	}
	newHeightBytes := encoding.Marshal(oldHeight + 1)
	err = bh.Put(BlockHeight, newHeightBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Add the block to the block path.
	bp := tx.Bucket(BlockPath)
	err = bp.Put(newHeightBytes, bid[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// popPath removes a block from the "end" of the chain, i.e. the block
// with the largest height.
func popPath(tx *bolt.Tx) {
	// Fetch and update the block height.
	bh := tx.Bucket(BlockHeight)
	oldHeightBytes := bh.Get(BlockHeight)
	var oldHeight types.BlockHeight
	err := encoding.Unmarshal(oldHeightBytes, &oldHeight)
	if build.DEBUG && err != nil {
		panic(err)
	}
	newHeightBytes := encoding.Marshal(oldHeight - 1)
	err = bh.Put(BlockHeight, newHeightBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Remove the block from the path - make sure to remove the block at
	// oldHeight.
	bp := tx.Bucket(BlockPath)
	err = bp.Delete(oldHeightBytes)
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// isSiacoinOutput returns true if there is a siacoin output of that id in the
// database.
func isSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) bool {
	bucket := tx.Bucket(SiacoinOutputs)
	sco := bucket.Get(id[:])
	return sco != nil
}

// getSiacoinOutput fetches a siacoin output from the database. An error is
// returned if the siacoin output does not exist.
func getSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) (types.SiacoinOutput, error) {
	scoBytes := tx.Bucket(SiacoinOutputs).Get(id[:])
	if scoBytes == nil {
		return types.SiacoinOutput{}, errNilItem
	}
	var sco types.SiacoinOutput
	err := encoding.Unmarshal(scoBytes, &sco)
	if err != nil {
		return types.SiacoinOutput{}, err
	}
	return sco, nil
}

// addSiacoinOutput adds a siacoin output to the database. An error is returned
// if the siacoin output is already in the database.
func addSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	siacoinOutputs := tx.Bucket(SiacoinOutputs)
	// Sanity check - should not be adding an item that exists.
	if build.DEBUG && siacoinOutputs.Get(id[:]) != nil {
		panic("repeat siacoin output")
	}
	err := siacoinOutputs.Put(id[:], encoding.Marshal(sco))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeSiacoinOutput removes a siacoin output from the database. An error is
// returned if the siacoin output is not in the database prior to removal.
func removeSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) {
	scoBucket := tx.Bucket(SiacoinOutputs)
	// Sanity check - should not be removing an item that is not in the db.
	if build.DEBUG && scoBucket.Get(id[:]) == nil {
		panic("nil siacoin output")
	}
	err := scoBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getFileContract fetches a file contract from the database, returning an
// error if it is not there.
func getFileContract(tx *bolt.Tx, id types.FileContractID) (fc types.FileContract, err error) {
	fcBytes := tx.Bucket(FileContracts).Get(id[:])
	if fcBytes == nil {
		return types.FileContract{}, errNilItem
	}
	err = encoding.Unmarshal(fcBytes, &fc)
	if err != nil {
		return types.FileContract{}, err
	}
	return fc, nil
}

// addFileContract adds a file contract to the database. An error is returned
// if the file contract is already in the database.
func addFileContract(tx *bolt.Tx, id types.FileContractID, fc types.FileContract) {
	// Add the file contract to the database.
	fcBucket := tx.Bucket(FileContracts)
	// Sanity check - should not be adding a file contract already in the db.
	if build.DEBUG && fcBucket.Get(id[:]) != nil {
		panic("repeat file contract")
	}
	err := fcBucket.Put(id[:], encoding.Marshal(fc))
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Add an entry for when the file contract expires.
	expirationBucketID := append(prefix_fcex, encoding.Marshal(fc.WindowEnd)...)
	expirationBucket, err := tx.CreateBucketIfNotExists(expirationBucketID)
	if build.DEBUG && err != nil {
		panic(err)
	}
	err = expirationBucket.Put(id[:], []byte{})
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeFileContract removes a file contract from the database.
func removeFileContract(tx *bolt.Tx, id types.FileContractID) {
	// Delete the file contract entry.
	fcBucket := tx.Bucket(FileContracts)
	fcBytes := fcBucket.Get(id[:])
	// Sanity check - should not be removing a file contract not in the db.
	if build.DEBUG && fcBytes == nil {
		panic("nil file contract")
	}
	err := fcBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Delete the entry for the file contract's expiration. The portion of
	// 'fcBytes' used to determine the expiration bucket id is the
	// byte-representation of the file contract window end, which always
	// appears at bytes 48-56.
	expirationBucketID := append(prefix_fcex, fcBytes[48:56]...)
	expirationBucket := tx.Bucket(expirationBucketID)
	expirationBytes := expirationBucket.Get(id[:])
	if expirationBytes == nil {
		panic(errNilItem)
	}
	err = expirationBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// addSiafundOutput adds a siafund output to the database. An error is returned
// if the siafund output is already in the database.
func addSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID, sco types.SiafundOutput) {
	siafundOutputs := tx.Bucket(SiafundOutputs)
	// Sanity check - should not be adding an item already in the db.
	if build.DEBUG && siafundOutputs.Get(id[:]) != nil {
		panic("repeat siafund output")
	}
	err := siafundOutputs.Put(id[:], encoding.Marshal(sco))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeSiafundOutput removes a siafund output from the database. An error is
// returned if the siafund output is not in the database prior to removal.
func removeSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) {
	sfoBucket := tx.Bucket(SiafundOutputs)
	if build.DEBUG && sfoBucket.Get(id[:]) == nil {
		panic("nil siafund output")
	}
	err := sfoBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// getSiafundPool returns the current value of the siafund pool. No error is
// returned as the siafund pool should always be available.
func getSiafundPool(tx *bolt.Tx) (pool types.Currency) {
	bucket := tx.Bucket(SiafundPool)
	poolBytes := bucket.Get(SiafundPool)
	// An error should only be returned if the object stored in the siafund
	// pool bucket is either unavailable or otherwise malformed. As this is a
	// developer error, a panic is appropriate.
	err := encoding.Unmarshal(poolBytes, &pool)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return pool
}

// setSiafundPool updates the saved siafund pool on disk
func setSiafundPool(tx *bolt.Tx, c types.Currency) {
	err := tx.Bucket(SiafundPool).Put(SiafundPool, encoding.Marshal(c))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// addDSCO adds a delayed siacoin output to the consnesus set.
func addDSCO(tx *bolt.Tx, bh types.BlockHeight, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	// Sanity check - output should not already be in the full set of outputs.
	if build.DEBUG && tx.Bucket(SiacoinOutputs).Get(id[:]) != nil {
		panic("dsco already in output set")
	}
	dscoBucketID := append(prefix_dsco, encoding.EncUint64(uint64(bh))...)
	dscoBucket := tx.Bucket(dscoBucketID)
	// Sanity check - should not be adding an item already in the db.
	if build.DEBUG && dscoBucket.Get(id[:]) != nil {
		panic(errRepeatInsert)
	}
	err := dscoBucket.Put(id[:], encoding.Marshal(sco))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// removeDSCO removes a delayed siacoin output from the consensus set.
func removeDSCO(tx *bolt.Tx, bh types.BlockHeight, id types.SiacoinOutputID) {
	bucketID := append(prefix_dsco, encoding.Marshal(bh)...)
	// Sanity check - should not remove an item not in the db.
	dscoBucket := tx.Bucket(bucketID)
	if build.DEBUG && dscoBucket.Get(id[:]) == nil {
		fmt.Println("NIL DSCO", id)
		// panic("nil dsco")
	}
	err := dscoBucket.Delete(id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// createDSCOBucket creates a bucket for the delayed siacoin outputs at the
// input height.
func createDSCOBucket(tx *bolt.Tx, bh types.BlockHeight) {
	bucketID := append(prefix_dsco, encoding.Marshal(bh)...)
	_, err := tx.CreateBucket(bucketID)
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// deleteDSCOBucket deletes the bucket that held a set of delayed siacoin
// outputs.
func deleteDSCOBucket(tx *bolt.Tx, h types.BlockHeight) {
	// Delete the bucket.
	bucketID := append(prefix_dsco, encoding.Marshal(h)...)
	bucket := tx.Bucket(bucketID)
	if build.DEBUG && bucket == nil {
		panic(errNilBucket)
	}

	// TODO: Check that the bucket is empty. Using Stats() does not work at the
	// moment, as there is an error in the boltdb code.

	err := tx.DeleteBucket(bucketID)
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// BREAK //
// BREAK //
// BREAK //
// BREAK //

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

// removeItem deletes an item from a bucket. In debug mode, a panic is thrown
// if the bucket does not exist or if the item is not in the bucket.
func removeItem(tx *bolt.Tx, bucket []byte, key interface{}) error {
	k := encoding.Marshal(key)
	b := tx.Bucket(bucket)
	if build.DEBUG && b == nil {
		panic(errNilBucket)
	}
	if build.DEBUG && b.Get(k) == nil {
		panic(errNilItem)
	}
	return b.Delete(k)
}

// getItem returns an item from a bucket. In debug mode, a panic is thrown if
// the bucket does not exist or if the item does not exist.
func getItem(tx *bolt.Tx, bucket []byte, key interface{}) ([]byte, error) {
	b := tx.Bucket(bucket)
	if build.DEBUG && b == nil {
		panic(errNilBucket)
	}
	k := encoding.Marshal(key)
	item := b.Get(k)
	if item == nil {
		return nil, errNilItem
	}
	return item, nil
}

// forEach iterates through a bucket, calling the supplied closure on each
// element.
func forEach(tx *bolt.Tx, bucket []byte, fn func(k, v []byte) error) error {
	b := tx.Bucket(bucket)
	if build.DEBUG && b == nil {
		panic(errNilBucket)
	}
	return b.ForEach(fn)
}

// getItem is a generic function to insert an item into the set database
//
// DEPRECATED
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

// getPath retreives the block id of a block at a given hegiht from the path
//
// DEPRECATED
func (db *setDB) getPath(h types.BlockHeight) (id types.BlockID) {
	idBytes, err := db.getItem(BlockPath, h)
	if err != nil {
		panic(err)
	}
	err = encoding.Unmarshal(idBytes, &id)
	if err != nil {
		panic(err)
	}
	return
}

// pathHeight returns the size of the current path
func (db *setDB) pathHeight() types.BlockHeight {
	return types.BlockHeight(db.lenBucket(BlockPath))
}

// addBlockMap adds a processedBlock to the block map
// This will eventually take a processed block as an argument
func (db *setDB) addBlockMap(pb *processedBlock) error {
	return db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, BlockMap, pb.Block.ID(), *pb)
	})
}

// getBlockMap queries the set database to return a processedBlock
// with the given ID
//
// DEPRECATED
func (db *setDB) getBlockMap(id types.BlockID) *processedBlock {
	bnBytes, err := db.getItem(BlockMap, id)
	if build.DEBUG && err != nil {
		panic(err)
	}
	var pb processedBlock
	err = encoding.Unmarshal(bnBytes, &pb)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return &pb
}

// inBlockMap checks for the existance of a block with a given ID in
// the consensus set
func (db *setDB) inBlockMap(id types.BlockID) bool {
	return db.inBucket(BlockMap, id)
}

// rmBlockMap removes a processedBlock from the blockMap bucket
func (db *setDB) rmBlockMap(id types.BlockID) error {
	return db.rmItem(BlockMap, id)
}

// updateBlockMap is a wrapper function for modification of
func (db *setDB) updateBlockMap(pb *processedBlock) {
	err := db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(BlockMap)
		id := pb.Block.ID()
		return bucket.Put(id[:], encoding.Marshal(*pb))
	})
	if err != nil {
		panic(err)
	}
}

func getSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) (types.SiafundOutput, error) {
	sfoBytes, err := getItem(tx, SiafundOutputs, id)
	if err != nil {
		return types.SiafundOutput{}, err
	}
	var sfo types.SiafundOutput
	err = encoding.Unmarshal(sfoBytes, &sfo)
	if err != nil {
		return types.SiafundOutput{}, err
	}
	return sfo, nil
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

// inSiafundOutputs is a wrapper around inBucket which returns a true
// if an output with the given id is in the database
func (db *setDB) inSiafundOutputs(id types.SiafundOutputID) bool {
	return db.inBucket(SiafundOutputs, id)
}

// rmSiafundOutputs removes a siafund output from the database
func (db *setDB) rmSiafundOutputs(id types.SiafundOutputID) error {
	return db.rmItem(SiafundOutputs, id)
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

// addFileContracts is a wrapper around addItem for adding a file
// contract to the consensusset
func (db *setDB) addFileContracts(id types.FileContractID, fc types.FileContract) error {
	return db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, FileContracts, id, fc)
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

// addSiacoinOutputs adds a given siacoin output to the SiacoinOutputs bucket
func (db *setDB) addSiacoinOutputs(id types.SiacoinOutputID, sco types.SiacoinOutput) error {
	return db.Update(func(tx *bolt.Tx) error {
		return insertItem(tx, SiacoinOutputs, id, sco)
	})
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

// inSiacoinOutputs returns a bool showing if a soacoin output ID is
// in the siacoin outputs bucket
func (db *setDB) inSiacoinOutputs(id types.SiacoinOutputID) bool {
	return db.inBucket(SiacoinOutputs, id)
}

// rmSiacoinOutputs removes a siacoin output form the siacoin outputs map
func (db *setDB) rmSiacoinOutputs(id types.SiacoinOutputID) error {
	return db.rmItem(SiacoinOutputs, id)
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

// getDelayedSiacoinOutputs returns a particular siacoin output given a height and an ID
func (db *setDB) getDelayedSiacoinOutputs(h types.BlockHeight, id types.SiacoinOutputID) types.SiacoinOutput {
	bucketID := append(prefix_dsco, encoding.Marshal(h)...)
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

// inDelayedSiacoinOutputsHeight returns a boolean showing if a siacoin output exists at a given height
func (db *setDB) inDelayedSiacoinOutputsHeight(h types.BlockHeight, id types.SiacoinOutputID) bool {
	bucketID := append(prefix_dsco, encoding.Marshal(h)...)
	return db.inBucket(bucketID, id)
}

// lenDelayedSiacoinOutputsHeight returns the number of outputs stored at one height
func (db *setDB) lenDelayedSiacoinOutputsHeight(h types.BlockHeight) uint64 {
	bucketID := append(prefix_dsco, encoding.Marshal(h)...)
	return db.lenBucket(bucketID)
}

func forEachDSCO(tx *bolt.Tx, bh types.BlockHeight, fn func(id types.SiacoinOutputID, sco types.SiacoinOutput) error) error {
	bucketID := append(prefix_dsco, encoding.Marshal(bh)...)
	return forEach(tx, bucketID, func(kb, vb []byte) error {
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
		return fn(key, value)
	})
}

// forEachDelayedSiacoinOutputsHeight applies a function to every siacoin output at a given height
func (db *setDB) forEachDelayedSiacoinOutputsHeight(h types.BlockHeight, fn func(k types.SiacoinOutputID, v types.SiacoinOutput)) {
	bucketID := append(prefix_dsco, encoding.Marshal(h)...)
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
