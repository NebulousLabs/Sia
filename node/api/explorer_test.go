package api

import (
	"testing"

	"gitlab.com/NebulousLabs/Sia/types"
)

// TestIntegrationExplorerGET probes the GET call to /explorer.
func TestIntegrationExplorerGET(t *testing.T) {
	t.Skip("Explorer has deadlock issues")
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	var eg ExplorerGET
	err = st.getAPI("/explorer", &eg)
	if err != nil {
		t.Fatal(err)
	}
	if eg.Height != st.server.api.cs.Height() {
		t.Error("height not accurately reported by explorer")
	}
	if eg.MinerPayoutCount == 0 {
		t.Error("Miner payout count is incorrect")
	}
}

// TestIntegrationExplorerBlockGET probes the GET call to /explorer/block.
func TestIntegrationExplorerBlockGET(t *testing.T) {
	t.Skip("Explorer has deadlock issues")
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	var ebg ExplorerBlockGET
	err = st.getAPI("/explorer/blocks/0", &ebg)
	if err != nil {
		t.Fatal(err)
	}
	if ebg.Block.BlockID != ebg.Block.RawBlock.ID() {
		t.Error("block id and block do not match up from api call")
	}
	if ebg.Block.BlockID != types.GenesisBlock.ID() {
		t.Error("wrong block returned by /explorer/block?height=0")
	}
}

// TestIntegrationExplorerHashGet probes the GET call to /explorer/hash/:hash.
func TestIntegrationExplorerHashGet(t *testing.T) {
	t.Skip("Explorer has deadlock issues")
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
