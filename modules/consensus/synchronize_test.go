package consensus

import (
	"testing"
	"time"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/types"
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
	defer cst1.closeCst()
	cst2, err := createConsensusSetTester("TestSynchronize2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.closeCst()

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
	defer cst1.closeCst()
	cst2, err := createConsensusSetTester("TestResynchronize2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.closeCst()

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
	defer cst.closeCst()

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
	lockID := cst.cs.mu.Lock()
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

	cst.cs.mu.Unlock(lockID)

	// remaining IDs should be empty
	var emptyID types.BlockID
	for i, id := range history[14:31] {
		if id != emptyID {
			t.Errorf("Expected empty ID at index %v, got %v", i+17, id)
		}
	}
}
