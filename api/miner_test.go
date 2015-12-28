package api

import (
	"io/ioutil"
	"net/url"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationMinerGET checks the GET call to the /miner endpoint.
func TestIntegrationMinerGET(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationMinerGET")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
func TestIntegrationMinerStartStop(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationMinerStartStop")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Start the cpu miner, give time for the first hashrate readings to
	// appear.
	err = st.stdPostAPI("/miner/start", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if !st.server.miner.CPUMining() {
		t.Error("cpu miner is reporting that it is not on")
	}

	// Check the numbers through the status api call.
	var mg MinerGET
	err = st.getAPI("/miner", &mg)
	if err != nil {
		t.Fatal(err)
	}
	if !mg.CPUMining {
		t.Error("cpu is not reporting through the api that it is mining.")
	}

	// Stop the cpu miner and wait for the stop call to go through.
	err = st.stdPostAPI("/miner/stop", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if st.server.miner.CPUMining() {
		t.Error("cpu miner is reporting that it is not on")
	}

	// Check the numbers through the status api call.
	err = st.getAPI("/miner", &mg)
	if err != nil {
		t.Fatal(err)
	}
	if mg.CPUMining {
		t.Error("cpu is not reporting through the api that it is mining.")
	}
}

// TestIntegrationMinerHeader checks that the header GET and POST calls are
// useful tools for mining blocks.
func TestIntegrationMinerHeader(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationMinerHeader")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	startingHeight := st.cs.Height()

	// Get a header that can be used for mining.
	resp, err := HttpGET("http://" + st.server.listener.Addr().String() + "/miner/header")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	targetAndHeader, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Twiddle the header bits until a block has been found.
	//
	// Note: this test treats the target as hardcoded, if the testing target is
	// changed, this test will also need to be changed.
	if types.RootTarget[0] != 128 {
		t.Fatal("test will fail because the testing constants have been unexpectedly changed")
	}
	var header [80]byte
	copy(header[:], targetAndHeader[32:])
	headerHash := crypto.HashObject(header)
	for headerHash[0] >= types.RootTarget[0] {
		header[35]++
		headerHash = crypto.HashObject(header)
	}

	// Submit the solved header through the api and check that the height of
	// the blockchain increases.
	resp, err = HttpPOST("http://"+st.server.listener.Addr().String()+"/miner/header", string(header[:]))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	time.Sleep(500 * time.Millisecond)
	if st.cs.Height() != startingHeight+1 {
		t.Errorf("block height did not increase after trying to mine a block through the api, started at %v and ended at %v", startingHeight, st.cs.Height())
	}
}
