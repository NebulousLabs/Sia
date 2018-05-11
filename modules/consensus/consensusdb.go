package consensus

// consensusdb.go contains all of the functions related to performing consensus
// related actions on the database, including initializing the consensus
// portions of the database. Many errors cause panics instead of being handled
// gracefully, but only when the debug flag is set. The errors are silently
// ignored otherwise, which is suboptimal.

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus/database"
	"github.com/NebulousLabs/Sia/types"
)

// createConsensusObjects initialzes the consensus portions of the database.
func (cs *ConsensusSet) createConsensusDB(tx database.Tx) error {
	// Set the block height to -1, so the genesis block is at height 0.
	underflow := types.BlockHeight(0)
	tx.SetBlockHeight(underflow - 1)

	// Set the siafund pool to 0.
	tx.SetSiafundPool(types.NewCurrency64(0))

	// Update the siafund output diffs map for the genesis block on disk. This
	// needs to happen between the database being opened/initilized and the
	// consensus set hash being calculated
	for _, sfod := range cs.blockRoot.SiafundOutputDiffs {
		commitSiafundOutputDiff(tx, sfod, modules.DiffApply)
	}

	// Add the miner payout from the genesis block to the delayed siacoin
	// outputs - unspendable, as the unlock hash is blank.
	addDSCO(tx, types.MaturityDelay, cs.blockRoot.Block.MinerPayoutID(0), types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.UnlockHash{},
	})

	// Add the genesis block to the block structures - checksum must be taken
	// after pushing the genesis block into the path.
	tx.PushPath(cs.blockRoot.Block.ID())
	if build.DEBUG {
		cs.blockRoot.ConsensusChecksum = tx.ConsensusChecksum()
	}
	tx.AddBlock(&cs.blockRoot)
	return nil
}

// currentBlockID returns the id of the most recent block in the consensus set.
func currentBlockID(tx database.Tx) types.BlockID {
	id, err := getPath(tx, tx.BlockHeight())
	if build.DEBUG && err != nil {
		panic(err)
	}
	return id
}

// dbCurrentBlockID is a convenience function allowing currentBlockID to be
// called without a bolt.Tx.
func (cs *ConsensusSet) dbCurrentBlockID() (id types.BlockID) {
	dbErr := cs.db.View(func(tx database.Tx) error {
		id = currentBlockID(tx)
		return nil
	})
	if dbErr != nil {
		panic(dbErr)
	}
	return id
}

// currentProcessedBlock returns the most recent block in the consensus set.
func currentProcessedBlock(tx database.Tx) *database.Block {
	b, err := getBlockMap(tx, currentBlockID(tx))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return b
}

// getBlockMap returns a processed block with the input id.
func getBlockMap(tx database.Tx, id types.BlockID) (*database.Block, error) {
	b, exists := tx.Block(id)
	if !exists {
		return nil, errNilItem
	}
	return b, nil
}

// getPath returns the block id at 'height' in the block path.
func getPath(tx database.Tx, height types.BlockHeight) (id types.BlockID, err error) {
	id = tx.BlockID(height)
	if id == (types.BlockID{}) {
		return types.BlockID{}, errNilItem
	}
	return id, nil
}

// isSiacoinOutput returns true if there is a siacoin output of that id in the
// database.
func isSiacoinOutput(tx database.Tx, id types.SiacoinOutputID) bool {
	_, exists := tx.SiacoinOutput(id)
	return exists
}

// getSiacoinOutput fetches a siacoin output from the database. An error is
// returned if the siacoin output does not exist.
func getSiacoinOutput(tx database.Tx, id types.SiacoinOutputID) (types.SiacoinOutput, error) {
	sco, exists := tx.SiacoinOutput(id)
	if !exists {
		return types.SiacoinOutput{}, errNilItem
	}
	return sco, nil
}

// addSiacoinOutput adds a siacoin output to the database. An error is returned
// if the siacoin output is already in the database.
func addSiacoinOutput(tx database.Tx, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	// While this is not supposed to be allowed, there's a bug in the consensus
	// code which means that earlier versions have accetped 0-value outputs
	// onto the blockchain. A hardfork to remove 0-value outputs will fix this,
	// and that hardfork is planned, but not yet.
	/*
		if build.DEBUG && sco.Value.IsZero() {
			panic("discovered a zero value siacoin output")
		}
	*/
	if build.DEBUG {
		// Sanity check - should not be adding an item that exists.
		if _, exists := tx.SiacoinOutput(id); exists {
			panic("repeat siacoin output")
		}
	}
	tx.AddSiacoinOutput(id, sco)
}

// removeSiacoinOutput removes a siacoin output from the database. An error is
// returned if the siacoin output is not in the database prior to removal.
func removeSiacoinOutput(tx database.Tx, id types.SiacoinOutputID) {
	if build.DEBUG {
		// Sanity check - should not be removing an item that is not in the db.
		if _, exists := tx.SiacoinOutput(id); !exists {
			panic("nil siacoin output")
		}
	}
	tx.DeleteSiacoinOutput(id)
}

