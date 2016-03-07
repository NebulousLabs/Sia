package consensus

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// TestSynchronize tests that the consensus set can successfully synchronize
// to a peer.
func TestSynchronize(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst1, err := createConsensusSetTester("TestSynchronize1")
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := createConsensusSetTester("TestSynchronize2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.Close()

	// mine on cst2 until it is above cst1
	for cst1.cs.dbBlockHeight() >= cst2.cs.dbBlockHeight() {
		b, _ := cst2.miner.FindBlock()
		err = cst2.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}

	// connect gateways, triggering a Synchronize
	err = cst1.gateway.Connect(cst2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}

	// blockchains should now match
	for cst1.cs.dbCurrentBlockID() != cst2.cs.dbCurrentBlockID() {
		time.Sleep(10 * time.Millisecond)
	}

	// Mine on cst2 until it is more than 'MaxCatchUpBlocks' ahead of cst2.
	// NOTE: we have to disconnect prior to this, otherwise cst2 will relay
	// blocks to cst1.
	err = cst1.gateway.Disconnect(cst2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	// TODO: more than 30 causes a race condition!
	for cst2.cs.dbBlockHeight() < cst1.cs.dbBlockHeight()+20 {
		b, _ := cst2.miner.FindBlock()
		err = cst2.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}
	// reconnect
	err = cst1.gateway.Connect(cst2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}

	// block heights should now match
	for cst1.cs.dbBlockHeight() != cst2.cs.dbBlockHeight() {
		time.Sleep(250 * time.Millisecond)
	}

	/*
		// extend cst2 with a "bad" (old) block, and synchronize. cst1 should
		// reject the bad block.
		lockID := cst2.cs.mu.Lock()
		cst2.cs.db.pushPath(cst2.cs.db.getPath(0))
		cst2.cs.mu.Unlock(lockID)
		if cst1.cs.db.pathHeight() == cst2.cs.db.pathHeight() {
			t.Fatal("cst1 did not reject bad block")
		}
	*/
}

func TestResynchronize(t *testing.T) {
	t.Skip("takes way too long")

	cst1, err := createConsensusSetTester("TestResynchronize1")
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := createConsensusSetTester("TestResynchronize2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.Close()

	// TODO: without this extra block, sync fails. Why?
	b, _ := cst2.miner.FindBlock()
	err = cst2.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// connect and disconnect, so that cst1 and cst2 are synchronized
	err = cst1.gateway.Connect(cst2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	err = cst1.gateway.Disconnect(cst2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}

	if cst1.cs.dbCurrentBlockID() != cst2.cs.dbCurrentBlockID() {
		t.Fatal("Consensus Sets did not synchronize")
	}

	// mine a block on cst2, but hide it from cst1 during reconnect
	/*
		b, _ = cst2.miner.FindBlock()
		err = cst2.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
		lockID := cst2.cs.mu.Lock()
		id := cst2.cs.currentBlockID()
		err = cst2.cs.db.popPath()
		if err != nil {
			t.Fatal(err)
		}
		cst2.cs.mu.Unlock(lockID)

		err = cst1.gateway.Connect(cst2.gateway.Address())
		if err != nil {
			t.Fatal(err)
		}

		// add id back to cst2's current path
		lockID = cst2.cs.mu.Lock()
		err = cst2.cs.db.pushPath(id)
		if err != nil {
			t.Fatal(err)
		}
		cst2.cs.mu.Unlock(lockID)

		// cst1 should not have the block
		if cst1.cs.dbBlockHeight() == cst2.cs.dbBlockHeight() {
			t.Fatal("Consensus Sets should not have the same height")
		}
	*/
}

// TestBlockHistory tests that blockHistory returns the expected sequence of
// block IDs.
func TestBlockHistory(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst, err := createConsensusSetTester("TestBlockHistory")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// mine until we have enough blocks to test blockHistory
	for cst.cs.dbBlockHeight() < 50 {
		b, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}

	var history [32]types.BlockID
	_ = cst.cs.db.View(func(tx *bolt.Tx) error {
		history = blockHistory(tx)
		return nil
	})

	// validate history
	cst.cs.mu.Lock()
	// first 10 IDs are linear
	for i := types.BlockHeight(0); i < 10; i++ {
		id, err := cst.cs.dbGetPath(cst.cs.dbBlockHeight() - i)
		if err != nil {
			t.Fatal(err)
		}
		if history[i] != id {
			t.Errorf("Wrong ID in history: expected %v, got %v", id, history[i])
		}
	}
	// next 4 IDs are exponential
	heights := []types.BlockHeight{11, 15, 23, 39}
	for i, height := range heights {
		id, err := cst.cs.dbGetPath(cst.cs.dbBlockHeight() - height)
		if err != nil {
			t.Fatal(err)
		}
		if history[10+i] != id {
			t.Errorf("Wrong ID in history: expected %v, got %v", height, history[10+i])
		}
	}
	// finally, the genesis ID
	genesisID, err := cst.cs.dbGetPath(0)
	if err != nil {
		t.Fatal(err)
	}
	if history[31] != genesisID {
		t.Errorf("Wrong ID in history: expected %v, got %v", genesisID, history[31])
	}

	cst.cs.mu.Unlock()

	// remaining IDs should be empty
	var emptyID types.BlockID
	for i, id := range history[14:31] {
		if id != emptyID {
			t.Errorf("Expected empty ID at index %v, got %v", i+17, id)
		}
	}
}

// mockGatewayCountBroadcasts implements modules.Gateway to mock the Broadcast
// method.
type mockGatewayCountBroadcasts struct {
	modules.Gateway
	numBroadcasts int
	mu            sync.RWMutex
}

// Broadcast is a mock implementation of modules.Gateway.Broadcast that
// increments a counter denoting the number of times it's been called.
func (g *mockGatewayCountBroadcasts) Broadcast(name string, obj interface{}, peers []modules.Peer) {
	g.mu.Lock()
	g.numBroadcasts++
	g.mu.Unlock()
	g.Gateway.Broadcast(name, obj, peers)
}

// TestSendBlocksBroadcastsOnce tests that the SendBlocks RPC call only
// Broadcasts one block, no matter how many blocks are sent. In the case 0
// blocks are sent, tests that Broadcast is never called.
func TestSendBlocksBroadcastsOnce(t *testing.T) {
	// Setup consensus sets.
	cst1, err := blankConsensusSetTester("TestSendBlocksBroadcastsOnce1")
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := blankConsensusSetTester("TestSendBlocksBroadcastsOnce2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.Close()
	// Setup mock gateway.
	mg := mockGatewayCountBroadcasts{Gateway: cst1.cs.gateway}
	cst1.cs.gateway = &mg
	err = cst1.cs.gateway.Connect(cst2.cs.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		blocksToMine          int
		expectedNumBroadcasts int
	}{
		{0, 0},
		{1, 1},
		{2, 1},
		{int(MaxCatchUpBlocks), 1},
		{2 * int(MaxCatchUpBlocks), 1},
		{2*int(MaxCatchUpBlocks) + 1, 1},
	}
	for _, test := range tests {
		mg.mu.Lock()
		mg.numBroadcasts = 0
		mg.mu.Unlock()
		for i := 0; i < test.blocksToMine; i++ {
			b, minerErr := cst2.miner.FindBlock()
			if minerErr != nil {
				t.Fatal(minerErr)
			}
			// managedAcceptBlock is used here instead of AcceptBlock so as not to
			// call Broadcast outside of the SendBlocks RPC.
			err = cst2.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}
		err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "SendBlocks", cst1.cs.threadedReceiveBlocks)
		if err != nil {
			t.Fatal(err)
		}
		// Sleep to wait for possible calls to Broadcast to complete. We cannot
		// wait on a channel because we don't know how many times broadcast has
		// been called.
		time.Sleep(10 * time.Millisecond)
		mg.mu.RLock()
		numBroadcasts := mg.numBroadcasts
		mg.mu.RUnlock()
		if numBroadcasts != test.expectedNumBroadcasts {
			t.Errorf("expected %d number of broadcasts, got %d", test.expectedNumBroadcasts, numBroadcasts)
		}
	}
}

// TestIntegrationRPCSendBlocks tests that the SendBlocks RPC adds blocks to
// the consensus set, and that the consensus set catches with the remote peer
// and possibly reorgs.
func TestIntegrationRPCSendBlocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	type sendBlocksTest struct {
		commonBlocksToMine types.BlockHeight
		localBlocksToMine  types.BlockHeight
		remoteBlocksToMine types.BlockHeight
		msg                string
	}
	tests := []sendBlocksTest{
		{
			msg: "SendBlocks shouldn't do anything when both CSs are at the genesis block",
		},
		{
			commonBlocksToMine: 10,
			msg:                "SendBlocks shouldn't do anything when both CSs are at the same block",
		},
		{
			commonBlocksToMine: 10,
			localBlocksToMine:  5,
			msg:                "SendBlocks shouldn't do anything when the remote CS is behind the local CS",
		},
		{
			commonBlocksToMine: 10,
			remoteBlocksToMine: 5,
			msg:                "SendBlocks should catch up the local CS to the remote CS when it is behind",
		},
		{
			remoteBlocksToMine: 10,
			localBlocksToMine:  5,
			msg:                "SendBlocks should reorg the local CS when the remote CS's chain is longer",
		},
		{
			commonBlocksToMine: 10,
			remoteBlocksToMine: 10,
			localBlocksToMine:  5,
			msg:                "SendBlocks should reorg the local CS when the remote CS's chain is longer",
		},
		{
			remoteBlocksToMine: MaxCatchUpBlocks - 1,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: MaxCatchUpBlocks - 1,
			localBlocksToMine:  MaxCatchUpBlocks - 2,
			msg:                "SendBlocks should reorg the local CS when the remote CS's chain is longer",
		},
		{
			remoteBlocksToMine: MaxCatchUpBlocks,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: MaxCatchUpBlocks,
			localBlocksToMine:  MaxCatchUpBlocks - 2,
			msg:                "SendBlocks should reorg the local CS when the remote CS's chain is longer",
		},
		{
			remoteBlocksToMine: MaxCatchUpBlocks + 1, // There was a bug that caused SendBlocks to be one block behind when its peer was ahead by (k * MaxCatchUpBlocks) + 1 blocks.
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: MaxCatchUpBlocks + 1, // There was a bug that caused SendBlocks to be one block behind when its peer was ahead by (k * MaxCatchUpBlocks) + 1 blocks.
			localBlocksToMine:  MaxCatchUpBlocks - 2,
			msg:                "SendBlocks should reorg the local CS when the remote CS's chain is longer",
		},
		{
			remoteBlocksToMine: 2*MaxCatchUpBlocks + 1,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 2*MaxCatchUpBlocks + 1,
			localBlocksToMine:  2*MaxCatchUpBlocks - 2,
			msg:                "SendBlocks should reorg the local CS when the remote CS's chain is longer",
		},
		{
			remoteBlocksToMine: 12,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 15,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 16,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 17,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 23,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 31,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 32,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
		{
			remoteBlocksToMine: 33,
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		},
	}
	for i := 1; i < 10; i++ {
		tests = append(tests, sendBlocksTest{
			remoteBlocksToMine: types.BlockHeight(i),
			msg:                "SendBlocks should catch up when the remote CS is ahead",
		})
	}

	for i, tt := range tests {
		// Create the "remote" peer.
		remoteCST, err := blankConsensusSetTester(filepath.Join("TestRPCSendBlocks - remote", strconv.Itoa(i)))
		if err != nil {
			t.Fatal(err)
		}
		// Create the "local" peer.
		localCST, err := blankConsensusSetTester(filepath.Join("TestRPCSendBlocks - local", strconv.Itoa(i)))
		if err != nil {
			t.Fatal(err)
		}

		localCST.cs.gateway.Connect(remoteCST.cs.gateway.Address())
		// Wait a second to let the OnConnectRPCs finish
		time.Sleep(100 * time.Millisecond)

		// Mine blocks.
		for i := types.BlockHeight(0); i < tt.commonBlocksToMine; i++ {
			b, err := remoteCST.miner.FindBlock()
			if err != nil {
				t.Fatal(err)
			}
			err = remoteCST.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
			err = localCST.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}
		for i := types.BlockHeight(0); i < tt.remoteBlocksToMine; i++ {
			b, err := remoteCST.miner.FindBlock()
			if err != nil {
				t.Fatal(err)
			}
			err = remoteCST.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}
		for i := types.BlockHeight(0); i < tt.localBlocksToMine; i++ {
			b, err := localCST.miner.FindBlock()
			if err != nil {
				t.Fatal(err)
			}
			err = localCST.cs.managedAcceptBlock(b)
			if err != nil {
				t.Fatal(err)
			}
		}

		localCurrentBlockID := localCST.cs.CurrentBlock().ID()
		remoteCurrentBlockID := remoteCST.cs.CurrentBlock().ID()

		err = localCST.cs.gateway.RPC(remoteCST.cs.gateway.Address(), "SendBlocks", localCST.cs.threadedReceiveBlocks)
		if err != nil {
			t.Error(err)
		}

		// Assume that if remoteBlocksToMine is greater than localBlocksToMine, then
		// the local CS must have received the new blocks (and reorged).
		if tt.remoteBlocksToMine > tt.localBlocksToMine {
			// Verify that the remote cs did not change.
			if remoteCST.cs.CurrentBlock().ID() != remoteCurrentBlockID {
				t.Errorf("%v: the remote CS is at a different current block than before SendBlocks", tt.msg)
			}
			// Verify that the local cs got the new blocks.
			if localCST.cs.Height() != remoteCST.cs.Height() {
				t.Errorf("%v: expected height %v, got %v", tt.msg, remoteCST.cs.Height(), localCST.cs.Height())
			}
			if localCST.cs.CurrentBlock().ID() != remoteCST.cs.CurrentBlock().ID() {
				t.Errorf("%v: remote and local CSTs have different current blocks", tt.msg)
			}
		} else {
			// Verify that the local cs did not change.
			if localCST.cs.CurrentBlock().ID() != localCurrentBlockID {
				t.Errorf("%v: the local CS is at a different current block than before SendBlocks", tt.msg)
			}
		}

		// Cleanup.
		localCST.cs.gateway.Disconnect(remoteCST.cs.gateway.Address())
		remoteCST.cs.gateway.Disconnect(localCST.cs.gateway.Address())
		err = localCST.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = remoteCST.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestRPCSendBlockSendsOnlyNecessaryBlocks tests that the SendBlocks RPC only
// sends blocks that the caller does not have and that are part of the longest
// chain.
func TestRPCSendBlockSendsOnlyNecessaryBlocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create the "remote" peer.
	cst, err := blankConsensusSetTester("TestRPCSendBlockSendsOnlyNecessaryBlocks - remote")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	// Create the "local" peer.
	//
	// We create this peer manually (not using blankConsensusSetTester) so that we
	// can connect it to the remote peer before calling consensus.New so as to
	// prevent SendBlocks from triggering on Connect.
	testdir := build.TempDir(modules.ConsensusDir, "TestRPCSendBlockSendsOnlyNecessaryBlocks - local")
	g, err := gateway.New("localhost:0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	err = g.Connect(cst.cs.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Add a few initial blocks to both consensus sets. These are the blocks we
	// want to make sure SendBlocks is not sending unnecessarily as both parties
	// already have them.
	knownBlocks := make(map[types.BlockID]struct{})
	for i := 0; i < 20; i++ {
		b, err := cst.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = cst.cs.managedAcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
		err = cs.managedAcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
		knownBlocks[b.ID()] = struct{}{}
	}

	// Add a few blocks to only the remote peer and store which blocks we add.
	addedBlocks := make(map[types.BlockID]struct{})
	for i := 0; i < 20; i++ {
		b, err := cst.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = cst.cs.managedAcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
		addedBlocks[b.ID()] = struct{}{}
	}

	err = cs.gateway.RPC(cst.cs.gateway.Address(), "SendBlocks", func(conn modules.PeerConn) error {
		// Get blockIDs to send.
		var history [32]types.BlockID
		cs.mu.RLock()
		err := cs.db.View(func(tx *bolt.Tx) error {
			history = blockHistory(tx)
			return nil
		})
		cs.mu.RUnlock()
		if err != nil {
			return err
		}

		// Send the block ids.
		if err := encoding.WriteObject(conn, history); err != nil {
			return err
		}

		moreAvailable := true
		for moreAvailable {
			// Read a slice of blocks from the wire.
			var newBlocks []types.Block
			if err := encoding.ReadObject(conn, &newBlocks, uint64(MaxCatchUpBlocks)*types.BlockSizeLimit); err != nil {
				return err
			}
			if err := encoding.ReadObject(conn, &moreAvailable, 1); err != nil {
				return err
			}

			// Check if the block needed to be sent.
			for _, newB := range newBlocks {
				_, ok := knownBlocks[newB.ID()]
				if ok {
					t.Error("SendBlocks sent an unnecessary block that the caller already had")
					continue
				}
				_, ok = addedBlocks[newB.ID()]
				if !ok {
					t.Error("SendBlocks sent an unnecessary block that the caller did not have")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
