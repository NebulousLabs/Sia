package consensus

import (
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
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
func (g *mockGatewayCountBroadcasts) Broadcast(string, interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.numBroadcasts++
	return
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
		{MaxCatchUpBlocks, 1},
		{2 * MaxCatchUpBlocks, 1},
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
