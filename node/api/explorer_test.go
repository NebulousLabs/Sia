package api

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationExplorerGET probes the GET call to /explorer.
func TestIntegrationExplorerGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	var egBlocks ExplorerBlockGET
	err = st.getAPI("/explorer", &egBlocks)
	if err != nil {
		t.Fatal(err)
	}

	if len(egBlocks.Blocks) != 1 {
		t.Errorf("wrong block len for ExplorerBlockGET: %d", len(egBlocks.Blocks))
	}
	eg := egBlocks.Blocks[0]
	if eg.Height != st.server.api.cs.Height() {
		t.Error("height not accurately reported by explorer")
	}
	if eg.MinerPayoutCount == 0 {
		t.Error("Miner payout count is incorrect")
	}
}

// TestIntegrationExplorerBlockGET probes the GET call to /explorer/block.
func TestIntegrationExplorerBlockGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	var egBlocks ExplorerBlockGET
	err = st.getAPI("/explorer/blocks/0", &egBlocks)
	if err != nil {
		t.Fatal(err)
	}

	if len(egBlocks.Blocks) != 1 {
		t.Errorf("wrong block len for ExplorerBlockGET: %d", len(egBlocks.Blocks))
	}
	ebg := egBlocks.Blocks[0]
	if ebg.BlockID != ebg.RawBlock.ID() {
		t.Error("block id and block do not match up from api call")
	}
	if ebg.BlockID != types.GenesisBlock.ID() {
		t.Errorf("wrong block returned by /explorer/block?height=0.  Got: %s.  Expected: %s", ebg.BlockID, types.GenesisBlock.ID())
	}
}

// TestIntegrationExplorerHashGet probes the GET call to /explorer/hash/:hash.
func TestIntegrationExplorerHashGet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	var ehg ExplorerHashGET
	gb := types.GenesisBlock
	err = st.getAPI("/explorer/hashes/"+gb.ID().String(), &ehg)
	if err != nil {
		t.Fatal(err)
	}
	if ehg.HashType != "blockid" {
		t.Error("wrong hash type returned when requesting block hash")
	}
	if ehg.Block.BlockID != gb.ID() {
		t.Error("wrong block type returned")
	}
}
