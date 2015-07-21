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
	Version: "0.1",
	Header:  "Consensus Set Backing Database",
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

func bnToPb(bn blockNode) processedBlock {
	pb := processedBlock{
		Block:  bn.block,
		Parent: bn.parent.block.ID(),

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
	return pb
}

// pbToBn exists to move a processed block to a block node. It
// requires the consensus block Map. Its current placement in this
// file is a bit awkward, so it should be moved some point in the near
// future
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

func openDB(filename string) (*setDB, error) {
	db, err := persist.OpenDatabase(meta, filename)
	if err != nil {
		return nil, err
	}

	var buckets []string = []string{
		"Path", "BlockMap",
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

// addItem and getItem are part of consensus due to stricter error conditions.
//
// addItem should only be called from this file, and adds a new item
// to the database
func (db *setDB) addItem(bucket string, key, value interface{}) error {
	v := encoding.Marshal(value)
	k := encoding.Marshal(key)
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
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

func (db *setDB) getItem(bucket string, key interface{}) (item []byte, err error) {
	k := encoding.Marshal(key)
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if build.DEBUG {
			if b == nil {
				panic(errNilBucket)
			}
		}
		item = b.Get(k)
		if build.DEBUG {
			if item == nil {
				panic(errNilItem)
			}
		}
		return nil
	})
	return
}

// AddBlock inserts a block into the database at the "end" of the chain, i.e.
// the current height + 1.
func (db *setDB) addPath(block types.Block) error {
	value := encoding.Marshal(block.ID())
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Path"))
		key := encoding.EncUint64(uint64(b.Stats().KeyN))
		return b.Put(key, value)
	})
}

// RemoveBlock removes a block from the "end" of the chain, i.e. the block
// with the largest height.
func (db *setDB) rmPath() error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Path"))
		key := encoding.EncUint64(uint64(b.Stats().KeyN - 1))
		return b.Delete(key)
	})
}

// path retreives the block id of a block at a given hegiht from the path
func (db *setDB) path(h types.BlockHeight) (id types.BlockID, err error) {
	idBytes, err := db.getItem("Path", h)
	if err != nil {
		return
	}
	err = encoding.Unmarshal(idBytes, &id)
	return
}

// pathHeight returns the size of the current path
func (db *setDB) pathHeight() (types.BlockHeight, error) {
	h, err := db.BucketSize("Path")
	return types.BlockHeight(h), err
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
