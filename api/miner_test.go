package api

import (
	"testing"
)

// TestMinerGET checks the GET call to the /miner endpoint.
func (srv *Server) TestMinerGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestMinerGET")
	if err != nil {
		t.Fatal(err)
	}

	// TODO: Mine some stale blocks.

	// Get the api returned fields of the miner.
	var mg MinerGET
	err = st.getAPI("/miner", &mg)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the correctness of the results.
	blocksMined, staleBlocksMined := st.server.miner.BlocksMined()
	if mg.BlocksMined != blocksMined {
		t.Error("blocks mined did not succeed")
	}
	if mg.StaleBlocksMined != staleBlocksMined {
		t.Error("stale blocks mined is incorrect")
	}
	if mg.CPUHashrate != st.server.miner.CPUHashrate() {
		t.Error("mismatched cpu hashrate")
	}
	if mg.CPUMining != st.server.miner.CPUMining() {
		t.Error("mismatched cpu miner status")
	}
}
