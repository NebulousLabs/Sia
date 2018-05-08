package database

import (
	"github.com/NebulousLabs/Sia/persist"
	"github.com/coreos/bbolt"
)

var (
	prefixDSCO = []byte("dsco_")
	prefixFCEX = []byte("fcex_")
)

var (
	// blockHeight is a bucket that stores the current block height.
	blockHeight = []byte("BlockHeight")

	// blockMap is a database bucket containing all of the processed blocks,
	// keyed by their id. This includes blocks that are not currently in the
	// consensus set, and blocks that may not have been fully validated yet.
	blockMap = []byte("BlockMap")

	// blockPath is a database bucket containing a mapping from the height of a
	// block to the id of the block at that height. BlockPath only includes
	// blocks in the current path.
	blockPath = []byte("BlockPath")

	// bucketOak is the database bucket that contains all of the fields related
	// to the oak difficulty adjustment algorithm. The cumulative difficulty and
	// time values are stored for each block id, and then the key "OakInit"
	// contains the value "true" if the oak fields have been properly
	// initialized.
	bucketOak = []byte("Oak")

	// consistency is a database bucket with a flag indicating whether
	// inconsistencies within the database have been detected.
	consistency = []byte("Consistency")

	// fileContracts is a database bucket that contains all of the open file
	// contracts.
	fileContracts = []byte("FileContracts")

	// siacoinOutputs is a database bucket that contains all of the unspent
	// siacoin outputs.
	siacoinOutputs = []byte("SiacoinOutputs")

	// siafundOutputs is a database bucket that contains all of the unspent
	// siafund outputs.
	siafundOutputs = []byte("SiafundOutputs")

	// siafundPool is a database bucket storing the current value of the
	// siafund pool.
	siafundPool = []byte("SiafundPool")
)

var (
	// fieldOakInit is a field in BucketOak that gets set to "true" after the
	// oak initialiation process has completed.
	fieldOakInit = []byte("OakInit")
)

var (
	// valueOakInit is the value that the oak init field is set to if the oak
	// difficulty adjustment fields have been correctly intialized.
	valueOakInit = []byte("true")
)

// A DB is a database suitable for storing consensus data.
type DB interface {
	Update(func(Tx) error) error
	View(func(Tx) error) error
	Close() error
}

// Open opens and initializes a consensus database.
func Open(filename string) (DB, error) {
	pdb, err := persist.OpenDatabase(persist.Metadata{
		Header:  "Consensus Set Database",
		Version: "0.5.0",
	}, filename)
	if err != nil {
		return nil, err
	}

	// create the database buckets.
	buckets := [][]byte{
		blockHeight,
		blockMap,
		blockPath,
		consistency,
		siacoinOutputs,
		fileContracts,
		siafundOutputs,
		siafundPool,
	}
	err = pdb.Update(func(tx *bolt.Tx) error {
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return boltWrapper{pdb.DB}, nil
}

// boltWrapper wraps bolt.DB to make it satisfy the DB interface.
type boltWrapper struct {
	db *bolt.DB
}

func (w boltWrapper) Update(fn func(Tx) error) error {
	return w.db.Update(func(tx *bolt.Tx) error {
		return fn(txWrapper{tx})
	})
}

func (w boltWrapper) View(fn func(Tx) error) error {
	return w.db.View(func(tx *bolt.Tx) error {
		return fn(txWrapper{tx})
	})
}

func (w boltWrapper) Close() error {
	return w.db.Close()
}
