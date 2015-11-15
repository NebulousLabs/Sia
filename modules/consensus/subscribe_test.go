package consensus

import (
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
