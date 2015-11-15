package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationChangeLog does a general test of the changelog by creating a
// subscriber that subscribes partway into startup and checking that the
// correct ordering of blocks are provided.
func TestIntegrationChangeLog(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Get a blank consensus set tester so that the mocked subscriber can join
	// immediately after genesis.
	cst, err := blankConsensusSetTester("TestIntegrationChangeLog")
	if err != nil {
		t.Fatal(err)
	}

	// Add a mocked subscriber and check that it receives the correct number of
	// blocks.
	ms := newMockSubscriber()
	cst.cs.ConsensusSetPersistentSubscribe(&ms, modules.ConsensusChangeID{})
	if ms.updates[0].AppliedBlocks[0].ID() != cst.cs.blockRoot.Block.ID() {
		t.Fatal("subscription did not correctly receive the genesis block")
	}
	if len(ms.updates) != 1 {
		t.Fatal("subscription resulted in the wrong number of blocks being sent")
	}

	// Create a copy of the subscriber that will subscribe to the consensus at
	// the tail of the updates.
	tailSubscriber := ms.copySub()
	cst.cs.ConsensusSetPersistentSubscribe(&tailSubscriber, tailSubscriber.updates[len(tailSubscriber.updates)-1].ID)
	if len(tailSubscriber.updates) != 1 {
		t.Fatal("subscription resulted in the wrong number of blocks being sent")
	}

	// Create a copy of the subscriber that will join when it is not at 0, but it is behind.
	behindSubscriber := ms.copySub()
	cst.addSiafunds()
	cst.mineSiacoins()
	cst.cs.ConsensusSetPersistentSubscribe(&behindSubscriber, behindSubscriber.updates[len(behindSubscriber.updates)-1].ID)
	if types.BlockHeight(len(behindSubscriber.updates)) != cst.cs.dbBlockHeight()+1 {
		t.Fatal("subscription resulted in the wrong number of blocks being sent")
	}
	if len(ms.updates) != len(tailSubscriber.updates) {
		t.Error("subscribers have inconsistent update chains")
	}
	if len(ms.updates) != len(behindSubscriber.updates) {
		t.Error("subscribers have inconsisitent update chains")
	}
}
