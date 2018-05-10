package consensus

import "github.com/NebulousLabs/Sia/modules/consensus/database"

// dbBacktrackToCurrentPath is a convenience function to call
// backtrackToCurrentPath without a bolt.Tx.
func (cs *ConsensusSet) dbBacktrackToCurrentPath(pb *database.Block) (pbs []*database.Block) {
	_ = cs.db.Update(func(tx database.Tx) error {
		pbs = backtrackToCurrentPath(tx, pb)
		return nil
	})
	return pbs
}

// dbRevertToNode is a convenience function to call revertToBlock without a
// bolt.Tx.
func (cs *ConsensusSet) dbRevertToNode(pb *database.Block) (pbs []*database.Block) {
	_ = cs.db.Update(func(tx database.Tx) error {
		pbs = cs.revertToBlock(tx, pb)
		return nil
	})
	return pbs
}

// dbForkBlockchain is a convenience function to call forkBlockchain without a
// bolt.Tx.
func (cs *ConsensusSet) dbForkBlockchain(pb *database.Block) (revertedBlocks, appliedBlocks []*database.Block, err error) {
	updateErr := cs.db.Update(func(tx database.Tx) error {
		revertedBlocks, appliedBlocks, err = cs.forkBlockchain(tx, pb)
		return nil
	})
	if updateErr != nil {
		panic(updateErr)
	}
	return revertedBlocks, appliedBlocks, err
}
