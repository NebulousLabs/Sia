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
}
