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

// TestInvalidConsensusChangeSubscription checks that the consensus set returns
// modules.ErrInvalidConsensusChangeID in the event of a subscriber using an
// unrecognized id.
func TestInvalidConsensusChangeSubscription(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	ms := newMockSubscriber()
	badCCID := modules.ConsensusChangeID{255, 255, 255}
	err = cst.cs.ConsensusSetSubscribe(&ms, badCCID, cst.cs.tg.StopChan())
	if err != modules.ErrInvalidConsensusChangeID {
		t.Error("consensus set returning the wrong error during an invalid subscription:", err)
	}

	cst.cs.mu.Lock()
	for i := range cst.cs.subscribers {
		if cst.cs.subscribers[i] == &ms {
			t.Fatal("subscriber was not removed from subscriber list after an erroneus subscription")
		}
	}
	cst.cs.mu.Unlock()
}

// TestInvalidToValidSubscription is a regression test. Previously, the
// consensus set would not unsubscribe a module if it returned an error during
// subscription. When the module resubscribed, the module would be
// double-subscribed to the consensus set.
func TestInvalidToValidSubscription(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Start by performing a bad subscribe.
	ms := newMockSubscriber()
	badCCID := modules.ConsensusChangeID{255, 255, 255}
	err = cst.cs.ConsensusSetSubscribe(&ms, badCCID, cst.cs.tg.StopChan())
	if err != modules.ErrInvalidConsensusChangeID {
		t.Error("consensus set returning the wrong error during an invalid subscription:", err)
	}

	// Perform a correct subscribe.
	err = cst.cs.ConsensusSetSubscribe(&ms, modules.ConsensusChangeBeginning, cst.cs.tg.StopChan())
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block and check that the mock subscriber only got a single
	// consensus change.
	numPrevUpdates := len(ms.updates)
	_, err = cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.updates) != numPrevUpdates+1 {
		t.Error("subscriber received two consensus changes for a single block")
	}
}

// TestUnsubscribe checks that the consensus set correctly unsubscribes a
// subscriber if the Unsubscribe call is made.
func TestUnsubscribe(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Subscribe the mock subscriber to the consensus set.
	ms := newMockSubscriber()
	err = cst.cs.ConsensusSetSubscribe(&ms, modules.ConsensusChangeBeginning, cst.cs.tg.StopChan())
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
