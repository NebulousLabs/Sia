package consensus

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules/consensus/database"
	"github.com/NebulousLabs/fastrand"
)

// manageErr handles an error detected by the consistency checks.
func manageErr(tx database.Tx, err error) {
	tx.MarkInconsistent()
	if build.DEBUG {
		panic(err)
	} else {
		fmt.Println(err)
	}
}

// checkRevertApply reverts the most recent block, checking to see that the
// consensus set hash matches the hash obtained for the previous block. Then it
// applies the block again and checks that the consensus set hash matches the
// original consensus set hash.
func (cs *ConsensusSet) checkRevertApply(tx database.Tx) {
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
	if tx.ConsensusChecksum() != parent.ConsensusChecksum {
		manageErr(tx, errors.New("consensus checksum mismatch after reverting"))
	}
	_, _, err = cs.forkBlockchain(tx, current)
	if err != nil {
		manageErr(tx, err)
	}
	if tx.ConsensusChecksum() != current.ConsensusChecksum {
		manageErr(tx, errors.New("consensus checksum mismatch after re-applying"))
	}
}

// checkConsistency runs a series of checks to make sure that the consensus set
// is consistent with some rules that should always be true.
func (cs *ConsensusSet) checkConsistency(tx database.Tx) {
	if cs.checkingConsistency {
		return
	}

	cs.checkingConsistency = true
	err := tx.CheckCurrencyCounts()
	if err != nil {
		manageErr(tx, err)
	}
	if build.DEBUG {
		cs.checkRevertApply(tx)
	}
	cs.checkingConsistency = false
}

// maybeCheckConsistency runs a consistency check with a small probability.
// Useful for detecting database corruption in production without needing to go
// through the extremely slow process of running a consistency check every
// block.
func (cs *ConsensusSet) maybeCheckConsistency(tx database.Tx) {
	if fastrand.Intn(1000) == 0 {
		cs.checkConsistency(tx)
	}
}

// TODO: Check that every file contract has an expiration too, and that the
// number of file contracts + the number of expirations is equal.
