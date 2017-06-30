package api

// ecosystem_test.go provides tests for whole-ecosystem testing, consisting of
// multiple full, non-state-sharing nodes connected in various arrangements and
// performing various full-ecosystem tasks.
//
// To the absolute greatest extent possible, nodes are queried and updated
// exclusively through the API.

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/types"
)

// TestHostPoorConnectivity creates several full server testers and links them
// together in a way that might mimic a full host ecosystem with a renter, and
// then isolates one of the hosts from the network, denying the host proper
// transaction propagation. The renters performed chained contract forming and
// uploading in the same manner that might happen in the wild, and then the
// host must get a file contract to the blockchain despite not getting any of
// the dependencies into the transaction pool from the flood network.
func TestHostPoorConnectivity(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create the various nodes that will be forming the simulated ecosystem of
	// this test.
	stLeader, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer stLeader.panicClose()
	stHost1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost1.panicClose()
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost2.panicClose()
	stHost3, err := blankServerTester(t.Name() + " - Host 3")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost3.panicClose()
	stHost4, err := blankServerTester(t.Name() + " - Host 4")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost4.panicClose()
	stRenter1, err := blankServerTester(t.Name() + " - Renter 1")
	if err != nil {
		t.Fatal(err)
	}
	defer stRenter1.panicClose()
	stRenter2, err := blankServerTester(t.Name() + " - Renter 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stRenter2.panicClose()

	// Fetch all of the addresses of the nodes that got created.
	var ggSTL, ggSTH1, ggSTH2, ggSTH3, ggSTH4, ggSTR1, ggSTR2 GatewayGET
	err = stLeader.getAPI("/gateway", &ggSTL)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost1.getAPI("/gateway", &ggSTH1)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost2.getAPI("/gateway", &ggSTH2)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost3.getAPI("/gateway", &ggSTH3)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost4.getAPI("/gateway", &ggSTH4)
	if err != nil {
		t.Fatal(err)
	}
	err = stRenter1.getAPI("/gateway", &ggSTR1)
	if err != nil {
		t.Fatal(err)
	}
	err = stRenter2.getAPI("/gateway", &ggSTR2)
	if err != nil {
		t.Fatal(err)
	}

	// Connect all of the peers in a circle, so that everyone is connected but
	// there are a lot of hops.
	err = stLeader.stdPostAPI("/gateway/connect/"+string(ggSTH1.NetAddress), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost1.stdPostAPI("/gateway/connect/"+string(ggSTH2.NetAddress), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost2.stdPostAPI("/gateway/connect/"+string(ggSTH3.NetAddress), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost3.stdPostAPI("/gateway/connect/"+string(ggSTH4.NetAddress), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost4.stdPostAPI("/gateway/connect/"+string(ggSTR1.NetAddress), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = stRenter1.stdPostAPI("/gateway/connect/"+string(ggSTR2.NetAddress), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = stRenter2.stdPostAPI("/gateway/connect/"+string(ggSTL.NetAddress), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Connectivity check - all nodes should be synchronized to the leader's
	// chain, which should have been the longest.
	allTesters := []*serverTester{stLeader, stHost1, stHost2, stHost3, stHost4, stRenter1, stRenter2}
	chainTip, err := synchronizationCheck(allTesters)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block from each node, to give the node money in the wallet that
	// is recognized by the shared chain.
	for i := range allTesters {
		// Wait until the current tester has 'chainTip' as its current
		// block, to make sure the network is building a community chain
		// instead of creating orphans.
		var cg ConsensusGET
		success := false
		for j := 0; j < 100; j++ {
			err = allTesters[i].getAPI("/consensus", &cg)
			if err != nil {
				t.Fatal(err)
			}
			if cg.CurrentBlock == chainTip {
				success = true
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		if !success {
			t.Fatal("nodes do not seem to be synchronizing")
		}
		err := allTesters[i].cs.Flush()
		if err != nil {
			t.Fatal(err)
		}

		// Mine a block for this node. The next iteration will wait for
		// synchronization before mining the block for the next node.
		block, err := allTesters[i].miner.AddBlock()
		if err != nil {
			t.Fatal(err, i)
		}
		chainTip = block.ID()
	}

	// Wait until the leader has the most recent block.
	var cg ConsensusGET
	success := false
	for i := 0; i < 100; i++ {
		err = allTesters[0].getAPI("/consensus", &cg)
		if err != nil {
			t.Fatal(err)
		}
		if cg.CurrentBlock == chainTip {
			success = true
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if !success {
		t.Fatal("nodes do not seem to be synchronizing")
	}

	// Make sure that everyone has the most recent block.
	_, err = synchronizationCheck(allTesters)
	if err != nil {
		t.Fatal(err)
	}

	// Mine blocks from the leader until everyone's miner payouts have matured
	// and become spendable.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := stLeader.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = synchronizationCheck(allTesters)
	if err != nil {
		t.Fatal(err)
	}
}
