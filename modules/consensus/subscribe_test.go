package consensus

import (
	"sync"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	bolt "github.com/coreos/bbolt"
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

// TestModuletDesync is a reproduction test for the bug that caused a module to
// desync while subscribing to the consensus set.
func TestModuleDesync(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	deps := &dependencySleepAfterInitializeSubscribe{}
	cst, err := blankConsensusSetTester(t.Name(), deps)
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Mine some blocks.
	for i := 0; i < 10; i++ {
		if _, err := cst.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}

	// Enable the dependency.
	ms := newMockSubscriber()
	deps.enable()

	// Subscribe to the consensusSet non-blocking.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err = cst.cs.ConsensusSetSubscribe(&ms, modules.ConsensusChangeBeginning, cst.cs.tg.StopChan())
		if err != nil {
			t.Error("consensus set returning the wrong error during an invalid subscription:", err)
		}
		wg.Done()
	}()

	// Mine some more blocks to make sure the module falls behind.
	for i := 0; i < 10; i++ {
		if _, err := cst.miner.AddBlock(); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for the module to be subscribed.
	wg.Wait()

	// Get all the updates from the consensusSet.
	updates := make([]modules.ConsensusChange, 0)
	cst.cs.mu.Lock()
	err = cst.cs.db.View(func(tx *bolt.Tx) error {
		entry := cst.cs.genesisEntry()
		exists := true
		for ; exists; entry, exists = entry.NextEntry(tx) {
			cc, err := cst.cs.computeConsensusChange(tx, entry)
			if err != nil {
				return err
			}
			updates = append(updates, cc)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cst.cs.mu.Unlock()

	// The received updates should match.
	if len(updates) != len(ms.updates) {
		t.Fatal("Number of updates doesn't match")
	}
	for i := 0; i < len(updates); i++ {
		if updates[i].ID != ms.updates[i].ID {
			t.Fatal("Update IDs don't match")
		}
	}

	// Make sure the last update is the recent one in the database.
	cst.cs.mu.Lock()
	recentChangeID, err := cst.cs.recentConsensusChangeID()
	cst.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if updates[len(updates)-1].ID != recentChangeID {
		t.Fatal("last update doesn't equal recentChangeID")
	}
}
