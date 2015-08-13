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

	st, err := createServerTester("TestConsensusGET")
	if err != nil {
		t.Fatal(err)
	}
	var cg ConsensusGET
	err = st.getAPI("/consensus", &cg)
	if err != nil {
		t.Fatal(err)
	}
	if cg.Height != 4 {
		t.Error("wrong height returned in consensus GET call")
	}
	if cg.CurrentBlock != st.server.currentBlock.ID() {
		t.Error("wrong block returned in consensus GET call")
	}
	expectedTarget := types.Target{64}
	if cg.Target != expectedTarget {
		t.Error("wrong target returned in consensus GET call")
	}
}
