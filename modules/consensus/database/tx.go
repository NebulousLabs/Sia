package database

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/coreos/bbolt"
)

// A Tx is a database transaction.
type Tx interface {
	Bucket(name []byte) *bolt.Bucket
	CreateBucket(name []byte) (*bolt.Bucket, error)
	CreateBucketIfNotExists(name []byte) (*bolt.Bucket, error)
	DeleteBucket(name []byte) error
	ForEach(func([]byte, *bolt.Bucket) error) error

	// ConsensusChecksum grabs a checksum of the consensus set by pushing all
	// of the elements in sorted order into a Merkle tree and taking the root.
	// All consensus sets with the same current block should have identical
	// consensus checksums.
	ConsensusChecksum() crypto.Hash

	// MarkInconsistent marks the database as inconsistent.
	MarkInconsistent()

	// SiafundPool returns the value of the Siafund pool.
	SiafundPool() types.Currency
	// SetSiafundPool sets the value of the Siafund pool.
	SetSiafundPool(pool types.Currency)

	// BlockHeight returns the height of the blockchain.
	BlockHeight() types.BlockHeight
	// SetBlockHeight sets the height of the blockchain.
	SetBlockHeight(height types.BlockHeight)

	// PushPath appends a BlockID to the current path.
	PushPath(id types.BlockID)
	// PopPath removes the last BlockID in the current path.
	PopPath()
	// BlockID returns the ID of the block at the specified height in the
	// current path.
	BlockID(height types.BlockHeight) types.BlockID

	// ChangeEntry returns the ChangeEntry with the specified id.
	ChangeEntry(id modules.ConsensusChangeID) (ChangeEntry, bool)
	// AppendChangeEntry appends ce to the list of change entries.
	AppendChangeEntry(ce ChangeEntry)

	// DifficultyTotals returns the difficulty adjustment parameters for a
	// given block.
	DifficultyTotals(id types.BlockID) (totalTime int64, totalTarget types.Target)
	// SetDifficultyTotals sets the difficulty adjustment parameters for a
	// given block.
	SetDifficultyTotals(id types.BlockID, totalTime int64, totalTarget types.Target)
}

type txWrapper struct {
	*bolt.Tx
}

// manageErr handles an error detected by the consistency checks.
func manageErr(tx Tx, err error) {
	tx.MarkInconsistent()
	if build.DEBUG {
		panic(err)
	} else {
		fmt.Println(err)
	}
}

// ConsensusChecksum implements the Tx interface.
func (tx txWrapper) ConsensusChecksum() crypto.Hash {
	// Create a checksum tree.
	tree := crypto.NewTree()

	// For all of the constant buckets, push every key and every value. Buckets
	// are sorted in byte-order, therefore this operation is deterministic.
	consensusSetBuckets := []*bolt.Bucket{
		tx.Bucket(blockPath),
		tx.Bucket(siacoinOutputs),
		tx.Bucket(fileContracts),
		tx.Bucket(siafundOutputs),
		tx.Bucket(siafundPool),
	}
	for i := range consensusSetBuckets {
		err := consensusSetBuckets[i].ForEach(func(k, v []byte) error {
			tree.Push(k)
			tree.Push(v)
			return nil
		})
		if err != nil {
			manageErr(tx, err)
		}
	}

	// Iterate through all the buckets looking for buckets prefixed with
	// prefixDSCO or prefixFCEX. Buckets are presented in byte-sorted order by
	// name.
	err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		// If the bucket is not a delayed siacoin output bucket or a file
		// contract expiration bucket, skip.
		if !bytes.HasPrefix(name, prefixDSCO) && !bytes.HasPrefix(name, prefixFCEX) {
			return nil
		}

		// The bucket is a prefixed bucket - add all elements to the tree.
		return b.ForEach(func(k, v []byte) error {
			tree.Push(k)
			tree.Push(v)
			return nil
		})
	})
	if err != nil {
		manageErr(tx, err)
	}

	return tree.Root()
}

// MarkInconsistent implements the Tx interface.
func (tx txWrapper) MarkInconsistent() {
	cerr := tx.Bucket(consistency).Put(consistency, encoding.Marshal(true))
	if build.DEBUG && cerr != nil {
		panic(cerr)
	}
}

