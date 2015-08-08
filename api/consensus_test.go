package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationConsensusGet probes the GET call to /consensus.
func TestIntegrationConsensusGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st := newServerTester("TestConsensusGET", t)
	var css ConsensusSetStatus
	err := st.getAPI("/consensus", &css)
	if err != nil {
		t.Fatal(err)
	}
	if css.Height != 4 {
		t.Error("wrong height returned in consensus GET call")
	}
	if css.CurrentBlock != st.server.currentBlock.ID() {
		t.Error("wrong block returned in consensus GET call")
	}
	expectedTarget := types.Target{64}
	if css.Target != expectedTarget {
		t.Error("wrong target returned in consensus GET call")
	}
}
