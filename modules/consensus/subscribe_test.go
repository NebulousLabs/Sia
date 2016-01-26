package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// mockSubscriber receives and holds changes to the consensus set, remembering
// the order in which changes were received.
type mockSubscriber struct {
	updates []modules.ConsensusChange
}

// newMockSubscriber returns a mockSubscriber that is ready to subscribe to a
// consensus set. Currently blank, but can be expanded to support more features
// in the future.
func newMockSubscriber() mockSubscriber {
	return mockSubscriber{}
}

// ProcessConsensusChange adds a consensus change to the mock subscriber.
func (ms *mockSubscriber) ProcessConsensusChange(cc modules.ConsensusChange) {
	ms.updates = append(ms.updates, cc)
}

// copySub creates and returns a new mock subscriber that has identical
// internals to the input mockSubscriber. The copy will not be subscribed to
// the consensus set even if the original is.
func (ms *mockSubscriber) copySub() (cms mockSubscriber) {
	cms.updates = make([]modules.ConsensusChange, len(ms.updates))
	copy(cms.updates, ms.updates)
	return cms
}

// TestUnitInvalidConsensusChangeSubscription checks that the consensus set
// returns modules.ErrInvalidConsensusChangeID in the event of a subscriber
// using an unrecognized id.
func TestUnitInvalidConsensusChangeSubscription(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	cst, err := createConsensusSetTester("TestUnitInvalidConsensusChangeSubscription")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	ms := newMockSubscriber()
	badCCID := modules.ConsensusChangeID{1}
	err = cst.cs.ConsensusSetPersistentSubscribe(&ms, badCCID)
	if err != modules.ErrInvalidConsensusChangeID {
		t.Error("consensus set returning the wrong error during an invalid subscription:", err)
	}
}

// TestUnitUnsubscribe checks that the consensus set correctly unsubscribes a
// subscriber if the Unsubscribe call is made.
func TestUnitUnsubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	cst, err := createConsensusSetTester("TestUnitInvalidConsensusChangeSubscription")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Subscribe the mock subscriber to the consensus set.
	ms := newMockSubscriber()
	err = cst.cs.ConsensusSetPersistentSubscribe(&ms, modules.ConsensusChangeID{})
	if err != nil {
		t.Fatal(err)
	}

	// Check that the subscriber is receiving updates.
	msLen := len(ms.updates)
	if msLen == 0 {
		t.Error("mock subscriber is not receiving updates")
	}
	_, err = cst.miner.AddBlock() // should cause another update to be sent to the subscriber
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.updates) != msLen+1 {
		t.Error("mock subscriber did not receive the correct number of updates")
	}

	// Unsubscribe the subscriber and then check that it is no longer receiving
	// updates.
	cst.cs.Unsubscribe(&ms)
	_, err = cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.updates) != msLen+1 {
		t.Error("mock subscriber was not correctly unsubscribed")
	}
}
