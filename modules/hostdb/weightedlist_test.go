package hostdb

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// uniformTreeVerification checks that everything makes sense in the tree given
// the number of entries that the tree is supposed to have and also given that
// every entropy has the same weight.
func uniformTreeVerification(hdb *HostDB, numEntries int, t *testing.T) {
	// Check that the weight of the hostTree is what is expected.
	randomHost, err := hdb.RandomHost()
	if err != nil {
		t.Fatal(err)
	}
	expectedWeight := consensus.NewCurrency(uint64(numEntries))
	expectedWeight.Mul(entryWeight(randomHost))
	if hdb.hostTree.weight != expectedWeight {
		t.Error("Expected weight is incorrect")
	}

	// Check that the length of activeHosts and the count of hostTree are
	// consistent.
	if len(hdb.activeHosts) != numEntries {
		t.Error("activeHosts should equal ", numEntries, "equals", len(hdb.activeHosts))
	}

	// Select many random hosts and do naive statistical analysis on the
	// results.
	if !testing.Short() {
		// Pull a bunch of random hosts and count how many times we pull each
		// host.
		selectionMap := make(map[network.Address]int)
		expected := 100
		for i := 0; i < expected*numEntries; i++ {
			entry, err := hdb.RandomHost()
			if err != nil {
				t.Fatal(err)
			}
			selectionMap[entry.IPAddress] = selectionMap[entry.IPAddress] + 1
		}

		// See if each host was selected enough times.
		errorBound := 64 // Pretty large, but will still detect if something is seriously wrong.
		for i, count := range selectionMap {
			if count < expected-errorBound || count > expected+errorBound {
				t.Error(i, count)
			}
		}
	}
}

// TestWeightedList inserts and removes nodes in a semi-random manner and
// verifies that the tree stays consistent through the adjustments.
func TestWeightedList(t *testing.T) {
	// Create a hostdb and 3 equal entries to insert.
	hdb, err := New(consensus.CreateGenesisState(consensus.GenesisTimestamp))
	if err != nil {
		t.Fatal(err)
	}

	// Create a bunch of host entries of equal weight.
	firstInsertions := 64
	for i := 0; i < firstInsertions; i++ {
		var entry modules.HostEntry
		entry.Collateral = consensus.NewCurrency(10)
		entry.Price = consensus.NewCurrency(10)
		entry.Freeze = consensus.NewCurrency(10)
		entry.IPAddress = network.Address(strconv.Itoa(i))
		hdb.Insert(entry)
	}
	uniformTreeVerification(hdb, firstInsertions, t)

	// Remove a few hosts and check that the tree is still in order.
	removals := 12
	// Keep a map of what we've removed so far.
	removedMap := make(map[int]struct{})
	for i := 0; i < removals; i++ {
		// Try numbers until we roll a number that's not been removed yet.
		var randInt int
		for {
			randBig, err := rand.Int(rand.Reader, big.NewInt(int64(firstInsertions)))
			if err != nil {
				t.Fatal(err)
			}
			randInt = int(randBig.Int64())
			_, exists := removedMap[randInt]
			if !exists {
				break
			}
		}

		// Remove the entry and add it to the list of removed entries
		err := hdb.Remove(network.Address(strconv.Itoa(randInt)))
		if err != nil {
			t.Fatal(err)
		}
		removedMap[randInt] = struct{}{}
	}
	uniformTreeVerification(hdb, firstInsertions-removals, t)

	// Do some more insertions.
	secondInsertions := 64
	for i := firstInsertions; i < firstInsertions+secondInsertions; i++ {
		var entry modules.HostEntry
		entry.Collateral = consensus.NewCurrency(10)
		entry.Price = consensus.NewCurrency(10)
		entry.Freeze = consensus.NewCurrency(10)
		entry.IPAddress = network.Address(strconv.Itoa(i))
		hdb.Insert(entry)
	}
	uniformTreeVerification(hdb, firstInsertions-removals+secondInsertions, t)
}
