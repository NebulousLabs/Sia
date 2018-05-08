package database

import (
	"bytes"
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
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
