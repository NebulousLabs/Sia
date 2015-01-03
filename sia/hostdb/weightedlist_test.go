package hostdb

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

func TestWeightedList(t *testing.T) {
	// Create a hostdb and 3 equal entries to insert.
	hdb := New()

	// Create a bunch of host entries of equal weight.
	firstInsertions := 64
	for i := 0; i < firstInsertions; i++ {
		entry := HostEntry{
			ID:     strconv.Itoa(i),
			Burn:   10,
			Freeze: 10,
			Price:  10,
		}
		hdb.Insert(entry)
	}

	// Check that the length of activeHosts and the count of hostTree are
	// consistent.
	if len(hdb.activeHosts) != firstInsertions {
		t.Error("activeHosts should equal ", firstInsertions, "equals", len(hdb.activeHosts))
	}
	if hdb.hostTree.count != firstInsertions {
		t.Error("hostTree count is off")
	}

	// Check that the weight of the hostTree is what is expected.
	randomHost, err := hdb.RandomHost()
	if err != nil {
		t.Fatal(err)
	}
	expectedWeight := consensus.Currency(firstInsertions) * randomHost.Weight()
	if hdb.hostTree.weight != expectedWeight {
		t.Error("Expected weight is incorrect")
	}

	// Select many random hosts and do naive statistical analysis on the
	// results.
	if !testing.Short() {
		// Pull a bunch of random hosts and count how many times we pull each
		// host.
		selectionSlice := make([]int, firstInsertions)
		expected := 50
		for i := 0; i < expected*firstInsertions; i++ {
			entry, err := hdb.RandomHost()
			if err != nil {
				t.Fatal(err)
			}
			idInt, err := strconv.Atoi(entry.ID)
			if err != nil {
				t.Fatal(err)
			}
			selectionSlice[idInt]++
		}

		// See if each host was selected enough times.
		errorBound := 21
		for i, count := range selectionSlice {
			if count < expected-errorBound || count > expected+errorBound {
				t.Error(i, count)
			}
		}
	}

	// Remove a few hosts and check that the tree is still in order.
}