// SiafundPool implements the Tx interface.
func (tx txWrapper) SiafundPool() types.Currency {
	var pool types.Currency
	err := encoding.Unmarshal(tx.Bucket(siafundPool).Get(siafundPool), &pool)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return pool
}

// SetSiafundPool implements the Tx interface.
func (tx txWrapper) SetSiafundPool(pool types.Currency) {
	err := tx.Bucket(siafundPool).Put(siafundPool, encoding.Marshal(pool))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// BlockHeight implements the Tx interface.
func (tx txWrapper) BlockHeight() types.BlockHeight {
	var height types.BlockHeight
	err := encoding.Unmarshal(tx.Bucket(blockHeight).Get(blockHeight), &height)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return height
}

// SetBlockHeight implements the Tx interface.
func (tx txWrapper) SetBlockHeight(height types.BlockHeight) {
	err := tx.Bucket(blockHeight).Put(blockHeight, encoding.Marshal(height))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// BlockID implements the Tx interface.
func (tx txWrapper) BlockID(height types.BlockHeight) types.BlockID {
	var id types.BlockID
	copy(id[:], tx.Bucket(blockPath).Get(encoding.Marshal(height)))
	return id
}

// PushPath implements the Tx interface.
func (tx txWrapper) PushPath(id types.BlockID) {
	newHeight := tx.BlockHeight() + 1
	tx.SetBlockHeight(newHeight)

	err := tx.Bucket(blockPath).Put(encoding.Marshal(newHeight), id[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// PopPath implements the Tx interface.
func (tx txWrapper) PopPath() {
	oldHeight := tx.BlockHeight()
	tx.SetBlockHeight(oldHeight - 1)

	err := tx.Bucket(blockPath).Delete(encoding.Marshal(oldHeight))
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// ChangeEntry implements the Tx interface.
func (tx txWrapper) ChangeEntry(id modules.ConsensusChangeID) (ChangeEntry, bool) {
	var cn changeNode
	changeNodeBytes := tx.Bucket(changeLog).Get(id[:])
	if changeNodeBytes == nil {
		return ChangeEntry{}, false
	}
	err := encoding.Unmarshal(changeNodeBytes, &cn)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return cn.Entry, true
}

// AppendChangeEntry implements the Tx interface.
func (tx txWrapper) AppendChangeEntry(ce ChangeEntry) {
	ceid := ce.ID()
	b := tx.Bucket(changeLog)
	err := b.Put(ceid[:], encoding.Marshal(changeNode{Entry: ce}))
	if build.DEBUG && err != nil {
		panic(err)
	}

	// If this is not the first change entry, update the previous entry to
	// point to this one.
	if tailID := b.Get(changeLogTailID); tailID != nil {
		var tailCN changeNode
		err = encoding.Unmarshal(b.Get(tailID), &tailCN)
		if build.DEBUG && err != nil {
			panic(err)
		}
		tailCN.Next = ceid
		err = b.Put(tailID, encoding.Marshal(tailCN))
		if build.DEBUG && err != nil {
			panic(err)
		}
	}

	// Update the tail ID.
	err = b.Put(changeLogTailID, ceid[:])
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// DifficultyTotals implements the Tx interface.
func (tx txWrapper) DifficultyTotals(id types.BlockID) (totalTime int64, totalTarget types.Target) {
	bytes := tx.Bucket(bucketOak).Get(id[:])
	if bytes == nil {
		return 0, types.Target{}
	}
	totalTime = int64(binary.LittleEndian.Uint64(bytes[:8]))
	copy(totalTarget[:], bytes[8:])
	return
}

// SetDifficultyTotals implements the Tx interface.
func (tx txWrapper) SetDifficultyTotals(id types.BlockID, totalTime int64, totalTarget types.Target) {
	bytes := make([]byte, 40)
	binary.LittleEndian.PutUint64(bytes[:8], uint64(totalTime))
	copy(bytes[8:], totalTarget[:])
	err := tx.Bucket(bucketOak).Put(id[:], bytes)
	if build.DEBUG && err != nil {
		panic(err)
	}
}