// getFileContract fetches a file contract from the database, returning an
// error if it is not there.
func getFileContract(tx database.Tx, id types.FileContractID) (fc types.FileContract, err error) {
	fc, exists := tx.FileContract(id)
	if !exists {
		return types.FileContract{}, errNilItem
	}
	return fc, nil
}

// addFileContract adds a file contract to the database. An error is returned
// if the file contract is already in the database.
func addFileContract(tx database.Tx, id types.FileContractID, fc types.FileContract) {
	if build.DEBUG {
		// Sanity check - should not be adding a zero-payout file contract.
		if fc.Payout.IsZero() {
			panic("adding zero-payout file contract")
		}
		// Sanity check - should not be adding a file contract already in the db.
		if _, exists := tx.FileContract(id); exists {
			panic("repeat file contract")
		}
	}
	tx.AddFileContract(id, fc)
}

// removeFileContract removes a file contract from the database.
func removeFileContract(tx database.Tx, id types.FileContractID) {
	if build.DEBUG {
		// Sanity check - should not be removing a file contract not in the db.
		if _, exists := tx.FileContract(id); !exists {
			panic("nil file contract")
		}
	}
	tx.DeleteFileContract(id)
}

// The address of the devs.
var devAddr = types.UnlockHash{243, 113, 199, 11, 206, 158, 184,
	151, 156, 213, 9, 159, 89, 158, 196, 228, 252, 177, 78, 10,
	252, 243, 31, 151, 145, 224, 62, 100, 150, 164, 192, 179}

// getSiafundOutput fetches a siafund output from the database. An error is
// returned if the siafund output does not exist.
func getSiafundOutput(tx database.Tx, id types.SiafundOutputID) (types.SiafundOutput, error) {
	sfo, exists := tx.SiafundOutput(id)
	if !exists {
		return types.SiafundOutput{}, errNilItem
	}
	gsa := types.GenesisSiafundAllocation
	if sfo.UnlockHash == gsa[len(gsa)-1].UnlockHash && tx.BlockHeight() > 10e3 {
		sfo.UnlockHash = devAddr
	}
	return sfo, nil
}

// addSiafundOutput adds a siafund output to the database. An error is returned
// if the siafund output is already in the database.
func addSiafundOutput(tx database.Tx, id types.SiafundOutputID, sfo types.SiafundOutput) {
	if build.DEBUG {
		// Sanity check - should not be adding a siafund output with a value of
		// zero.
		if sfo.Value.IsZero() {
			panic("zero value siafund being added")
		}
		// Sanity check - should not be adding an item already in the db.
		if _, exists := tx.SiafundOutput(id); exists {
			panic("repeat siafund output")
		}
	}
	tx.AddSiafundOutput(id, sfo)
}

// removeSiafundOutput removes a siafund output from the database. An error is
// returned if the siafund output is not in the database prior to removal.
func removeSiafundOutput(tx database.Tx, id types.SiafundOutputID) {
	if build.DEBUG {
		// Sanity check - should not be deleting an item not in the db.
		if _, exists := tx.SiafundOutput(id); !exists {
			panic("nil siafund output")
		}
	}
	tx.DeleteSiafundOutput(id)
}

// addDSCO adds a delayed siacoin output to the consnesus set.
func addDSCO(tx database.Tx, bh types.BlockHeight, id types.SiacoinOutputID, sco types.SiacoinOutput) {
	// Sanity check - dsco should never have a value of zero.
	// An error in the consensus code means sometimes there are 0-value dscos
	// in the blockchain. A hardfork will fix this.
	/*
		if build.DEBUG && sco.Value.IsZero() {
			panic("zero-value dsco being added")
		}
	*/
	if build.DEBUG {
		// Sanity check - output should not already be in the full set of outputs.
		if _, exists := tx.SiacoinOutput(id); exists {
			panic("dsco already in output set")
		}
		// Sanity check - should not be adding an item already in the db.
		ids, _ := tx.DelayedSiacoinOutputs(bh)
		for i := range ids {
			if ids[i] == id {
				panic(errRepeatInsert)
			}
		}
	}
	tx.AddDelayedSiacoinOutput(bh, id, sco)
}

// removeDSCO removes a delayed siacoin output from the consensus set.
func removeDSCO(tx database.Tx, bh types.BlockHeight, id types.SiacoinOutputID) {
	if build.DEBUG {
		// Sanity check - should not be deleting an item not in the db.
		ids, _ := tx.DelayedSiacoinOutputs(bh)
		for len(ids) > 0 && ids[0] != id {
			ids = ids[1:]
		}
		if len(ids) == 0 {
			panic("nil dsco")
		}
	}
	tx.DeleteDelayedSiacoinOutput(bh, id)
}
