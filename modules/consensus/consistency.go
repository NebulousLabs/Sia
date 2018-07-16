package consensus

import (
	"bytes"
	"errors"
	"fmt"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/encoding"
	"gitlab.com/NebulousLabs/Sia/types"
	"gitlab.com/NebulousLabs/fastrand"

	"github.com/coreos/bbolt"
)

// manageErr handles an error detected by the consistency checks.
func manageErr(tx *bolt.Tx, err error) {
	markInconsistency(tx)
	if build.DEBUG {
		panic(err)
	} else {
		fmt.Println(err)
	}
}

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

// checkSiacoinCount checks that the number of siacoins countable within the
// consensus set equal the expected number of siacoins for the block height.
func checkSiacoinCount(tx *bolt.Tx) {
	// Iterate through all the buckets looking for the delayed siacoin output
	// buckets, and check that they are for the correct heights.
	var dscoSiacoins types.Currency
	err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		// Check if the bucket is a delayed siacoin output bucket.
		if !bytes.HasPrefix(name, prefixDSCO) {
			return nil
		}

		// Sum up the delayed outputs in this bucket.
		err := b.ForEach(func(_, delayedOutput []byte) error {
			var sco types.SiacoinOutput
			err := encoding.Unmarshal(delayedOutput, &sco)
			if err != nil {
				manageErr(tx, err)
			}
			dscoSiacoins = dscoSiacoins.Add(sco.Value)
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		manageErr(tx, err)
	}

	// Add all of the siacoin outputs.
	var scoSiacoins types.Currency
	err = tx.Bucket(SiacoinOutputs).ForEach(func(_, scoBytes []byte) error {
		var sco types.SiacoinOutput
		err := encoding.Unmarshal(scoBytes, &sco)
		if err != nil {
			manageErr(tx, err)
		}
		scoSiacoins = scoSiacoins.Add(sco.Value)
		return nil
	})
	if err != nil {
		manageErr(tx, err)
	}

	// Add all of the payouts from file contracts.
	var fcSiacoins types.Currency
	err = tx.Bucket(FileContracts).ForEach(func(_, fcBytes []byte) error {
		var fc types.FileContract
		err := encoding.Unmarshal(fcBytes, &fc)
		if err != nil {
			manageErr(tx, err)
		}
		var fcCoins types.Currency
		for _, output := range fc.ValidProofOutputs {
			fcCoins = fcCoins.Add(output.Value)
		}
		fcSiacoins = fcSiacoins.Add(fcCoins)
		return nil
	})
	if err != nil {
		manageErr(tx, err)
	}

	// Add all of the siafund claims.
	var claimSiacoins types.Currency
	err = tx.Bucket(SiafundOutputs).ForEach(func(_, sfoBytes []byte) error {
		var sfo types.SiafundOutput
		err := encoding.Unmarshal(sfoBytes, &sfo)
		if err != nil {
			manageErr(tx, err)
		}

		coinsPerFund := getSiafundPool(tx).Sub(sfo.ClaimStart)
		claimCoins := coinsPerFund.Mul(sfo.Value).Div(types.SiafundCount)
		claimSiacoins = claimSiacoins.Add(claimCoins)
		return nil
	})
	if err != nil {
		manageErr(tx, err)
	}

	expectedSiacoins := types.CalculateNumSiacoins(blockHeight(tx))
	totalSiacoins := dscoSiacoins.Add(scoSiacoins).Add(fcSiacoins).Add(claimSiacoins)
	if !totalSiacoins.Equals(expectedSiacoins) {
		diagnostics := fmt.Sprintf("Wrong number of siacoins\nDsco: %v\nSco: %v\nFc: %v\nClaim: %v\n", dscoSiacoins, scoSiacoins, fcSiacoins, claimSiacoins)
		if totalSiacoins.Cmp(expectedSiacoins) < 0 {
			diagnostics += fmt.Sprintf("total: %v\nexpected: %v\n expected is bigger: %v", totalSiacoins, expectedSiacoins, expectedSiacoins.Sub(totalSiacoins))
		} else {
			diagnostics += fmt.Sprintf("total: %v\nexpected: %v\n expected is bigger: %v", totalSiacoins, expectedSiacoins, totalSiacoins.Sub(expectedSiacoins))
		}
		manageErr(tx, errors.New(diagnostics))
	}
}

// checkSiafundCount checks that the number of siafunds countable within the
// consensus set equal the expected number of siafunds for the block height.
func checkSiafundCount(tx *bolt.Tx) {
	var total types.Currency
	err := tx.Bucket(SiafundOutputs).ForEach(func(_, siafundOutputBytes []byte) error {
		var sfo types.SiafundOutput
		err := encoding.Unmarshal(siafundOutputBytes, &sfo)
		if err != nil {
			manageErr(tx, err)
		}
		total = total.Add(sfo.Value)
		return nil
	})
	if err != nil {
		manageErr(tx, err)
	}
	if !total.Equals(types.SiafundCount) {
		manageErr(tx, errors.New("wrong number of siafunds in the consensus set"))
	}
}

// checkDSCOs scans the sets of delayed siacoin outputs and checks for
// consistency.
func checkDSCOs(tx *bolt.Tx) {
	// Create a map to track which delayed siacoin output maps exist, and
	// another map to track which ids have appeared in the dsco set.
	dscoTracker := make(map[types.BlockHeight]struct{})
	idMap := make(map[types.SiacoinOutputID]struct{})

	// Iterate through all the buckets looking for the delayed siacoin output
	// buckets, and check that they are for the correct heights.
	err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		// If the bucket is not a delayed siacoin output bucket or a file
		// contract expiration bucket, skip.
		if !bytes.HasPrefix(name, prefixDSCO) {
			return nil
		}

		// Add the bucket to the dscoTracker.
		var height types.BlockHeight
		err := encoding.Unmarshal(name[len(prefixDSCO):], &height)
		if err != nil {
			manageErr(tx, err)
		}
		_, exists := dscoTracker[height]
		if exists {
			return errors.New("repeat dsco map")
		}
		dscoTracker[height] = struct{}{}

		var total types.Currency
		err = b.ForEach(func(idBytes, delayedOutput []byte) error {
			// Check that the output id has not appeared in another dsco.
			var id types.SiacoinOutputID
			copy(id[:], idBytes)
			_, exists := idMap[id]
			if exists {
				return errors.New("repeat delayed siacoin output")
			}
			idMap[id] = struct{}{}

			// Sum the funds in the bucket.
			var sco types.SiacoinOutput
			err := encoding.Unmarshal(delayedOutput, &sco)
			if err != nil {
				manageErr(tx, err)
			}
			total = total.Add(sco.Value)
			return nil
		})
		if err != nil {
			return err
		}

		// Check that the minimum value has been achieved - the coinbase from
		// an earlier block is guaranteed to be in the bucket.
		minimumValue := types.CalculateCoinbase(height - types.MaturityDelay)
		if total.Cmp(minimumValue) < 0 {
			return errors.New("total number of coins in the delayed output bucket is incorrect")
		}
		return nil
	})
	if err != nil {
		manageErr(tx, err)
	}

	// Check that all of the correct heights are represented.
	currentHeight := blockHeight(tx)
	expectedBuckets := 0
	for i := currentHeight + 1; i <= currentHeight+types.MaturityDelay; i++ {
		if i < types.MaturityDelay {
			continue
		}
		_, exists := dscoTracker[i]
		if !exists {
			manageErr(tx, errors.New("missing a dsco bucket"))
		}
		expectedBuckets++
	}
	if len(dscoTracker) != expectedBuckets {
		manageErr(tx, errors.New("too many dsco buckets"))
	}
}

