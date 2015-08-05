package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
	"github.com/boltdb/bolt"
)

var meta = persist.Metadata{
	Version: "0.4.0",
	Header:  "Consensus Set Database",
}

var (
	errBadSetInsert = errors.New("attempting to add an already existing item to the consensus set")
	errNilBucket    = errors.New("using a bucket that does not exist")
	errNilItem      = errors.New("requested item does not exist")
	errNotGuarded   = errors.New("database modification not protected by guard")
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

	var buckets []string = []string{
		"Path",
		"BlockMap",
		"Metadata",
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucketName := range buckets {
			_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return err
			}
		}
		// Initilize the consistency guards
		b := tx.Bucket([]byte("Metadata"))
		err := b.Put([]byte("GuardA"), encoding.Marshal(0))
		if err != nil {
			return err
		}
		return b.Put([]byte("GuardB"), encoding.Marshal(0))
	})
	return &setDB{db, true}, err
}

// startConsistencyGuard increments the first guard. If this is not
// equal to the second, a transaction is taking place in the database
func (db *setDB) startConsistencyGuard() {
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Metadata"))
		var i int
		err := encoding.Unmarshal(b.Get([]byte("GuardA")), &i)
		if err != nil {
			return err
		}
		return b.Put([]byte("GuardA"), encoding.Marshal(i+1))
	})
	if err != nil {
		panic(err)
	}
}

// startConsistencyGuard increments the first guard. If this is not
// equal to the second, a transaction is taking place in the database
func (db *setDB) stopConsistencyGuard() {
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Metadata"))
		var i int
		err := encoding.Unmarshal(b.Get([]byte("GuardB")), &i)
		if err != nil {
			return err
		}
		return b.Put([]byte("GuardB"), encoding.Marshal(i+1))
	})
	if err != nil {
		panic(err)
	}
}

// checkConsistencyGuard checks the two guards and returns true if
// they differ. This signifies that thaer there is a transaction
// taking place.
func (db *setDB) checkConsistencyGuard() bool {
	var guarded bool
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Metadata"))
		var x, y int
		err := encoding.Unmarshal(b.Get([]byte("GuardA")), &x)
		if err != nil {
			return err
		}
		err = encoding.Unmarshal(b.Get([]byte("GuardB")), &y)
		if err != nil {
			return err
		}
		guarded = x != y
		return nil
	})
	if err != nil {
		panic(err)
	}
	return guarded
}

// addItem should only be called from this file, and adds a new item
// to the database
//
// addItem and getItem are part of consensus due to stricter error
// conditions than a generic bolt implementation
func (db *setDB) addItem(bucket string, key, value interface{}) error {
	// Check that this transaction is guarded by consensusGuard.
	// However, allow direct database modifications when testing
	if build.DEBUG && !db.checkConsistencyGuard() && build.Release != "testing" {
		panic(errNotGuarded)
	}
	v := encoding.Marshal(value)
	k := encoding.Marshal(key)
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		// Sanity check: make sure the buckets exists and that
		// you are not inserting something that already exists
		if build.DEBUG {
			if b == nil {
				panic(errNilBucket)
			}
			i := b.Get(k)
			if i != nil {
				panic(errBadSetInsert)
			}
		}
		return b.Put(k, v)
	})
}

// getItem is a generic function to insert an item into the set database
func (db *setDB) getItem(bucket string, key interface{}) (item []byte, err error) {
	k := encoding.Marshal(key)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		// Sanity check to make sure the bucket exists.
		if build.DEBUG {
			if b == nil {
				panic(errNilBucket)
			}
		}
		item = b.Get(k)
		// Sanity check to make sure the item requested exists
		if build.DEBUG {
			if item == nil {
				panic(errNilItem)
			}
		}
		return nil
	})
	return item, err
}

// rmItem removes an item from a bucket
func (db *setDB) rmItem(bucket string, key interface{}) error {
	if build.DEBUG && !db.checkConsistencyGuard() && build.Release != "testing" {
		panic(errNotGuarded)
	}
	k := encoding.Marshal(key)
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
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
func (db *setDB) inBucket(bucket string, key interface{}) bool {
	exists, err := db.Exists(bucket, encoding.Marshal(key))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return exists
}

// pushPath inserts a block into the database at the "end" of the chain, i.e.
// the current height + 1.
func (db *setDB) pushPath(bid types.BlockID) error {
	if build.DEBUG && !db.checkConsistencyGuard() && build.Release != "testing" {
		panic(errNotGuarded)
	}
	value := encoding.Marshal(bid)
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Path"))
		key := encoding.EncUint64(uint64(b.Stats().KeyN))
		return b.Put(key, value)
	})
}

// popPath removes a block from the "end" of the chain, i.e. the block
// with the largest height.
func (db *setDB) popPath() error {
	if build.DEBUG && !db.checkConsistencyGuard() && build.Release != "testing" {
		panic(errNotGuarded)
	}
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Path"))
		key := encoding.EncUint64(uint64(b.Stats().KeyN - 1))
		return b.Delete(key)
	})
}

// getPath retreives the block id of a block at a given hegiht from the path
func (db *setDB) getPath(h types.BlockHeight) (id types.BlockID) {
	idBytes, err := db.getItem("Path", h)
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
	h, err := db.BucketSize("Path")
	if err != nil {
		panic(err)
	}
	return types.BlockHeight(h)
}

// addBlockMap adds a processedBlock to the block map
// This will eventually take a processed block as an argument
func (db *setDB) addBlockMap(pb *processedBlock) error {
	return db.addItem("BlockMap", pb.Block.ID(), *pb)
}

// getBlockMap queries the set database to return a processedBlock
// with the given ID
func (db *setDB) getBlockMap(id types.BlockID) *processedBlock {
	bnBytes, err := db.getItem("BlockMap", id)
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
	return db.inBucket("BlockMap", id)
}

// rmBlockMap removes a processedBlock from the blockMap bucket
func (db *setDB) rmBlockMap(id types.BlockID) error {
	return db.rmItem("BlockMap", id)
}

// updateBlockMap is a wrapper function for modification of
func (db *setDB) updateBlockMap(pb *processedBlock) {
	// These errors will only be caused by an error by bolt
	// e.g. database being closed.
	err := db.rmBlockMap(pb.Block.ID())
	if build.DEBUG && err != nil {
		panic(err)
	}
	err = db.addBlockMap(pb)
	if build.DEBUG && err != nil {
		panic(err)
	}
}
