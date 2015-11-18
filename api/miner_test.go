package api

import (
	"testing"
	"time"
)

// TestIntegrationMinerGET checks the GET call to the /miner endpoint.
func (srv *Server) TestIntegrationMinerGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationMinerGET")
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

// TestIntegrationMinerStartStop checks that the miner start and miner stop api endpoints
// toggle the cpu miner.
func (srv *Server) TestIntegrationMinerStartStop(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationMinerStartStop")
	if err != nil {
		t.Fatal(err)
	}

	// Start the cpu miner, give time for the first hashrate readings to
	// appear.
	err = st.stdGetAPI("/miner/start")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if st.server.miner.CPUHashrate() == 0 {
		t.Error("cpu miner is reporting no hashrate")
	}
	if !st.server.miner.CPUMining() {
		t.Error("cpu miner is reporting that it is not on")
	}

	// Check the numbers through the status api call.
	var mg MinerGET
	err = st.getAPI("/miner", &mg)
	if err != nil {
		t.Fatal(err)
	}
	if mg.CPUHashrate == 0 {
		t.Error("cpu hashrate is reported at zero")
	}
	if !mg.CPUMining {
		t.Error("cpu is not reporting through the api that it is mining.")
	}

	// Stop the cpu miner and wait for the stop call to go through.
	err = st.stdGetAPI("/miner/stop")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	if st.server.miner.CPUHashrate() != 0 {
		t.Error("cpu miner is reporting no hashrate")
	}
	if st.server.miner.CPUMining() {
		t.Error("cpu miner is reporting that it is not on")
	}

	// Check the numbers through the status api call.
	err = st.getAPI("/miner", &mg)
	if err != nil {
		t.Fatal(err)
	}
	if mg.CPUHashrate != 0 {
		t.Error("cpu hashrate is reported at zero")
	}
	if mg.CPUMining {
		t.Error("cpu is not reporting through the api that it is mining.")
	}
}
