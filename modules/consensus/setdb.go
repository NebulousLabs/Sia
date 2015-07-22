package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
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
	errNilItem      = errors.New("Requested item does not exist")
)

// setDB is a wrapper around the persist bolt db which backs the
// consensus set
type setDB struct {
	*persist.BoltDatabase
}

// processedBlock is a copy/rename of blockNode, with the pointers to
// other blockNodes replaced with block ID's, and all the fields
// exported, so that a block node can be marshalled
type processedBlock struct {
	Block    types.Block
	Parent   types.BlockID
	Children []types.BlockID

	Height      types.BlockHeight
	Depth       types.Target
	ChildTarget types.Target

	DiffsGenerated            bool
	SiacoinOutputDiffs        []modules.SiacoinOutputDiff
	FileContractDiffs         []modules.FileContractDiff
	SiafundOutputDiffs        []modules.SiafundOutputDiff
	DelayedSiacoinOutputDiffs []modules.DelayedSiacoinOutputDiff
	SiafundPoolDiffs          []modules.SiafundPoolDiff

	ConsensusSetHash crypto.Hash
}

// bnToPb and pbToBn convert between blockNodes and
// processedBlocks. As block nodes will be replaced with
// processedBlocks, this code should be considered deprecated

// bnToPb converts a blockNode to a processed block
// DEPRECATED
func bnToPb(bn blockNode) processedBlock {
	pb := processedBlock{
		Block: bn.block,

		Height:      bn.height,
		Depth:       bn.depth,
		ChildTarget: bn.childTarget,

		DiffsGenerated:            bn.diffsGenerated,
		SiacoinOutputDiffs:        bn.siacoinOutputDiffs,
		FileContractDiffs:         bn.fileContractDiffs,
		SiafundOutputDiffs:        bn.siafundOutputDiffs,
		DelayedSiacoinOutputDiffs: bn.delayedSiacoinOutputDiffs,
		SiafundPoolDiffs:          bn.siafundPoolDiffs,

		ConsensusSetHash: bn.consensusSetHash,
	}
	for _, c := range bn.children {
		pb.Children = append(pb.Children, c.block.ID())
	}
	if bn.parent != nil {
		pb.Parent = bn.parent.block.ID()
	}

	return pb
}

// pbToBn exists to move a processed block to a block node. It
// requires the consensus block Map.
// DEPRECATED
func (cs *ConsensusSet) pbToBn(pb *processedBlock) blockNode {
	parent, exists := cs.blockMap[pb.Parent]
	if !exists {
		panic("block parent not in consensus set")
	}

	bn := blockNode{
		block:  pb.Block,
		parent: parent,

		height:      pb.Height,
		depth:       pb.Depth,
		childTarget: pb.ChildTarget,

		diffsGenerated:            pb.DiffsGenerated,
		siacoinOutputDiffs:        pb.SiacoinOutputDiffs,
		fileContractDiffs:         pb.FileContractDiffs,
		siafundOutputDiffs:        pb.SiafundOutputDiffs,
		delayedSiacoinOutputDiffs: pb.DelayedSiacoinOutputDiffs,
		siafundPoolDiffs:          pb.SiafundPoolDiffs,

		consensusSetHash: pb.ConsensusSetHash,
	}
	return bn
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
	}

	// Create buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucketName := range buckets {
			_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return err
			}
		}
		return nil
	})
	return &setDB{db}, nil
}

// addItem should only be called from this file, and adds a new item
// to the database
//
// addItem and getItem are part of consensus due to stricter error
// conditions than a generic bolt implementation
func (db *setDB) addItem(bucket string, key, value interface{}) error {
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
	return
}

// pushPath inserts a block into the database at the "end" of the chain, i.e.
// the current height + 1.
func (db *setDB) pushPath(block types.Block) error {
	value := encoding.Marshal(block.ID())
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Path"))
		key := encoding.EncUint64(uint64(b.Stats().KeyN))
		return b.Put(key, value)
	})
}

// popPath removes a block from the "end" of the chain, i.e. the block
// with the largest height.
func (db *setDB) popPath() error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Path"))
		key := encoding.EncUint64(uint64(b.Stats().KeyN - 1))
		return b.Delete(key)
	})
}

// path retreives the block id of a block at a given hegiht from the path
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

// addBlockMap adds a block node to the block map
func (db *setDB) addBlockMap(bn blockNode) error {
	return db.addItem("BlockMap", bn.block.ID(), bnToPb(bn))
}

// Function needs to have access to the blockMap inside consensusSet
// because bnFromPb requires the parent ID. This could be fixed at
// some point, as this function really should be part of setDB
func (db *setDB) getBlockMap(id types.BlockID) (*processedBlock, error) {
	bnBytes, err := db.getItem("BlockMap", id)
	if err != nil {
		return nil, err
	}
	var pb processedBlock
	err = encoding.Unmarshal(bnBytes, &pb)
	return &pb, err
}
