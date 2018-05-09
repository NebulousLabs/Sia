package consensus

// changelog.go implements a persistent changelog in the consenus database
// tracking all of the atomic changes to the consensus set. The primary use of
// the changelog is for subscribers that have persistence - instead of
// subscribing from the very beginning and receiving all changes from genesis
// each time the daemon starts up, the subscribers can start from the most
// recent change that they are familiar with.
//
// The changelog is set up as a singley linked list where each change points
// forward to the next change. In bolt, the key is a hash of the database.ChangeEntry
// and the value is a struct containing the database.ChangeEntry and the key of the next
// database.ChangeEntry. The empty hash key leads to the 'changeTail', which contains
// the id of the most recent database.ChangeEntry.
//
// Initialization only needs to worry about creating the blank change entry,
// the genesis block will call 'append' later on during initialization.

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus/database"
	"github.com/NebulousLabs/Sia/types"
)

// appendChangeLog adds a new change entry to the change log.
func appendChangeLog(tx database.Tx, ce database.ChangeEntry) error {
	tx.AppendChangeEntry(ce)
	return nil
}

// getEntry returns the change entry with a given id, using a bool to indicate
// existence.
func getEntry(tx database.Tx, id modules.ConsensusChangeID) (ce database.ChangeEntry, exists bool) {
	return tx.ChangeEntry(id)
}

// createChangeLog initializes the change log with the genesis block.
func (cs *ConsensusSet) createChangeLog(tx database.Tx) error {
	appendChangeLog(tx, cs.genesisEntry())
	return nil
}

// genesisEntry returns the id of the genesis block log entry.
func (cs *ConsensusSet) genesisEntry() database.ChangeEntry {
	return database.ChangeEntry{
		AppliedBlocks: []types.BlockID{cs.blockRoot.Block.ID()},
	}
}
