package consensus

import (
	"github.com/coreos/bbolt"
)

// dbBacktrackToCurrentPath is a convenience function to call
// backtrackToCurrentPath without a bolt.Tx.
func (cs *ConsensusSet) dbBacktrackToCurrentPath(pb *processedBlock) (pbs []*processedBlock) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pbs = backtrackToCurrentPath(tx, pb)
		return nil
	})
	return pbs
}

// dbRevertToNode is a convenience function to call revertToBlock without a
// bolt.Tx.
func (cs *ConsensusSet) dbRevertToNode(pb *processedBlock) (pbs []*processedBlock) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pbs = cs.revertToBlock(tx, pb)
		return nil
	})
	return pbs
}

// dbForkBlockchain is a convenience function to call forkBlockchain without a
// bolt.Tx.
func (cs *ConsensusSet) dbForkBlockchain(pb *processedBlock) (revertedBlocks, appliedBlocks []*processedBlock, err error) {
	updateErr := cs.db.Update(func(tx *bolt.Tx) error {
		revertedBlocks, appliedBlocks, err = cs.forkBlockchain(tx, pb)
		return nil
	})
	if updateErr != nil {
		panic(updateErr)
	}
	return revertedBlocks, appliedBlocks, err
}
