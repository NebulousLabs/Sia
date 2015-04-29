package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules/consensus"
)

// TestBlockBootstrap checks that consensus.Synchronize probably synchronizes
// the consensus set of a bootstrapping peer.
func TestBlockBootstrap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a server and give it 2500 blocks.
	st := newServerTester("TestBlockBootstrap1", t)
	for i := 0; i < 2*consensus.MaxCatchUpBlocks+1; i++ {
		st.mineBlock()
		st.csUpdateWait()
	}

	// Add a peer and spin until the peer is caught up. addPeer() already does
	// this check, but it's left here to be explict anyway.
	st2 := st.addPeer("TestBlockBootstrap2")
	if st.server.cs.Height() != st2.server.cs.Height() {
		t.Fatal("heights do not match after synchronize")
	}
}
