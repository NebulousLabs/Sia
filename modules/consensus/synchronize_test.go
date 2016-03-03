package consensus

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
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
		{1, 2},
		{2, 2},
		{int(MaxCatchUpBlocks), 2},
		{2 * int(MaxCatchUpBlocks), 2},
		{2*int(MaxCatchUpBlocks) + 1, 2},
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

// mock PeerConns for testing peer conns that fail reading or writing.
type (
	mockPeerConnFailingReader struct {
		modules.PeerConn
	}
	mockPeerConnFailingWriter struct {
		modules.PeerConn
	}
)

var (
	errFailingReader = errors.New("failing reader")
	errFailingWriter = errors.New("failing writer")
)

// Read is a mock implementation of modules.PeerConn.Read that always returns
// an error.
func (mockPeerConnFailingReader) Read([]byte) (int, error) {
	return 0, errFailingReader
}

// Write is a mock implementation of modules.PeerConn.Write that always returns
// an error.
func (mockPeerConnFailingWriter) Write([]byte) (int, error) {
	return 0, errFailingWriter
}

// TestBlockID probes the ConsensusSet.rpcBlockID method and tests that it
// correctly receives block ids and writes out the corresponding blocks.
func TestBlockID(t *testing.T) {
	cst, err := blankConsensusSetTester("TestBlockID")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	p1, p2 := net.Pipe()
	fnErr := make(chan error)

	tests := []struct {
		id      types.BlockID
		conn    modules.PeerConn
		fn      func() // handle reading and writing over the pipe to the mock conn.
		errWant error
		msg     string
	}{
		// TODO: Test with a failing database.
		// Test with a failing reader.
		{
			conn:    mockPeerConnFailingReader{PeerConn: p1},
			fn:      func() { fnErr <- nil },
			errWant: errFailingReader,
			msg:     "expected rpcBlockID to error with a failing reader conn",
		},
		// Test with a block id not found in the blockmap.
		{
			conn: p1,
			fn: func() {
				// Write a block id to the conn.
				fnErr <- encoding.WriteObject(p2, types.BlockID{})
			},
			errWant: errNilItem,
			msg:     "expected rpcBlockID to error with a nonexistent block id",
		},
		// Test with a failing writer.
		{
			conn: mockPeerConnFailingWriter{PeerConn: p1},
			fn: func() {
				// Write a valid block id to the conn.
				fnErr <- encoding.WriteObject(p2, types.GenesisBlock.ID())
			},
			errWant: errFailingWriter,
			msg:     "expected rpcBlockID to error with a failing writer conn",
		},
		// Test with a valid conn and valid block.
		{
			conn: p1,
			fn: func() {
				// Write a valid block id to the conn.
				if err := encoding.WriteObject(p2, types.GenesisBlock.ID()); err != nil {
					fnErr <- err
				}

				// Read the block written to the conn.
				var block types.Block
				if err := encoding.ReadObject(p2, &block, types.BlockSizeLimit); err != nil {
					fnErr <- err
				}
				// Verify the block is the expected block.
				if block.ID() != types.GenesisBlock.ID() {
					fnErr <- fmt.Errorf("rpcBlockID wrote a different block to conn than the block requested. requested block id: %v, received block id: %v", types.GenesisBlock.ID(), block.ID())
				}

				fnErr <- nil
			},
			errWant: nil,
			msg:     "expected rpcBlockID to succeed with a valid conn and valid block",
		},
	}
	for _, tt := range tests {
		go tt.fn()
		err := cst.cs.rpcBlockID(tt.conn)
		if err != tt.errWant {
			t.Errorf("%s: expected to fail with `%v', got: `%v'", tt.msg, tt.errWant, err)
		}
		err = <-fnErr
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestThreadedReceiveBlock probes the RPCFunc returned by
// cs.threadedReceiveBlock and tests that it correctly requests a block id and
// receives a block. Also tests that the block is correctly (not) accepted into
// the consensus set.
func TestThreadedReceiveBlock(t *testing.T) {
	cst, err := blankConsensusSetTester("TestThreadedReceiveBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	p1, p2 := net.Pipe()
	fnErr := make(chan error)

	tests := []struct {
		id      types.BlockID
		conn    modules.PeerConn
		fn      func() // handle reading and writing over the pipe to the mock conn.
		errWant error
		msg     string
	}{
		// Test with failing writer.
		{
			conn:    mockPeerConnFailingWriter{PeerConn: p1},
			fn:      func() { fnErr <- nil },
			errWant: errFailingWriter,
			msg:     "the function returned from threadedReceiveBlock should fail with a PeerConn with a failing writer",
		},
		// Test with failing reader.
		{
			conn: mockPeerConnFailingReader{PeerConn: p1},
			fn: func() {
				// Read the id written to conn.
				var id types.BlockID
				if err := encoding.ReadObject(p2, &id, crypto.HashSize); err != nil {
					fnErr <- err
				}
				// Verify the id is the expected id.
				expectedID := types.BlockID{}
				if id != expectedID {
					fnErr <- fmt.Errorf("id written to conn was %v, but id received was %v", expectedID, id)
				}
				fnErr <- nil
			},
			errWant: errFailingReader,
			msg:     "the function returned from threadedReceiveBlock should fail with a PeerConn with a failing reader",
		},
		// Test with a valid conn, but an invalid block.
		{
			id:   types.BlockID{1},
			conn: p1,
			fn: func() {
				// Read the id written to conn.
				var id types.BlockID
				if err := encoding.ReadObject(p2, &id, crypto.HashSize); err != nil {
					fnErr <- err
				}
				// Verify the id is the expected id.
				expectedID := types.BlockID{1}
				if id != expectedID {
					fnErr <- fmt.Errorf("id written to conn was %v, but id received was %v", expectedID, id)
				}

				// Write an invalid block to conn.
				block := types.Block{}
				if err := encoding.WriteObject(p2, block); err != nil {
					fnErr <- err
				}

				fnErr <- nil
			},
			errWant: errOrphan,
			msg:     "the function returned from threadedReceiveBlock should not accept an invalid block",
		},
		// Test with a valid conn and a valid block.
		{
			id:   types.BlockID{2},
			conn: p1,
			fn: func() {
				// Read the id written to conn.
				var id types.BlockID
				if err := encoding.ReadObject(p2, &id, crypto.HashSize); err != nil {
					fnErr <- err
				}
				// Verify the id is the expected id.
				expectedID := types.BlockID{2}
				if id != expectedID {
					fnErr <- fmt.Errorf("id written to conn was %v, but id received was %v", expectedID, id)
				}

				// Write a valid block to conn.
				block, err := cst.miner.FindBlock()
				if err != nil {
					fnErr <- err
				}
				if err := encoding.WriteObject(p2, block); err != nil {
					fnErr <- err
				}

				fnErr <- nil
			},
			errWant: nil,
			msg:     "the function returned from manageddReceiveBlock should accept a valid block",
		},
	}
	for _, tt := range tests {
		managedReceiveFN := cst.cs.threadedReceiveBlock(tt.id)
		go tt.fn()
		err := managedReceiveFN(tt.conn)
		if err != tt.errWant {
			t.Errorf("%s: expected to fail with `%v', got: `%v'", tt.msg, tt.errWant, err)
		}
		err = <-fnErr
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestIntegrationBlockIDRPC probes the BlockID RPC and tests that blocks are
// correctly requested, received, and accepted into the consensus set.
func TestIntegrationBlockIDRPC(t *testing.T) {
	cst1, err := blankConsensusSetTester("TestIntegrationBlockIDRPC1")
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := blankConsensusSetTester("TestIntegrationBlockIDRPC2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.Close()

	err = cst1.cs.gateway.Connect(cst2.cs.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	err = cst2.cs.gateway.Connect(cst1.cs.gateway.Address()) // TODO: why is this Connect call necessary? Shouldn't they connect to eachother with one Connect?
	if err != nil {
		t.Fatal(err)
	}

	// Test that cst1 doesn't accept a block it's already seen (the genesis block).
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "BlockID", cst1.cs.threadedReceiveBlock(types.GenesisBlock.ID()))
	if err != modules.ErrBlockKnown {
		t.Errorf("cst1 should reject known blocks: expected error '%v', got '%v'", modules.ErrBlockKnown, err)
	}

	// Test that cst2 errors when it doesn't recognize the requested block.
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "BlockID", cst1.cs.threadedReceiveBlock(types.BlockID{}))
	if err != io.EOF {
		t.Errorf("cst2 shouldn't return a block it doesn't recognize: expected error '%v', got '%v'", io.EOF, err)
	}

	// Test that cst1 accepts a block that extends its longest chain.
	block, err := cst2.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = cst2.cs.managedAcceptBlock(block) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "BlockID", cst1.cs.threadedReceiveBlock(block.ID()))
	if err != nil {
		t.Errorf("cst1 should accept a block that extends its longest chain: expected nil error, got '%v'", err)
	}

	// Test that cst2 accepts a block that extends its longest chain.
	block, err = cst1.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = cst1.cs.managedAcceptBlock(block) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	err = cst2.cs.gateway.RPC(cst1.cs.gateway.Address(), "BlockID", cst2.cs.threadedReceiveBlock(block.ID()))
	if err != nil {
		t.Errorf("cst2 should accept a block that extends its longest chain: expected nil error, got '%v'", err)
	}

	// Test that cst1 doesn't accept an orphan block.
	block, err = cst2.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = cst2.cs.managedAcceptBlock(block) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	block, err = cst2.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = cst2.cs.managedAcceptBlock(block) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "BlockID", cst1.cs.threadedReceiveBlock(block.ID()))
	if err != errOrphan {
		t.Errorf("cst1 should not accept an orphan block: expected error '%v', got '%v'", errOrphan, err)
	}
}

type mockGatewayCallsRPC struct {
	modules.Gateway
	rpcCalled chan string
}

func (g *mockGatewayCallsRPC) RPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	g.rpcCalled <- name
	return nil
}

// TestRelayHeader tests that rpcRelayHeader requests the corresponding blocks
// to valid headers with known parents, or requests the block history to orphan
// headers.
func TestRelayHeader(t *testing.T) {
	cst, err := blankConsensusSetTester("TestRelayHeader")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	mg := &mockGatewayCallsRPC{
		rpcCalled: make(chan string),
	}
	cst.cs.gateway = mg

	p1, p2 := net.Pipe()

	// Valid block that rpcRelayHeader should accept.
	validBlock, err := cst.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}

	// A block in the near future that rpcRelayHeader return an error for, but
	// still request the corresponding block.
	block, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.Timestamp = types.CurrentTimestamp() + 2 + types.FutureThreshold
	futureBlock, _ := cst.miner.SolveBlock(block, target)

	tests := []struct {
		header  types.BlockHeader
		errWant error
		errMSG  string
		rpcWant string
		rpcMSG  string
	}{
		// Test that rpcRelayHeader rejects known blocks.
		{
			header:  types.GenesisBlock.Header(),
			errWant: modules.ErrBlockKnown,
			errMSG:  "rpcRelayHeader should reject headers to known blocks",
		},
		// Test that rpcRelayHeader requests the parent blocks of orphan headers.
		{
			header:  types.BlockHeader{},
			errWant: nil,
			errMSG:  "rpcRelayHeader should not return an error for orphan headers",
			rpcWant: "SendBlocks",
			rpcMSG:  "rpcRelayHeader should request blocks when the relayed header is an orphan",
		},
		// Test that rpcRelayHeader accepts a valid header that extends the longest chain.
		{
			header:  validBlock.Header(),
			errWant: nil,
			errMSG:  "rpcRelayHeader should accept a valid header",
			rpcWant: "BlockID",
			rpcMSG:  "rpcRelayHeader should request the block of a valid header",
		},
		// Test that rpcRelayHeader requests a future, but otherwise valid block.
		{
			header:  futureBlock.Header(),
			errWant: nil,
			errMSG:  "rpcRelayHeader should not return an error for a future header",
			rpcWant: "BlockID",
			rpcMSG:  "rpcRelayHeader should request the corresponding block to a future, but otherwise valid header",
		},
	}
	for _, tt := range tests {
		go func() {
			encoding.WriteObject(p1, tt.header)
		}()
		err = cst.cs.rpcRelayHeader(p2)
		if err != tt.errWant {
			t.Errorf("%s: expected '%v', got '%v'", tt.errMSG, tt.errWant, err)
		}
		if tt.rpcWant == "" {
			select {
			case rpc := <-mg.rpcCalled:
				t.Errorf("no RPC call expected, but '%v' was called", rpc)
			case <-time.After(10 * time.Millisecond):
			}
		} else {
			select {
			case rpc := <-mg.rpcCalled:
				if rpc != tt.rpcWant {
					t.Errorf("%s: expected '%v', got '%v'", tt.rpcMSG, tt.rpcWant, rpc)
				}
			case <-time.After(10 * time.Millisecond):
				t.Errorf("%s: expected '%v', but no RPC was called", tt.rpcMSG, tt.rpcWant)
			}
		}
	}
}

// TestIntegrationBroadcastRelayHeader checks that broadcasting RelayHeader
// causes peers to also broadcast the header (if the block is valid).
func TestIntegrationBroadcastRelayHeader(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Setup consensus sets.
	cst1, err := blankConsensusSetTester("TestIntegrationBroadcastRelayHeader1")
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := blankConsensusSetTester("TestIntegrationBroadcastRelayHeader2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.Close()
	// Setup mock gateway.
	mg := &mockGatewayDoesBroadcast{
		Gateway:         cst2.cs.gateway,
		broadcastCalled: make(chan struct{}),
	}
	cst2.cs.gateway = mg
	err = cst1.cs.gateway.Connect(cst2.cs.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}

	// Test that broadcasting an invalid block header over RelayHeader on cst1.cs
	// does not result in cst2.cs.gateway receiving a broadcast.
	cst1.cs.gateway.Broadcast("RelayHeader", types.BlockHeader{}, cst1.cs.gateway.Peers())
	select {
	case <-mg.broadcastCalled:
		t.Fatal("RelayHeader broadcasted an invalid block header")
	case <-time.After(100 * time.Millisecond):
	}

	// Test that broadcasting a valid block header over RelayHeader on cst1.cs
	// causes cst2.cs.gateway to receive a broadcast.
	validBlock, err := cst1.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = cst1.cs.managedAcceptBlock(validBlock)
	if err != nil {
		t.Fatal(err)
	}
	cst1.cs.gateway.Broadcast("RelayHeader", validBlock.Header(), cst1.cs.gateway.Peers())
	select {
	case <-mg.broadcastCalled:
		// Broadcast is called twice, once to broadcast blocks to peers <= v0.5.1
		// and once to broadcast block headers to peers > v0.5.1.
		<-mg.broadcastCalled
	case <-time.After(100 * time.Millisecond):
		t.Fatal("RelayHeader didn't broadcast a valid block header")
	}
}
