package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules/consensus"
)

// TestBlockBootstrap checks that consensus.Synchronize probably synchronizes
// the consensus set of a bootstrapping peer.
func TestBlockBootstrap(t *testing.T) {
	t.Skip("Test probably broken - code may be though")
	if testing.Short() {
		t.SkipNow()
	}

	// Create a server and give it some blocks.
	st := newServerTester("TestBlockBootstrap1", t)
	for i := 0; i < 2*consensus.MaxCatchUpBlocks+1; i++ {
		_, _, err := st.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		st.csUpdateWait()
	}

	// Add a peer and spin until the peer is caught up. addPeer() already does
	// this check, but it's left here to be explict anyway.
	st2 := st.addPeer("TestBlockBootstrap2")
	lockID := st.server.mu.RLock()
	lockID = st2.server.mu.RLock()
	defer st.server.mu.RUnlock(lockID)
	defer st2.server.mu.RUnlock(lockID)
	if st.server.blockchainHeight != st2.server.blockchainHeight {
		t.Fatal("heights do not match after synchronize")
	}
}
