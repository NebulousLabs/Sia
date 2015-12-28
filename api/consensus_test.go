package api

import (
	"reflect"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationConsensusGet probes the GET call to /consensus.
func TestIntegrationConsensusGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestIntegrationConsensusGET")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	var cg ConsensusGET
	err = st.getAPI("/consensus", &cg)
	if err != nil {
		t.Fatal(err)
	}
	if cg.Height != 4 {
		t.Error("wrong height returned in consensus GET call")
	}
	if cg.CurrentBlock != st.server.cs.CurrentBlock().ID() {
		t.Error("wrong block returned in consensus GET call")
	}
	expectedTarget := types.Target{128}
	if cg.Target != expectedTarget {
		t.Error("wrong target returned in consensus GET call")
	}
}

// TestIntegrationConsensusBlockGET probes the GET call to /consensus/block.
func TestIntegrationConsensusBlockGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestIntegrationConsensusBlockGET")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	var cbg ConsensusBlockGET
	err = st.getAPI("/consensus/block?height=1", &cbg)
	if err != nil {
		t.Fatal(err)
	}
	block1, exists := st.cs.BlockAtHeight(1)
	if !exists {
		t.Fatal("unexpected dne")
	}
	if !reflect.DeepEqual(block1, cbg.Block) {
		t.Fatal("blocks do not match")
	}

	// Sanity check - BlockAtHeight should be working.
	gb := st.cs.GenesisBlock()
	if block1.ParentID != gb.ID() {
		t.Fatal("genesis block and block1 mismatch")
	}
}