// checkRevertApply reverts the most recent block, checking to see that the
// consensus set hash matches the hash obtained for the previous block. Then it
// applies the block again and checks that the consensus set hash matches the
// original consensus set hash.
func (cs *ConsensusSet) checkRevertApply(tx *bolt.Tx) {
	current := currentProcessedBlock(tx)
	// Don't perform the check if this block is the genesis block.
	if current.Block.ID() == cs.blockRoot.Block.ID() {
		return
	}

	parent, err := getBlockMap(tx, current.Block.ParentID)
	if err != nil {
		manageErr(tx, err)
	}
	if current.Height != parent.Height+1 {
		manageErr(tx, errors.New("parent structure of a block is incorrect"))
	}
	_, _, err = cs.forkBlockchain(tx, parent)
	if err != nil {
		manageErr(tx, err)
	}
	if consensusChecksum(tx) != parent.ConsensusChecksum {
		manageErr(tx, errors.New("consensus checksum mismatch after reverting"))
	}
	_, _, err = cs.forkBlockchain(tx, current)
	if err != nil {
		manageErr(tx, err)
	}
	if consensusChecksum(tx) != current.ConsensusChecksum {
		manageErr(tx, errors.New("consensus checksum mismatch after re-applying"))
	}
}

// checkConsistency runs a series of checks to make sure that the consensus set
// is consistent with some rules that should always be true.
func (cs *ConsensusSet) checkConsistency(tx *bolt.Tx) {
	if cs.checkingConsistency {
		return
	}

	cs.checkingConsistency = true
	checkDSCOs(tx)
	checkSiacoinCount(tx)
	checkSiafundCount(tx)
	if build.DEBUG {
		cs.checkRevertApply(tx)
	}
	cs.checkingConsistency = false
}

// maybeCheckConsistency runs a consistency check with a small probability.
// Useful for detecting database corruption in production without needing to go
// through the extremely slow process of running a consistency check every
// block.
func (cs *ConsensusSet) maybeCheckConsistency(tx *bolt.Tx) {
	if fastrand.Intn(1000) == 0 {
		cs.checkConsistency(tx)
	}
}

// TODO: Check that every file contract has an expiration too, and that the
// number of file contracts + the number of expirations is equal.
