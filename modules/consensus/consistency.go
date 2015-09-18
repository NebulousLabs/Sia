package consensus

import (
	"errors"
	"strings"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
)

var (
	errSiacoinMiscount = errors.New("consensus set has the wrong number of siacoins given the height")
	errSiafundMiscount = errors.New("consensus set has the wrong number of siafunds")
)

// consensusChecksum grabs a checksum of the consensus set by pushing all of
// the elements in sorted order into a merkle tree and taking the root. All
// consensus sets with the same current block should have identical consensus
// checksums.
func consensusChecksum(tx *bolt.Tx) crypto.Hash {
	// Create a checksum tree.
	tree := crypto.NewTree()

	// For all of the constant buckets, push every key and every value. Buckets
	// are sorted in byte-order, therefore this operation is deterministic.
	consensusSetBuckets := []*bolt.Bucket{
		tx.Bucket(BlockPath),
		tx.Bucket(SiacoinOutputs),
		tx.Bucket(FileContracts),
		tx.Bucket(SiafundOutputs),
		tx.Bucket(SiafundPool),
	}
	for i := range consensusSetBuckets {
		err := consensusSetBuckets[i].ForEach(func(k, v []byte) error {
			tree.Push(k)
			tree.Push(v)
			return nil
		})
		if build.DEBUG && err != nil {
			panic(err)
		}
	}

	// Iterate through all the buckets looking for buckets prefixed with
	// prefixDSCO, and add all of the delayed siacoin outputs. Buckets are
	// presented in byte-sorted order by name.
	err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		// If the bucket is not a delayed siacoin output bucket or a file
		// contract expiration bucket, skip.
		if !strings.HasPrefix(string(name), string(prefixDSCO)) && !strings.HasPrefix(string(name), string(prefixFCEX)) {
			return nil
		}

		// The bucket is a prefixed bucket - add all elements to the tree.
		return b.ForEach(func(k, v []byte) error {
			tree.Push(k)
			tree.Push(v)
			return nil
		})
	})
	if build.DEBUG && err != nil {
		panic(err)
	}

	return tree.Root()
}

// checkSiacoinCount checks that the number of siacoins countable within the
// consensus set equal the expected number of siacoins for the block height.
func checkSiacoinCount(tx *bolt.Tx) error {
	return nil
}

// checkDSCOs scans the sets of delayed siacoin outputs and checks for
// consistency.
func checkDSCOs(tx *bolt.Tx) error {
	return nil
}

// checkRevertApply reverts the most recent block, checking to see that the
// consensus set hash matches the hash obtained for the previous block. Then it
// applies the block again and checks that the consensus set hash matches the
// original consensus set hash.
func (cs *ConsensusSet) checkRevertApply(tx *bolt.Tx) error {
	current := currentProcessedBlock(tx)
	parent, err := getBlockMap(tx, current.Block.ParentID)
	if err != nil {
		return err
	}
	_, _, err = cs.forkBlockchain(tx, parent)
	if err != nil {
		return err
	}
	if consensusChecksum(tx) != parent.ConsensusChecksum {
		return errors.New("consensus checksum mismatch after reverting")
	}
	_, _, err = cs.forkBlockchain(tx, current)
	if err != nil {
		return err
	}
	if consensusChecksum(tx) != current.ConsensusChecksum {
		return errors.New("consensus checksum mismatch after re-applying")
	}
	return nil
}

// checkConsistency runs a series of checks to make sure that the consensus set
// is consistent with some rules that should always be true.
func (cs *ConsensusSet) checkConsistency(tx *bolt.Tx) error {
	err := checkSiacoinCount(tx)
	if err != nil {
		return err
	}
	err = checkDSCOs(tx)
	if err != nil {
		return err
	}
	// err = checkSiafundCount(tx)
	if err != nil {
		return err
	}
	err = cs.checkRevertApply(tx)
	if err != nil {
		return err
	}
	return nil
}
