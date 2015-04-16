package hostdb

import (
	"crypto/rand"
	"errors"
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
func (hdbt *hdbTester) uniformTreeVerification(numEntries int) error {
	// Check that the weight of the hostTree is what is expected.
	expectedWeight := types.NewCurrency64(uint64(numEntries)).Mul(hdbt.hostdb.hostTree.hostEntry.weight)
	if hdbt.hostdb.hostTree.weight.Cmp(expectedWeight) != 0 {
		return errors.New("expected weight is incorrect")
	}

	// Check that the length of activeHosts and the count of hostTree are
	// consistent.
	if len(hdbt.hostdb.activeHosts) != numEntries {
		hdbt.t.Error("activeHosts should equal ", numEntries, "equals", len(hdbt.hostdb.activeHosts))
		return errors.New("unexpected number of active hosts")
	}

	// Select many random hosts and do naive statistical analysis on the
	// results.
	if !testing.Short() {
		// Pull a bunch of random hosts and count how many times we pull each
		// host.
		selectionMap := make(map[modules.NetAddress]int)
		expected := 100
		for i := 0; i < expected*numEntries; i++ {
			entry, err := hdbt.hostdb.RandomHost()
			if err != nil {
				return err
			}
			selectionMap[entry.IPAddress] = selectionMap[entry.IPAddress] + 1
		}

		// See if each host was selected enough times.
		errorBound := 64 // Pretty large, but will still detect if something is seriously wrong.
		for i, count := range selectionMap {
			if count < expected-errorBound || count > expected+errorBound {
				hdbt.t.Error(i, count)
				return errors.New("error bound was breached")
			}
		}
	}
	return nil
}

// TestWeightedList inserts and removes nodes in a semi-random manner and
// verifies that the tree stays consistent through the adjustments.
func TestWeightedList(t *testing.T) {
	// Create a hostdb and 3 equal entries to insert.
	hdbt := newHDBTester("TestWeightedList", t)

	// Create a bunch of host entries of equal weight.
	firstInsertions := 64
	for i := 0; i < firstInsertions; i++ {
		entry := hostEntry{
			HostSettings: modules.HostSettings{IPAddress: fakeAddr(uint8(i))},
			weight:       types.NewCurrency64(10),
		}
		hdbt.hostdb.insertNode(&entry)
	}
	err := hdbt.uniformTreeVerification(firstInsertions)
	if err != nil {
		t.Error(err)
	}

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
		err := hdbt.hostdb.RemoveHost(fakeAddr(randInt))
		if err != nil {
			t.Fatal(err)
		}
		removedMap[randInt] = struct{}{}
	}
	err = hdbt.uniformTreeVerification(firstInsertions - removals)
	if err != nil {
		t.Error(err)
	}

	// Do some more insertions.
	secondInsertions := 64
	for i := firstInsertions; i < firstInsertions+secondInsertions; i++ {
		entry := hostEntry{
			HostSettings: modules.HostSettings{IPAddress: fakeAddr(uint8(i))},
			weight:       types.NewCurrency64(10),
		}
		hdbt.hostdb.insertNode(&entry)
	}
	hdbt.uniformTreeVerification(firstInsertions - removals + secondInsertions)
}

// TestVariedWeights runs broad statistical tests on selecting hosts with
// multiple different weights.
func TestVariedWeights(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	hdbt := newHDBTester("TestVariedWeights", t)

	// insert i hosts with the weights 0, 1, ..., i-1. 100e3 selections will be made
	// per weight added to the tree, the total number of selections necessary
	// will be tallied up as hosts are created.
	hostCount := 5
	expectedPerWeight := int(100e3)
	selections := 0
	for i := 0; i < hostCount; i++ {
		entry := hostEntry{
			HostSettings: modules.HostSettings{IPAddress: fakeAddr(uint8(i))},
			weight:       types.NewCurrency64(uint64(i)),
		}
		hdbt.hostdb.insertNode(&entry)
		selections += i * expectedPerWeight
	}

	// Perform many random selections, noting which host was selected each
	// time.
	selectionMap := make(map[string]int)
	for i := 0; i < selections; i++ {
		randEntry, err := hdbt.hostdb.RandomHost()
		if err != nil {
			t.Fatal(err)
		}
		node, exists := hdbt.hostdb.activeHosts[randEntry.IPAddress]
		if !exists {
			t.Fatal("can't find randomly selected node in tree")
		}
		selectionMap[node.weight.String()] += 1
	}

	// Check that each host was selected an expected number of times. An error
	// will be reported if the host of 0 weight is ever selected.
	acceptableError := 0.1
	for weight, timesSelected := range selectionMap {
		intWeight, err := strconv.Atoi(weight)
		if err != nil {
			t.Fatal(err)
		}

		expectedSelected := float64(intWeight * expectedPerWeight)
		if float64(expectedSelected) < float64(timesSelected)*acceptableError || float64(expectedSelected) > float64(timesSelected)/acceptableError {
			t.Error("weighted list does not appear to be selecting in a uniform distribution based on weight")
		}
	}
}

// TestRepeatInsert inserts 2 hosts with the same address.
func TestRepeatInsert(t *testing.T) {
	hdbt := newHDBTester("TestRepeatInsert", t)

	entry1 := hostEntry{
		HostSettings: modules.HostSettings{IPAddress: fakeAddr(0)},
		weight:       types.NewCurrency64(1),
	}
	entry2 := entry1
	hdbt.hostdb.insertNode(&entry1)

	entry2.weight = types.NewCurrency64(100)
	hdbt.hostdb.insertNode(&entry2)
	if len(hdbt.hostdb.activeHosts) != 1 {
		t.Error("insterting the same entry twice should result in only 1 entry in the hostdb")
	}
}
