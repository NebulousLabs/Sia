package api

// ecosystem_test.go provides tooling and tests for whole-ecosystem testing,
// consisting of multiple full, non-state-sharing nodes connected in various
// arrangements and performing various full-ecosystem tasks.
//
// To the absolute greatest extent possible, nodes are queried and updated
// exclusively through the API.

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/types"
)

// addStorageToAllHosts adds a storage folder with a bunch of storage to each
// host.
func addStorageToAllHosts(sts []*serverTester) error {
	for _, st := range sts {
		values := url.Values{}
		values.Set("path", st.dir)
		values.Set("size", "1048576")
		err := st.stdPostAPI("/host/storage/folders/add", values)
		if err != nil {
			return err
		}
	}
	return nil
}

// announceAllHosts will announce every host in the tester set to the
// blockchain.
func announceAllHosts(sts []*serverTester) error {
	// Check that all announcements will be on the same chain.
	_, err := synchronizationCheck(sts)
	if err != nil {
		return err
	}

	// Announce each host.
	for _, st := range sts {
		// Set the host to be accepting contracts.
		acceptingContractsValues := url.Values{}
		acceptingContractsValues.Set("acceptingcontracts", "true")
		err = st.stdPostAPI("/host", acceptingContractsValues)
		if err != nil {
			return err
		}

		// Fetch the host net address.
		var hg HostGET
		err = st.getAPI("/host", &hg)
		if err != nil {
			return err
		}

		// Make the announcement.
		announceValues := url.Values{}
		announceValues.Set("address", string(hg.ExternalSettings.NetAddress))
		err = st.stdPostAPI("/host/announce", announceValues)
		if err != nil {
			return err
		}
	}

	// Wait until all of the transactions have propagated to all of the nodes.
	//
	// TODO: Replace this direct transaction pool call with a call to the
	// /transactionpool endpoint.
	//
	// TODO: At some point the number of transactions needed to make an
	// announcement may change. Currently its 2.
	for i := 0; i < 50; i++ {
		if len(sts[0].tpool.TransactionList()) == len(sts)*2 {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if len(sts[0].tpool.TransactionList()) != len(sts)*2 {
		return errors.New("Host announcements do not seem to have propagated to the leader's tpool")
	}

	// Mine a block and then wait for all of the nodes to syncrhonize to it.
	_, err = sts[0].miner.AddBlock()
	if err != nil {
		return err
	}
	_, err = synchronizationCheck(sts)
	if err != nil {
		return err
	}

	// Block until every node has completed the scan of every other node, so
	// that each node has a full hostdb.
	for _, st := range sts {
		var ah HostdbActiveGET
		for i := 0; i < 50; i++ {
			err = st.getAPI("/hostdb/active", &ah)
			if err != nil {
				return err
			}
			if len(ah.Hosts) >= len(sts) {
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		if len(ah.Hosts) < len(sts) {
			return errors.New("one of the nodes hostdbs was unable to find at least one host announcement")
		}
	}
	return nil
}

// fullyConnectNodes takes a bunch of tester nodes and connects each to the
// other, creating a fully connected graph so that everyone is on the same
// chain.
//
// After connecting the nodes, it verifies that all the nodes have
// synchronized.
func fullyConnectNodes(sts []*serverTester) error {
	for i, sta := range sts {
		var gg GatewayGET
		err := sta.getAPI("/gateway", &gg)
		if err != nil {
			return err
		}

		// Connect this node to every other node.
		for _, stb := range sts[i+1:] {
			err := stb.stdPostAPI("/gateway/connect/"+string(gg.NetAddress), nil)
			if err != nil {
				return err
			}
		}
	}

	// Perform a synchronization check.
	_, err := synchronizationCheck(sts)
	return err
}

// fundAllNodes will make sure that each node has mined a block in the longest
// chain, then will mine enough blocks that the miner payouts manifest in the
// wallets of each node.
func fundAllNodes(sts []*serverTester) error {
	// Check that all of the nodes are synchronized.
	chainTip, err := synchronizationCheck(sts)
	if err != nil {
		return err
	}

	// Mine a block for each node to fund their wallet.
	for i := range sts {
		err := waitForBlock(chainTip, sts[i])
		if err != nil {
			return err
		}

		// Mine a block. The next iteration of this loop will ensure that the
		// block propagates and does not get orphaned.
		block, err := sts[i].miner.AddBlock()
		if err != nil {
			return err
		}
		chainTip = block.ID()
	}

	// Wait until the chain tip has propagated to the first node.
	err = waitForBlock(chainTip, sts[0])
	if err != nil {
		return err
	}

	// Mine types.MaturityDelay more blocks from the final node to mine a
	// block, to guarantee that all nodes have had their payouts mature, such
	// that their wallets can begin spending immediately.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err := sts[0].miner.AddBlock()
		if err != nil {
			return err
		}
	}

	// Block until every node has the full chain.
	_, err = synchronizationCheck(sts)
	return err
}

// synchronizationCheck takes a bunch of server testers as input and checks
// that they all have the same current block as the first server tester. The
// first server tester needs to have the most recent block in order for the
// check to work.
func synchronizationCheck(sts []*serverTester) (types.BlockID, error) {
	// Prefer returning an error in the event of a zero-length server tester -
	// an error should be returned if the developer accidentally uses a nil
	// slice instead of whatever value was intended, and there's no reason to
	// check for synchronization if there aren't any nodes to be synchronized.
	if len(sts) == 0 {
		return types.BlockID{}, errors.New("no server testers provided")
	}

	var cg ConsensusGET
	err := sts[0].getAPI("/consensus", &cg)
	if err != nil {
		return types.BlockID{}, err
	}
	leaderBlockID := cg.CurrentBlock
	for i := range sts {
		// Spin until the current block matches the leader block.
		success := false
		for j := 0; j < 100; j++ {
			err = sts[i].getAPI("/consensus", &cg)
			if err != nil {
				return types.BlockID{}, err
			}
			if cg.CurrentBlock == leaderBlockID {
				success = true
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		if !success {
			return types.BlockID{}, errors.New("synchronization check failed - nodes do not seem to be synchronized")
		}
	}
	return leaderBlockID, nil
}

// waitForBlock will block until the provided chain tip is the most recent
// block in the provided testing node.
func waitForBlock(chainTip types.BlockID, st *serverTester) error {
	var cg ConsensusGET
	success := false
	for j := 0; j < 100; j++ {
		err := st.getAPI("/consensus", &cg)
		if err != nil {
			return err
		}
		if cg.CurrentBlock == chainTip {
			success = true
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if !success {
		return errors.New("node never reached the correct chain tip")
	}
	return nil
}

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
	t.Parallel()

	// Create the various nodes that will be forming the simulated ecosystem of
	// this test.
	stLeader, err := createServerTester("TestHostPoorConnectivity - Leader")
	if err != nil {
		t.Fatal(err)
	}
	stHost1, err := blankServerTester("TestHostPoorConnectivity - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	stHost2, err := blankServerTester("TestHostPoorConnectivity - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	stHost3, err := blankServerTester("TestHostPoorConnectivity - Host 3")
	if err != nil {
		t.Fatal(err)
	}
	stHost4, err := blankServerTester("TestHostPoorConnectivity - Host 4")
	if err != nil {
		t.Fatal(err)
	}
	stRenter1, err := blankServerTester("TestHostPoorConnectivity - Renter 1")
	if err != nil {
		t.Fatal(err)
	}
	stRenter2, err := blankServerTester("TestHostPoorConnectivity - Renter 2")
	if err != nil {
		t.Fatal(err)
	}

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
