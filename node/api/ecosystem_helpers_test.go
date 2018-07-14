package api

// ecosystem_helpers_test.go has a bunch of helper functions to make setting up
// large ecosystem tests easier.
//
// List of helper functions:
//    addStorageToAllHosts // adds a storage folder to every host
//    announceAllHosts     // announce all hosts to the network (and mine a block)
//    fullyConnectNodes    // connects each server tester to all the others
//    fundAllNodes         // mines blocks until all server testers have money
//    synchronizationCheck // checks that all server testers have the same recent block
//    waitForBlock         // block until the provided block is the most recent block for all server testers

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/NebulousLabs/Sia/build"
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

	// Grab the initial transaction pool size to know how many total transactions
	// there should be after announcement.
	initialTpoolSize := len(sts[0].tpool.TransactionList())

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
		if len(sts[0].tpool.TransactionList()) == len(sts)*2+initialTpoolSize {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if len(sts[0].tpool.TransactionList()) < len(sts)*2+initialTpoolSize {
		return fmt.Errorf("Host announcements do not seem to have propagated to the leader's tpool: %v, %v, %v", len(sts), len(sts[0].tpool.TransactionList())+initialTpoolSize, initialTpoolSize)
	}

	// Mine a block and then wait for all of the nodes to syncrhonize to it.
	_, err = sts[0].miner.AddBlock()
	if err != nil {
		return err
	}
	// Block until the block propagated to all nodes
	for _, st := range sts[1:] {
		err = waitForBlock(sts[0].cs.CurrentBlock().ID(), st)
		if err != nil {
			return (err)
		}
	}
	// Check if all nodes are on the same block now
	_, err = synchronizationCheck(sts)
	if err != nil {
		return err
	}

	// Block until every node has completed the scan of every other node, so
	// that each node has a full hostdb.
	for _, st := range sts {
		err := build.Retry(600, 100*time.Millisecond, func() error {
			var ah HostdbActiveGET
			err = st.getAPI("/hostdb/active", &ah)
			if err != nil {
				return err
			}
			if len(ah.Hosts) < len(sts) {
				return errors.New("one of the nodes hostdbs was unable to find at least one host announcement")
			}
			return nil
		})
		if err != nil {
			return err
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
			// Try connecting to the other node until both have the other in
			// their peer list.
			err = build.Retry(100, time.Millisecond*100, func() error {
				// NOTE: this check depends on string-matching an error in the
				// gateway. If that error changes at all, this string will need to
				// be updated.
				err := stb.stdPostAPI("/gateway/connect/"+string(gg.NetAddress), nil)
				if err != nil && err.Error() != "already connected to this peer" {
					return err
				}

				// Check that the gateways are connected.
				bToA := false
				aToB := false
				var ggb GatewayGET
				err = stb.getAPI("/gateway", &ggb)
				if err != nil {
					return err
				}
				for _, peer := range ggb.Peers {
					if peer.NetAddress == gg.NetAddress {
						bToA = true
						break
					}
				}
				err = sta.getAPI("/gateway", &gg)
				if err != nil {
					return err
				}
				for _, peer := range gg.Peers {
					if peer.NetAddress == ggb.NetAddress {
						aToB = true
						break
					}
				}
				if !aToB || !bToA {
					return fmt.Errorf("called connect between two nodes, but they are not peers: %v %v %v %v %v %v", aToB, bToA, gg.NetAddress, ggb.NetAddress, gg.Peers, ggb.Peers)
				}
				return nil

			})
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

	// Wait until all nodes are on the same block
	for _, st := range sts[1:] {
		err := waitForBlock(sts[0].cs.CurrentBlock().ID(), st)
		if err != nil {
			return types.BlockID{}, err
		}
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
