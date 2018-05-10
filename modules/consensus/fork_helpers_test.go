package consensus

import "github.com/NebulousLabs/Sia/modules/consensus/database"

// dbBacktrackToCurrentPath is a convenience function to call
// backtrackToCurrentPath without a bolt.Tx.
func (cs *ConsensusSet) dbBacktrackToCurrentPath(b *database.Block) (bs []*database.Block) {
	_ = cs.db.Update(func(tx database.Tx) error {
		bs = backtrackToCurrentPath(tx, b)
		return nil
	})
	return bs
}

// dbRevertToNode is a convenience function to call revertToBlock without a
// bolt.Tx.
func (cs *ConsensusSet) dbRevertToNode(b *database.Block) (bs []*database.Block) {
	_ = cs.db.Update(func(tx database.Tx) error {
		bs = cs.revertToBlock(tx, b)
		return nil
	})
	return bs
}

// dbForkBlockchain is a convenience function to call forkBlockchain without a
// bolt.Tx.
func (cs *ConsensusSet) dbForkBlockchain(b *database.Block) (revertedBlocks, appliedBlocks []*database.Block, err error) {
	updateErr := cs.db.Update(func(tx database.Tx) error {
		revertedBlocks, appliedBlocks, err = cs.forkBlockchain(tx, b)
		return nil
	})
	if updateErr != nil {
		panic(updateErr)
	}
	return revertedBlocks, appliedBlocks, err
}
