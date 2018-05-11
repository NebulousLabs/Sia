package database

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
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
	// time values are stored for each block id.
	bucketOak = []byte("Oak")

	// ChangeLog contains a list of atomic changes that have happened to the
	// consensus set so that subscribers can subscribe from the most recent
	// change they have seen.
	changeLog = []byte("ChangeLog")

	// ChangeLogTailID is a key that points to the id of the current changelog
	// tail.
	changeLogTailID = []byte("ChangeLogTailID")

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
		bucketOak,
		changeLog,
		changeLogTailID,
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

		// If consistency flag is not set, initialize it to false (no
		// inconsistencies).
		if tx.Bucket(consistency).Get(consistency) == nil {
			err = tx.Bucket(consistency).Put(consistency, encoding.Marshal(false))
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

// Block is a a block stored in the database along with relevant metadata.
type Block struct {
	types.Block
	Height      types.BlockHeight
	Depth       types.Target
	ChildTarget types.Target

	DiffsGenerated            bool
	SiacoinOutputDiffs        []modules.SiacoinOutputDiff
	FileContractDiffs         []modules.FileContractDiff
	SiafundOutputDiffs        []modules.SiafundOutputDiff
	DelayedSiacoinOutputDiffs []modules.DelayedSiacoinOutputDiff
	SiafundPoolDiffs          []modules.SiafundPoolDiff

	ConsensusChecksum crypto.Hash
}

// ChildDepth returns the depth of a blockNode's child nodes. The depth is the
// "sum" of the current depth and current difficulty. See target.Add for more
// detailed information.
func (b *Block) ChildDepth() types.Target {
	return b.Depth.AddDifficulties(b.ChildTarget)
}

// ChangeEntry records a single atomic change to the consensus set.
type ChangeEntry struct {
	RevertedBlocks []types.BlockID
	AppliedBlocks  []types.BlockID
	Next           modules.ConsensusChangeID
}

// ID returns the id of a change entry.
func (ce ChangeEntry) ID() modules.ConsensusChangeID {
	return modules.ConsensusChangeID(crypto.HashObject(ce))
}
