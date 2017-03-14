package miner

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestIntegrationBlockHeightReorg checks that the miner has the correct block
// height after a series of reorgs that go as far as the genesis block.
func TestIntegrationBlockHeightReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create 3 miner testers that will be used to cause each other to reorg.
	mt1, err := createMinerTester(t.Name() + "1")
	if err != nil {
		t.Fatal(err)
	}
	mt2, err := createMinerTester(t.Name() + "2")
	if err != nil {
		t.Fatal(err)
	}
	mt3, err := createMinerTester(t.Name() + "3")
	if err != nil {
		t.Fatal(err)
	}

	// Put one ahead of the other multiple times, which should thrash around
	// the height calculation and cause problems by dipping down to the genesis
	// block repeatedly.
	for i := 0; i < 2; i++ {
		b, err := mt1.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		mt1.minedBlocks = append(mt1.minedBlocks, b)
	}
	for i := 0; i < 3; i++ {
		b, err := mt2.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		mt2.minedBlocks = append(mt2.minedBlocks, b)
	}
	for _, block := range mt2.minedBlocks {
		err = mt1.cs.AcceptBlock(block)
		if err != nil && err != modules.ErrNonExtendingBlock {
			t.Fatal(err)
		}
	}
	if mt1.cs.CurrentBlock().ID() != mt2.cs.CurrentBlock().ID() {
		t.Fatal("mt1 and mt2 should have the same current block")
	}
	for i := 0; i < 2; i++ {
		b, err := mt1.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		mt1.minedBlocks = append(mt1.minedBlocks, b)
	}
	for i := 0; i < 3; i++ {
		b, err := mt2.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		mt2.minedBlocks = append(mt2.minedBlocks, b)
	}
	for _, block := range mt2.minedBlocks {
		err = mt1.cs.AcceptBlock(block)
		if err != nil && err != modules.ErrNonExtendingBlock && err != modules.ErrBlockKnown {
			t.Fatal(err)
		}
	}
	if mt1.cs.CurrentBlock().ID() != mt2.cs.CurrentBlock().ID() {
		t.Fatal("mt1 and mt2 should have the same current block")
	}
	for i := 0; i < 7; i++ {
		b, err := mt3.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		mt3.minedBlocks = append(mt3.minedBlocks, b)
	}
	for _, block := range mt3.minedBlocks {
		err = mt1.cs.AcceptBlock(block)
		if err != nil && err != modules.ErrNonExtendingBlock {
			t.Fatal(err)
		}
	}
	if mt1.cs.CurrentBlock().ID() == mt2.cs.CurrentBlock().ID() {
		t.Fatal("mt1 and mt2 should not have the same block height")
	}
	if mt1.cs.CurrentBlock().ID() != mt3.cs.CurrentBlock().ID() {
		t.Fatal("mt1 and mt3 should have the same current block")
	}
}
