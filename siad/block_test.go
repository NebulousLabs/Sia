package main

import (
	"testing"
)

func testEmptyBlock(t *testing.T, d *daemon) {
	// Check that the block will actually be empty.
	tset, err := d.tpool.TransactionSet()
	if err != nil {
		t.Fatal(err)
	}
	if len(tset) != 0 {
		t.Error("transaction pool is not empty")
	}

	// Create and submit an empty block.
	originalHeight := d.state.Height()
	originalUtxoSize := len(d.state.SortedUtxoSet())
	mineSingleBlock(t, d)
	if d.state.Height() != originalHeight+1 {
		t.Errorf("height should have increased by 1, went from %v to %v.", originalHeight, d.state.Height())
	}
	if len(d.state.SortedUtxoSet()) != originalUtxoSize+1 {
		t.Errorf("Uxto should have increased by 1, went from %v to %v.", originalUtxoSize, len(d.state.SortedUtxoSet()))
	}
}

func TestBlockHandling(t *testing.T) {
	d, err := testingDaemon()
	if err != nil {
		t.Fatal(err)
	}

	testEmptyBlock(t, d)
}
