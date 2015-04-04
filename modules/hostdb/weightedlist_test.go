package hostdb

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// fakeAddr returns a modules.NetAddress to be used in a HostEntry. Such
// addresses are needed in order to satisfy the HostDB's "1 host per IP" rule.
func fakeAddr(n uint8) modules.NetAddress {
	return modules.NetAddress("127.0.0." + strconv.Itoa(int(n)) + ":0")
}

// uniformTreeVerification checks that everything makes sense in the tree given
// the number of entries that the tree is supposed to have and also given that
// every entropy has the same weight.
func (hdbt *HostDBTester) uniformTreeVerification(numEntries int) {
	// Check that the weight of the hostTree is what is expected.
	randomHost, err := hdbt.RandomHost()
	if err != nil {
		hdbt.Fatal(err)
	}
	expectedWeight := types.NewCurrency64(uint64(numEntries)).Mul(entryWeight(randomHost))
	if hdbt.hostTree.weight.Cmp(expectedWeight) != 0 {
		hdbt.Error("Expected weight is incorrect")
	}

	// Check that the length of activeHosts and the count of hostTree are
	// consistent.
	if len(hdbt.activeHosts) != numEntries {
		hdbt.Error("activeHosts should equal ", numEntries, "equals", len(hdbt.activeHosts))
	}

	// Select many random hosts and do naive statistical analysis on the
	// results.
	if !testing.Short() {
		// Pull a bunch of random hosts and count how many times we pull each
		// host.
		selectionMap := make(map[modules.NetAddress]int)
		expected := 100
		for i := 0; i < expected*numEntries; i++ {
			entry, err := hdbt.RandomHost()
			if err != nil {
				hdbt.Fatal(err)
			}
			selectionMap[entry.IPAddress] = selectionMap[entry.IPAddress] + 1
		}

		// See if each host was selected enough times.
		errorBound := 64 // Pretty large, but will still detect if something is seriously wrong.
		for i, count := range selectionMap {
			if count < expected-errorBound || count > expected+errorBound {
				hdbt.Error(i, count)
			}
		}
	}
}

// TestWeightedList inserts and removes nodes in a semi-random manner and
// verifies that the tree stays consistent through the adjustments.
func TestWeightedList(t *testing.T) {
	// Create a hostdb and 3 equal entries to insert.
	hdbt := CreateHostDBTester("TestWeightedList", t)

	// Create a bunch of host entries of equal weight.
	firstInsertions := 64
	for i := 0; i < firstInsertions; i++ {
		entry := new(modules.HostEntry)
		entry.Collateral = types.NewCurrency64(10)
		entry.Price = types.NewCurrency64(10)
		entry.IPAddress = fakeAddr(uint8(i))
		hdbt.insertCompleteHostEntry(entry)
	}
	hdbt.uniformTreeVerification(firstInsertions)

	// Remove a few hosts and check that the tree is still in order.
	removals := 12
	// Keep a map of what we've removed so far.
	removedMap := make(map[uint8]struct{})
	for i := 0; i < removals; i++ {
		// Try numbers until we roll a number that's not been removed yet.
		var randInt uint8
		for {
			randBig, err := rand.Int(rand.Reader, big.NewInt(int64(firstInsertions)))
			if err != nil {
				t.Fatal(err)
			}
			randInt = uint8(randBig.Int64())
			_, exists := removedMap[randInt]
			if !exists {
				break
			}
		}

		// Remove the entry and add it to the list of removed entries
		err := hdbt.Remove(fakeAddr(randInt))
		if err != nil {
			t.Fatal(err)
		}
		removedMap[randInt] = struct{}{}
	}
	hdbt.uniformTreeVerification(firstInsertions - removals)

	// Do some more insertions.
	secondInsertions := 64
	for i := firstInsertions; i < firstInsertions+secondInsertions; i++ {
		entry := new(modules.HostEntry)
		entry.Collateral = types.NewCurrency64(10)
		entry.Price = types.NewCurrency64(10)
		entry.IPAddress = fakeAddr(uint8(i))
		hdbt.insertCompleteHostEntry(entry)
	}
	hdbt.uniformTreeVerification(firstInsertions - removals + secondInsertions)
}
