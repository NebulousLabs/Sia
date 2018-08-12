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

	"github.com/coreos/bbolt"
)

// TestSynchronize tests that the consensus set can successfully synchronize
// to a peer.
func TestSynchronize(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst1, err := createConsensusSetTester(t.Name() + "1")
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := createConsensusSetTester(t.Name() + "2")
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
	for i := 0; i < 50; i++ {
		if cst1.cs.dbCurrentBlockID() != cst2.cs.dbCurrentBlockID() {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if cst1.cs.dbCurrentBlockID() != cst2.cs.dbCurrentBlockID() {
		t.Fatal("Synchronize failed")
	}

	// Mine on cst2 until it is more than 'MaxCatchUpBlocks' ahead of cst1.
	// NOTE: we have to disconnect prior to this, otherwise cst2 will relay
	// blocks to cst1.
	cst1.gateway.Disconnect(cst2.gateway.Address())
	cst2.gateway.Disconnect(cst1.gateway.Address())
	for cst2.cs.dbBlockHeight() < cst1.cs.dbBlockHeight()+3+MaxCatchUpBlocks {
		_, err := cst2.miner.AddBlock()
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
	for i := 0; i < 50; i++ {
		if cst1.cs.dbBlockHeight() != cst2.cs.dbBlockHeight() {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if cst1.cs.dbBlockHeight() != cst2.cs.dbBlockHeight() {
		t.Fatal("synchronize failed")
	}

	// extend cst2 with a "bad" (old) block, and synchronize. cst1 should
	// reject the bad block.
	cst2.cs.mu.Lock()
	id, err := cst2.cs.dbGetPath(0)
	if err != nil {
		t.Fatal(err)
	}
	cst2.cs.dbPushPath(id)
	cst2.cs.mu.Unlock()

	// Sleep for a few seconds to allow the network call between the two time
	// to occur.
	time.Sleep(5 * time.Second)
	if cst1.cs.dbBlockHeight() == cst2.cs.dbBlockHeight() {
		t.Fatal("cst1 did not reject bad block")
	}
}

// TestBlockHistory tests that blockHistory returns the expected sequence of
// block IDs.
func TestBlockHistory(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst, err := createConsensusSetTester(t.Name())
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
	if testing.Short() {
		t.SkipNow()
	}

	// Setup consensus sets.
	cst1, err := blankConsensusSetTester(t.Name()+"1", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := blankConsensusSetTester(t.Name()+"2", modules.ProdDependencies)
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
		synced                bool
	}{
		// Test that no blocks are broadcast during IBD.
		{
			blocksToMine:          0,
			expectedNumBroadcasts: 0,
			synced:                false,
		},
		{
			blocksToMine:          1,
			expectedNumBroadcasts: 0,
			synced:                false,
		},
		{
			blocksToMine:          2,
			expectedNumBroadcasts: 0,
			synced:                false,
		},
		// Test that only one blocks is broadcast when IBD is done.
		{
			blocksToMine:          0,
			expectedNumBroadcasts: 0,
			synced:                true,
		},
		{
			blocksToMine:          1,
			expectedNumBroadcasts: 1,
			synced:                true,
		},
		{
			blocksToMine:          2,
			expectedNumBroadcasts: 1,
			synced:                true,
		},
		{
			blocksToMine:          int(MaxCatchUpBlocks),
			expectedNumBroadcasts: 1,
			synced:                true,
		},
		{
			blocksToMine:          2 * int(MaxCatchUpBlocks),
			expectedNumBroadcasts: 1,
			synced:                true,
		},
		{
			blocksToMine:          2*int(MaxCatchUpBlocks) + 1,
			expectedNumBroadcasts: 1,
			synced:                true,
		},
	}
	for j, test := range tests {
		cst1.cs.mu.Lock()
		cst1.cs.synced = test.synced
		cst1.cs.mu.Unlock()
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
			_, err = cst2.cs.managedAcceptBlocks([]types.Block{b})
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
			t.Errorf("test #%d: expected %d number of broadcasts, got %d", j, test.expectedNumBroadcasts, numBroadcasts)
		}
	}
}

// TestIntegrationRPCSendBlocks tests that the SendBlocks RPC adds blocks to
// the consensus set, and that the consensus set catches with the remote peer
// and possibly reorgs.
func TestIntegrationRPCSendBlocks(t *testing.T) {
	if testing.Short() || !build.VLONG {
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
		remoteCST, err := blankConsensusSetTester(filepath.Join(t.Name()+" - remote", strconv.Itoa(i)), modules.ProdDependencies)
		if err != nil {
			t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
		}
		// Create the "local" peer.
		localCST, err := blankConsensusSetTester(filepath.Join(t.Name()+" - local", strconv.Itoa(i)), modules.ProdDependencies)
		if err != nil {
			t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
		}

		localCST.cs.gateway.Connect(remoteCST.cs.gateway.Address())
		// Wait a second to let the OnConnectRPCs finish
		time.Sleep(100 * time.Millisecond)

		// Mine blocks.
		for i := types.BlockHeight(0); i < tt.commonBlocksToMine; i++ {
			b, err := remoteCST.miner.FindBlock()
			if err != nil {
				t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
			}
			_, err = remoteCST.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil {
				t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
			}
			_, err = localCST.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil {
				t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
			}
		}
		for i := types.BlockHeight(0); i < tt.remoteBlocksToMine; i++ {
			b, err := remoteCST.miner.FindBlock()
			if err != nil {
				t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
			}
			_, err = remoteCST.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil {
				t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
			}
		}
		for i := types.BlockHeight(0); i < tt.localBlocksToMine; i++ {
			b, err := localCST.miner.FindBlock()
			if err != nil {
				t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
			}
			_, err = localCST.cs.managedAcceptBlocks([]types.Block{b})
			if err != nil {
				t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
			}
		}

		localCurrentBlockID := localCST.cs.CurrentBlock().ID()
		remoteCurrentBlockID := remoteCST.cs.CurrentBlock().ID()

		err = localCST.cs.gateway.RPC(remoteCST.cs.gateway.Address(), "SendBlocks", localCST.cs.threadedReceiveBlocks)
		if err != nil {
			t.Errorf("test #%d, %v: %v", i, tt.msg, err)
		}

		// Assume that if remoteBlocksToMine is greater than localBlocksToMine, then
		// the local CS must have received the new blocks (and reorged).
		if tt.remoteBlocksToMine > tt.localBlocksToMine {
			// Verify that the remote cs did not change.
			if remoteCST.cs.CurrentBlock().ID() != remoteCurrentBlockID {
				t.Errorf("test #%d, %v: the remote CS is at a different current block than before SendBlocks", i, tt.msg)
			}
			// Verify that the local cs got the new blocks.
			if localCST.cs.Height() != remoteCST.cs.Height() {
				t.Errorf("test #%d, %v: expected height %v, got %v", i, tt.msg, remoteCST.cs.Height(), localCST.cs.Height())
			}
			if localCST.cs.CurrentBlock().ID() != remoteCST.cs.CurrentBlock().ID() {
				t.Errorf("test #%d, %v: remote and local CSTs have different current blocks", i, tt.msg)
			}
		} else {
			// Verify that the local cs did not change.
			if localCST.cs.CurrentBlock().ID() != localCurrentBlockID {
				t.Errorf("test #%d, %v: the local CS is at a different current block than before SendBlocks", i, tt.msg)
			}
		}

		// Cleanup.
		err = localCST.Close()
		if err != nil {
			t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
		}
		err = remoteCST.Close()
		if err != nil {
			t.Fatalf("test #%d, %v: %v", i, tt.msg, err)
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
	cst, err := blankConsensusSetTester(t.Name()+"- remote", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	// Create the "local" peer.
	//
	// We create this peer manually (not using blankConsensusSetTester) so that we
	// can connect it to the remote peer before calling consensus.New so as to
	// prevent SendBlocks from triggering on Connect.
	testdir := build.TempDir(modules.ConsensusDir, t.Name()+" - local")
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	err = g.Connect(cst.cs.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// Add a few initial blocks to both consensus sets. These are the blocks we
	// want to make sure SendBlocks is not sending unnecessarily as both parties
	// already have them.
	knownBlocks := make(map[types.BlockID]struct{})
	for i := 0; i < 20; i++ {
		b, err := cst.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		_, err = cst.cs.managedAcceptBlocks([]types.Block{b})
		if err != nil {
			t.Fatal(err)
		}
		_, err = cs.managedAcceptBlocks([]types.Block{b})
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
		_, err = cst.cs.managedAcceptBlocks([]types.Block{b})
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
	mockPeerConn struct {
		net.Conn
	}
	mockPeerConnFailingReader struct {
		mockPeerConn
	}
	mockPeerConnFailingWriter struct {
		mockPeerConn
	}
)

var (
	errFailingReader = errors.New("failing reader")
	errFailingWriter = errors.New("failing writer")
)

// Close returns 'nil', and does nothing behind the scenes. This is because the
// testing reuses pipes, but the consensus code now correctly closes conns after
// handling them.
func (pc mockPeerConn) Close() error {
	return nil
}

// RPCAddr implements this method of the modules.PeerConn interface.
func (pc mockPeerConn) RPCAddr() modules.NetAddress {
	return "mockPeerConn dialback addr"
}

// SetDeadline returns 'nil', and does nothing behind the scenes.
func (pc mockPeerConn) SetDeadline(time.Time) error {
	return nil
}

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

// TestSendBlk probes the ConsensusSet.rpcSendBlk method and tests that it
// correctly receives block ids and writes out the corresponding blocks.
func TestSendBlk(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := blankConsensusSetTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	p1, p2 := net.Pipe()
	mockP1 := mockPeerConn{p1}
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
			conn:    mockPeerConnFailingReader{mockP1},
			fn:      func() { fnErr <- nil },
			errWant: errFailingReader,
			msg:     "expected rpcSendBlk to error with a failing reader conn",
		},
		// Test with a block id not found in the blockmap.
		{
			conn: mockP1,
			fn: func() {
				// Write a block id to the conn.
				fnErr <- encoding.WriteObject(p2, types.BlockID{})
			},
			errWant: errNilItem,
			msg:     "expected rpcSendBlk to error with a nonexistent block id",
		},
		// Test with a failing writer.
		{
			conn: mockPeerConnFailingWriter{mockP1},
			fn: func() {
				// Write a valid block id to the conn.
				fnErr <- encoding.WriteObject(p2, types.GenesisID)
			},
			errWant: errFailingWriter,
			msg:     "expected rpcSendBlk to error with a failing writer conn",
		},
		// Test with a valid conn and valid block.
		{
			conn: mockP1,
			fn: func() {
				// Write a valid block id to the conn.
				if err := encoding.WriteObject(p2, types.GenesisID); err != nil {
					fnErr <- err
				}

				// Read the block written to the conn.
				var block types.Block
				if err := encoding.ReadObject(p2, &block, types.BlockSizeLimit); err != nil {
					fnErr <- err
				}
				// Verify the block is the expected block.
				if block.ID() != types.GenesisID {
					fnErr <- fmt.Errorf("rpcSendBlk wrote a different block to conn than the block requested. requested block id: %v, received block id: %v", types.GenesisID, block.ID())
				}

				fnErr <- nil
			},
			errWant: nil,
			msg:     "expected rpcSendBlk to succeed with a valid conn and valid block",
		},
	}
	for _, tt := range tests {
		go tt.fn()
		err := cst.cs.rpcSendBlk(tt.conn)
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
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := blankConsensusSetTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	p1, p2 := net.Pipe()
	mockP1 := mockPeerConn{p1}
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
			conn:    mockPeerConnFailingWriter{mockP1},
			fn:      func() { fnErr <- nil },
			errWant: errFailingWriter,
			msg:     "the function returned from threadedReceiveBlock should fail with a PeerConn with a failing writer",
		},
		// Test with failing reader.
		{
			conn: mockPeerConnFailingReader{mockP1},
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
			conn: mockP1,
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
			conn: mockP1,
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
		managedReceiveFN := cst.cs.managedReceiveBlock(tt.id)
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

// TestIntegrationSendBlkRPC probes the SendBlk RPC and tests that blocks are
// correctly requested, received, and accepted into the consensus set.
func TestIntegrationSendBlkRPC(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst1, err := blankConsensusSetTester(t.Name()+"1", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := blankConsensusSetTester(t.Name()+"2", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.Close()

	err = cst1.cs.gateway.Connect(cst2.cs.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	// Sleep to give the consensus sets time to finish the background startup
	// routines - if the block mined below is mined before the sets finish
	// synchronizing to each other, it screws up the test.
	time.Sleep(500 * time.Millisecond)

	// Test that cst1 doesn't accept a block it's already seen (the genesis block).
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "SendBlk", cst1.cs.managedReceiveBlock(types.GenesisID))
	if err != modules.ErrBlockKnown && err != modules.ErrNonExtendingBlock {
		t.Errorf("cst1 should reject known blocks: expected error '%v', got '%v'", modules.ErrBlockKnown, err)
	}
	// Test that cst2 errors when it doesn't recognize the requested block.
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "SendBlk", cst1.cs.managedReceiveBlock(types.BlockID{}))
	if err != io.EOF {
		t.Errorf("cst2 shouldn't return a block it doesn't recognize: expected error '%v', got '%v'", io.EOF, err)
	}

	// Test that cst1 accepts a block that extends its longest chain.
	block, err := cst2.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst2.cs.managedAcceptBlocks([]types.Block{block}) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "SendBlk", cst1.cs.managedReceiveBlock(block.ID()))
	if err != nil {
		t.Errorf("cst1 should accept a block that extends its longest chain: expected nil error, got '%v'", err)
	}

	// Test that cst2 accepts a block that extends its longest chain.
	block, err = cst1.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst1.cs.managedAcceptBlocks([]types.Block{block}) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	err = cst2.cs.gateway.RPC(cst1.cs.gateway.Address(), "SendBlk", cst2.cs.managedReceiveBlock(block.ID()))
	if err != nil {
		t.Errorf("cst2 should accept a block that extends its longest chain: expected nil error, got '%v'", err)
	}

	// Test that cst1 doesn't accept an orphan block.
	block, err = cst2.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst2.cs.managedAcceptBlocks([]types.Block{block}) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	block, err = cst2.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst2.cs.managedAcceptBlocks([]types.Block{block}) // Call managedAcceptBlock so that the block isn't broadcast.
	if err != nil {
		t.Fatal(err)
	}
	err = cst1.cs.gateway.RPC(cst2.cs.gateway.Address(), "SendBlk", cst1.cs.managedReceiveBlock(block.ID()))
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
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := blankConsensusSetTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	mg := &mockGatewayCallsRPC{
		Gateway:   cst.cs.gateway,
		rpcCalled: make(chan string),
	}
	cst.cs.gateway = mg

	p1, p2 := net.Pipe()
	mockP2 := mockPeerConn{p2}

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
			rpcWant: "SendBlk",
			rpcMSG:  "rpcRelayHeader should request the block of a valid header",
		},
		// Test that rpcRelayHeader requests a future, but otherwise valid block.
		{
			header:  futureBlock.Header(),
			errWant: nil,
			errMSG:  "rpcRelayHeader should not return an error for a future header",
			rpcWant: "SendBlk",
			rpcMSG:  "rpcRelayHeader should request the corresponding block to a future, but otherwise valid header",
		},
	}
	errChan := make(chan error)
	for _, tt := range tests {
		go func() {
			errChan <- encoding.WriteObject(p1, tt.header)
		}()
		err = cst.cs.threadedRPCRelayHeader(mockP2)
		if err != tt.errWant {
			t.Errorf("%s: expected '%v', got '%v'", tt.errMSG, tt.errWant, err)
		}
		err = <-errChan
		if err != nil {
			t.Fatal(err)
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
	cst1, err := blankConsensusSetTester(t.Name()+"1", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := blankConsensusSetTester(t.Name()+"2", modules.ProdDependencies)
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
	// Give time for on connect RPCs to finish.
	time.Sleep(500 * time.Millisecond)

	// Test that broadcasting an invalid block header over RelayHeader on cst1.cs
	// does not result in cst2.cs.gateway receiving a broadcast.
	cst1.cs.gateway.Broadcast("RelayHeader", types.BlockHeader{}, cst1.cs.gateway.Peers())
	select {
	case <-mg.broadcastCalled:
		t.Fatal("RelayHeader broadcasted an invalid block header")
	case <-time.After(500 * time.Millisecond):
	}

	// Test that broadcasting a valid block header over RelayHeader on cst1.cs
	// causes cst2.cs.gateway to receive a broadcast.
	validBlock, err := cst1.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst1.cs.managedAcceptBlocks([]types.Block{validBlock})
	if err != nil {
		t.Fatal(err)
	}
	cst1.cs.gateway.Broadcast("RelayHeader", validBlock.Header(), cst1.cs.gateway.Peers())
	select {
	case <-mg.broadcastCalled:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("RelayHeader didn't broadcast a valid block header")
	}
}

// TestIntegrationRelaySynchronize tests that blocks are relayed as they are
// accepted and that peers stay synchronized.
func TestIntegrationRelaySynchronize(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst1, err := blankConsensusSetTester(t.Name()+"1", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.Close()
	cst2, err := blankConsensusSetTester(t.Name()+"2", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.Close()
	cst3, err := blankConsensusSetTester(t.Name()+"3", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst3.Close()

	// Connect them like so: cst1 <-> cst2 <-> cst3
	err = cst1.gateway.Connect(cst2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	err = cst2.gateway.Connect(cst3.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	// Make sure cst1 is not connected to cst3.
	cst1.gateway.Disconnect(cst3.gateway.Address())
	cst3.gateway.Disconnect(cst1.gateway.Address())

	// Spin until the connection calls have completed.
	for i := 0; i < 100; i++ {
		time.Sleep(150 * time.Millisecond)
		if len(cst1.gateway.Peers()) >= 1 && len(cst3.gateway.Peers()) >= 1 {
			break
		}
	}
	if len(cst1.gateway.Peers()) < 1 || len(cst3.gateway.Peers()) < 1 {
		t.Fatal("Peer connection has failed.")
	}

	// Mine a block on cst1, expecting the block to propagate from cst1 to
	// cst2, and then to cst3.
	b1, err := cst1.miner.AddBlock()
	if err != nil {
		t.Log(b1.ID())
		t.Log(cst1.cs.CurrentBlock().ID())
		t.Log(cst2.cs.CurrentBlock().ID())
		t.Fatal(err)
	}

	// Spin until the block has propagated to cst2.
	for i := 0; i < 100; i++ {
		time.Sleep(150 * time.Millisecond)
		if cst2.cs.CurrentBlock().ID() == b1.ID() {
			break
		}
	}
	if cst2.cs.CurrentBlock().ID() != b1.ID() {
		t.Fatal("Block propagation has failed")
	}
	// Spin until the block has propagated to cst3.
	for i := 0; i < 100; i++ {
		time.Sleep(150 * time.Millisecond)
		if cst3.cs.CurrentBlock().ID() == b1.ID() {
			break
		}
	}
	if cst3.cs.CurrentBlock().ID() != b1.ID() {
		t.Fatal("Block propagation has failed")
	}

	// Mine a block on cst2.
	b2, err := cst2.miner.AddBlock()
	if err != nil {
		t.Log(b1.ID())
		t.Log(b2.ID())
		t.Log(cst2.cs.CurrentBlock().ID())
		t.Log(cst3.cs.CurrentBlock().ID())
		t.Fatal(err)
	}
	// Spin until the block has propagated to cst1.
	for i := 0; i < 100; i++ {
		time.Sleep(150 * time.Millisecond)
		if cst1.cs.CurrentBlock().ID() == b2.ID() {
			break
		}
	}
	if cst1.cs.CurrentBlock().ID() != b2.ID() {
		t.Fatal("block propagation has failed")
	}
	// Spin until the block has propagated to cst3.
	for i := 0; i < 100; i++ {
		time.Sleep(150 * time.Millisecond)
		if cst3.cs.CurrentBlock().ID() == b2.ID() {
			break
		}
	}
	if cst3.cs.CurrentBlock().ID() != b2.ID() {
		t.Fatal("block propagation has failed")
	}

	// Mine a block on cst3.
	b3, err := cst3.miner.AddBlock()
	if err != nil {
		t.Log(b1.ID())
		t.Log(b2.ID())
		t.Log(b3.ID())
		t.Log(cst1.cs.CurrentBlock().ID())
		t.Log(cst2.cs.CurrentBlock().ID())
		t.Log(cst3.cs.CurrentBlock().ID())
		t.Fatal(err)
	}
	// Spin until the block has propagated to cst1.
	for i := 0; i < 100; i++ {
		time.Sleep(150 * time.Millisecond)
		if cst1.cs.CurrentBlock().ID() == b3.ID() {
			break
		}
	}
	if cst1.cs.CurrentBlock().ID() != b3.ID() {
		t.Fatal("block propagation has failed")
	}
	// Spin until the block has propagated to cst2.
	for i := 0; i < 100; i++ {
		time.Sleep(150 * time.Millisecond)
		if cst2.cs.CurrentBlock().ID() == b3.ID() {
			break
		}
	}
	if cst2.cs.CurrentBlock().ID() != b3.ID() {
		t.Fatal("block propagation has failed")
	}

	// Check that cst1 and cst3 are not peers, if they are peers then this test
	// is invalid because it has failed to be certain that blocks can make
	// multiple hops.
	if len(cst1.gateway.Peers()) != 1 || cst1.gateway.Peers()[0].NetAddress == cst3.gateway.Address() {
		t.Log("Test is invalid, cst1 and cst3 have connected to each other")
	}
	if len(cst3.gateway.Peers()) != 1 || cst3.gateway.Peers()[0].NetAddress == cst1.gateway.Address() {
		t.Log("Test is invalid, cst3 and cst1 have connected to each other")
	}
}

// mockPeerConnMockReadWrite is a mock implementation of modules.PeerConn that
// returns fails reading or writing if readErr or writeErr is non-nil,
// respectively.
type mockPeerConnMockReadWrite struct {
	modules.PeerConn
	readErr  error
	writeErr error
}

// Read is a mock implementation of conn.Read that fails with the mock error if
// readErr != nil.
func (conn mockPeerConnMockReadWrite) Read(b []byte) (n int, err error) {
	if conn.readErr != nil {
		return 0, conn.readErr
	}
	return conn.PeerConn.Read(b)
}

// Write is a mock implementation of conn.Write that fails with the mock error
// if writeErr != nil.
func (conn mockPeerConnMockReadWrite) Write(b []byte) (n int, err error) {
	if conn.writeErr != nil {
		return 0, conn.writeErr
	}
	return conn.PeerConn.Write(b)
}

// mockNetError is a mock net.Error.
type mockNetError struct {
	error
	timeout   bool
	temporary bool
}

// Timeout is a mock implementation of net.Error.Timeout.
func (err mockNetError) Timeout() bool {
	return err.timeout
}

// Temporary is a mock implementation of net.Error.Temporary.
func (err mockNetError) Temporary() bool {
	return err.temporary
}

// TestThreadedReceiveBlocksStalls tests that threadedReceiveBlocks returns
// errSendBlocksStalled when the connection times out before a block is
// received.
func TestThreadedReceiveBlocksStalls(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst, err := blankConsensusSetTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	p1, p2 := net.Pipe()
	mockP2 := mockPeerConn{p2}

	writeTimeoutConn := mockPeerConnMockReadWrite{
		PeerConn: mockP2,
		writeErr: mockNetError{
			error:   errors.New("Write timeout"),
			timeout: true,
		},
	}
	readTimeoutConn := mockPeerConnMockReadWrite{
		PeerConn: mockP2,
		readErr: mockNetError{
			error:   errors.New("Read timeout"),
			timeout: true,
		},
	}

	readNetErrConn := mockPeerConnMockReadWrite{
		PeerConn: mockP2,
		readErr: mockNetError{
			error: errors.New("mock read net.Error"),
		},
	}
	writeNetErrConn := mockPeerConnMockReadWrite{
		PeerConn: mockP2,
		writeErr: mockNetError{
			error: errors.New("mock write net.Error"),
		},
	}

	readErrConn := mockPeerConnMockReadWrite{
		PeerConn: mockP2,
		readErr:  errors.New("mock read err"),
	}
	writeErrConn := mockPeerConnMockReadWrite{
		PeerConn: mockP2,
		writeErr: errors.New("mock write err"),
	}

	// Test that threadedReceiveBlocks errors with errSendBlocksStalled when 0
	// blocks have been sent and the conn times out.
	err = cst.cs.threadedReceiveBlocks(writeTimeoutConn)
	if err != errSendBlocksStalled {
		t.Errorf("expected threadedReceiveBlocks to err with \"%v\", got \"%v\"", errSendBlocksStalled, err)
	}
	errChan := make(chan error)
	go func() {
		var knownBlocks [32]types.BlockID
		errChan <- encoding.ReadObject(p1, &knownBlocks, 32*crypto.HashSize)
	}()
	err = cst.cs.threadedReceiveBlocks(readTimeoutConn)
	if err != errSendBlocksStalled {
		t.Errorf("expected threadedReceiveBlocks to err with \"%v\", got \"%v\"", errSendBlocksStalled, err)
	}
	err = <-errChan
	if err != nil {
		t.Fatal(err)
	}

	// Test that threadedReceiveBlocks errors when writing the block history fails.
	// Test with an error of type net.Error.
	err = cst.cs.threadedReceiveBlocks(writeNetErrConn)
	if err != writeNetErrConn.writeErr {
		t.Errorf("expected threadedReceiveBlocks to err with \"%v\", got \"%v\"", writeNetErrConn.writeErr, err)
	}
	// Test with an error of type error.
	err = cst.cs.threadedReceiveBlocks(writeErrConn)
	if err != writeErrConn.writeErr {
		t.Errorf("expected threadedReceiveBlocks to err with \"%v\", got \"%v\"", writeErrConn.writeErr, err)
	}

	// Test that threadedReceiveBlocks errors when reading blocks fails.
	// Test with an error of type net.Error.
	go func() {
		var knownBlocks [32]types.BlockID
		errChan <- encoding.ReadObject(p1, &knownBlocks, 32*crypto.HashSize)
	}()
	err = cst.cs.threadedReceiveBlocks(readNetErrConn)
	if err != readNetErrConn.readErr {
		t.Errorf("expected threadedReceiveBlocks to err with \"%v\", got \"%v\"", readNetErrConn.readErr, err)
	}
	err = <-errChan
	if err != nil {
		t.Fatal(err)
	}
	// Test with an error of type error.
	go func() {
		var knownBlocks [32]types.BlockID
		errChan <- encoding.ReadObject(p1, &knownBlocks, 32*crypto.HashSize)
	}()
	err = cst.cs.threadedReceiveBlocks(readErrConn)
	if err != readErrConn.readErr {
		t.Errorf("expected threadedReceiveBlocks to err with \"%v\", got \"%v\"", readErrConn.readErr, err)
	}
	err = <-errChan
	if err != nil {
		t.Fatal(err)
	}

	// TODO: Test that threadedReceiveBlocks doesn't error with a timeout if it has received one block before this timed out read/write.

	// TODO: Test that threadedReceiveBlocks doesn't error with errSendBlocksStalled if it successfully received one block.
}

// TestIntegrationSendBlocksStalls tests that the SendBlocks RPC fails with
// errSendBlockStalled when the RPC timesout and the requesting end has
// received 0 blocks.
func TestIntegrationSendBlocksStalls(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cstLocal, err := blankConsensusSetTester(t.Name()+"- local", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cstLocal.Close()
	cstRemote, err := blankConsensusSetTester(t.Name()+"- remote", modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer cstRemote.Close()

	cstLocal.cs.gateway.Connect(cstRemote.cs.gateway.Address())

	// Lock the remote CST so that SendBlocks blocks and timesout.
	cstRemote.cs.mu.Lock()
	defer cstRemote.cs.mu.Unlock()
	err = cstLocal.cs.gateway.RPC(cstRemote.cs.gateway.Address(), "SendBlocks", cstLocal.cs.threadedReceiveBlocks)
	if err != errSendBlocksStalled {
		t.Fatal(err)
	}
}
