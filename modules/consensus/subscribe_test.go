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
	defer cst.closeCst()

	ms := newMockSubscriber()
	badCCID := modules.ConsensusChangeID{1}
	err = cst.cs.ConsensusSetPersistentSubscribe(&ms, badCCID)
	if err != modules.ErrInvalidConsensusChangeID {
		t.Error("consensus set returning the wrong error during an invalid subscription:", err)
	}
}
