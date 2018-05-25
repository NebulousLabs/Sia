package consensus

// changelog.go implements a persistent changelog in the consenus database
// tracking all of the atomic changes to the consensus set. The primary use of
// the changelog is for subscribers that have persistence - instead of
// subscribing from the very beginning and receiving all changes from genesis
// each time the daemon starts up, the subscribers can start from the most
// recent change that they are familiar with.
//
// The changelog is set up as a singley linked list where each change points
// forward to the next change. In bolt, the key is a hash of the changeEntry
// and the value is a struct containing the changeEntry and the key of the next
// changeEntry. The empty hash key leads to the 'changeTail', which contains
// the id of the most recent changeEntry.
//
// Initialization only needs to worry about creating the blank change entry,
// the genesis block will call 'append' later on during initialization.

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/coreos/bbolt"
)

var (
	// ChangeLog contains a list of atomic changes that have happened to the
	// consensus set so that subscribers can subscribe from the most recent
	// change they have seen.
	ChangeLog = []byte("ChangeLog")

	// ChangeLogTailID is a key that points to the id of the current changelog
	// tail.
	ChangeLogTailID = []byte("ChangeLogTailID")
)

type (
	// changeEntry records a single atomic change to the consensus set.
	changeEntry struct {
		RevertedBlocks []types.BlockID
		AppliedBlocks  []types.BlockID
	}

	// changeNode contains a change entry and a pointer to the next change
	// entry, and is the object that gets stored in the database.
	changeNode struct {
		Entry changeEntry
		Next  modules.ConsensusChangeID
	}
)

// appendChangeLog adds a new change entry to the change log.
func appendChangeLog(tx *bolt.Tx, ce changeEntry) error {
	// Insert the change entry.
	cl := tx.Bucket(ChangeLog)
	ceid := ce.ID()
	cn := changeNode{Entry: ce, Next: modules.ConsensusChangeID{}}
	err := cl.Put(ceid[:], encoding.Marshal(cn))
	if err != nil {
		return err
	}

	// Update the tail node to point to the new change entry as the next entry.
	var tailID modules.ConsensusChangeID
	copy(tailID[:], cl.Get(ChangeLogTailID))
	if tailID != (modules.ConsensusChangeID{}) {
		// Get the old tail node.
		var tailCN changeNode
		tailCNBytes := cl.Get(tailID[:])
		err = encoding.Unmarshal(tailCNBytes, &tailCN)
		if err != nil {
			return err
		}

		// Point the 'next' of the old tail node to the new tail node and
		// insert.
		tailCN.Next = ceid
		err = cl.Put(tailID[:], encoding.Marshal(tailCN))
		if err != nil {
			return err
		}
	}

	// Update the tail id.
	err = cl.Put(ChangeLogTailID, ceid[:])
	if err != nil {
		return err
	}
	return nil
}

// getEntry returns the change entry with a given id, using a bool to indicate
// existence.
func getEntry(tx *bolt.Tx, id modules.ConsensusChangeID) (ce changeEntry, exists bool) {
	var cn changeNode
	cl := tx.Bucket(ChangeLog)
	changeNodeBytes := cl.Get(id[:])
	if changeNodeBytes == nil {
		return changeEntry{}, false
	}
	err := encoding.Unmarshal(changeNodeBytes, &cn)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return cn.Entry, true
}

// ID returns the id of a change entry.
func (ce *changeEntry) ID() modules.ConsensusChangeID {
	return modules.ConsensusChangeID(crypto.HashObject(ce))
}

// NextEntry returns the entry after the current entry.
func (ce *changeEntry) NextEntry(tx *bolt.Tx) (nextEntry changeEntry, exists bool) {
	// Get the change node associated with the provided change entry.
	ceid := ce.ID()
	var cn changeNode
	cl := tx.Bucket(ChangeLog)
	changeNodeBytes := cl.Get(ceid[:])
	err := encoding.Unmarshal(changeNodeBytes, &cn)
	if err != nil {
		build.Critical(err)
	}

	return getEntry(tx, cn.Next)
}

// createChangeLog assumes that no change log exists and creates a new one.
func (cs *ConsensusSet) createChangeLog(tx *bolt.Tx) error {
	// Create the changelog bucket.
	cl, err := tx.CreateBucket(ChangeLog)
	if err != nil {
		return err
	}

	// Add the genesis block as the first entry of the change log.
	ge := cs.genesisEntry()
	geid := ge.ID()
	cn := changeNode{
		Entry: ge,
		Next:  modules.ConsensusChangeID{},
	}
	err = cl.Put(geid[:], encoding.Marshal(cn))
	if err != nil {
		return err
	}
	err = cl.Put(ChangeLogTailID, geid[:])
	if err != nil {
		return err
	}
	return nil
}

// genesisEntry returns the id of the genesis block log entry.
func (cs *ConsensusSet) genesisEntry() changeEntry {
	return changeEntry{
		AppliedBlocks: []types.BlockID{cs.blockRoot.Block.ID()},
	}
}
