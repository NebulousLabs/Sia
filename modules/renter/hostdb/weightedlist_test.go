package hostdb

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
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
func uniformTreeVerification(hdb *HostDB, numEntries int) error {
	// Check that the weight of the hostTree is what is expected.
	expectedWeight := types.NewCurrency64(uint64(numEntries)).Mul(hdb.hostTree.hostEntry.weight)
	if hdb.hostTree.weight.Cmp(expectedWeight) != 0 {
		return errors.New("expected weight is incorrect")
	}

	// Check that the length of activeHosts and the count of hostTree are
	// consistent.
	if len(hdb.activeHosts) != numEntries {
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
			entries := hdb.RandomHosts(1)
			if len(entries) == 0 {
				return errors.New("no hosts!")
			}
			selectionMap[entries[0].IPAddress] = selectionMap[entries[0].IPAddress] + 1
		}

		// See if each host was selected enough times.
		errorBound := 64 // Pretty large, but will still detect if something is seriously wrong.
		for _, count := range selectionMap {
			if count < expected-errorBound || count > expected+errorBound {
				return errors.New("error bound was breached")
			}
		}
	}

	// Try removing an re-adding all hosts.
	var removedEntries []*hostEntry
	for {
		if hdb.hostTree.weight.IsZero() {
			break
		}
		randWeight, err := rand.Int(rand.Reader, hdb.hostTree.weight.Big())
		if err != nil {
			break
		}
		node, err := hdb.hostTree.nodeAtWeight(types.NewCurrency(randWeight))
		if err != nil {
			break
		}
		node.removeNode()
		delete(hdb.activeHosts, node.hostEntry.IPAddress)

		// remove the entry from the hostdb so it won't be selected as a
		// repeat.
		removedEntries = append(removedEntries, node.hostEntry)
	}
	for _, entry := range removedEntries {
		hdb.insertNode(entry)
	}
	return nil
}

// TestWeightedList inserts and removes nodes in a semi-random manner and
// verifies that the tree stays consistent through the adjustments.
func TestWeightedList(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a hostdb and 3 equal entries to insert.
	hdb := &HostDB{
		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Create a bunch of host entries of equal weight.
	firstInsertions := 64
	for i := 0; i < firstInsertions; i++ {
		entry := hostEntry{
			HostSettings: modules.HostSettings{IPAddress: fakeAddr(uint8(i))},
			weight:       types.NewCurrency64(10),
		}
		hdb.insertNode(&entry)
	}
	err := uniformTreeVerification(hdb, firstInsertions)
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
		err := hdb.RemoveHost(fakeAddr(randInt))
		if err != nil {
			t.Fatal(err)
		}
		removedMap[randInt] = struct{}{}
	}
	err = uniformTreeVerification(hdb, firstInsertions-removals)
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
		hdb.insertNode(&entry)
	}
	err = uniformTreeVerification(hdb, firstInsertions-removals+secondInsertions)
	if err != nil {
		t.Error(err)
	}
}

// TestVariedWeights runs broad statistical tests on selecting hosts with
// multiple different weights.
func TestVariedWeights(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := &HostDB{
		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// insert i hosts with the weights 0, 1, ..., i-1. 100e3 selections will be made
	// per weight added to the tree, the total number of selections necessary
	// will be tallied up as hosts are created.
	hostCount := 5
	expectedPerWeight := int(10e3)
	selections := 0
	for i := 0; i < hostCount; i++ {
		entry := hostEntry{
			HostSettings: modules.HostSettings{IPAddress: fakeAddr(uint8(i))},
			weight:       types.NewCurrency64(uint64(i)),
		}
		hdb.insertNode(&entry)
		selections += i * expectedPerWeight
	}

	// Perform many random selections, noting which host was selected each
	// time.
	selectionMap := make(map[string]int)
	for i := 0; i < selections; i++ {
		randEntry := hdb.RandomHosts(1)
		if len(randEntry) == 0 {
			t.Fatal("no hosts!")
		}
		node, exists := hdb.activeHosts[randEntry[0].IPAddress]
		if !exists {
			t.Fatal("can't find randomly selected node in tree")
		}
		selectionMap[node.hostEntry.weight.String()] += 1
	}

	// Check that each host was selected an expected number of times. An error
	// will be reported if the host of 0 weight is ever selected.
	acceptableError := 0.2
	for weight, timesSelected := range selectionMap {
		intWeight, err := strconv.Atoi(weight)
		if err != nil {
			t.Fatal(err)
		}

		expectedSelected := float64(intWeight * expectedPerWeight)
		if float64(expectedSelected)*acceptableError > float64(timesSelected) || float64(expectedSelected)/acceptableError < float64(timesSelected) {
			t.Error("weighted list not selecting in a uniform distribution based on weight")
			t.Error(expectedSelected)
			t.Error(timesSelected)
		}
	}
}

// TestRepeatInsert inserts 2 hosts with the same address.
func TestRepeatInsert(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := &HostDB{
		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	entry1 := hostEntry{
		HostSettings: modules.HostSettings{IPAddress: fakeAddr(0)},
		weight:       types.NewCurrency64(1),
	}
	entry2 := entry1
	hdb.insertNode(&entry1)

	entry2.weight = types.NewCurrency64(100)
	hdb.insertNode(&entry2)
	if len(hdb.activeHosts) != 1 {
		t.Error("insterting the same entry twice should result in only 1 entry in the hostdb")
	}
}

// TestRandomHosts probles the RandomHosts function.
func TestRandomHosts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdb := &HostDB{
		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Insert 3 hosts to be selected.
	entry1 := hostEntry{
		HostSettings: modules.HostSettings{IPAddress: fakeAddr(1)},
		weight:       types.NewCurrency64(1),
	}
	entry2 := hostEntry{
		HostSettings: modules.HostSettings{IPAddress: fakeAddr(2)},
		weight:       types.NewCurrency64(2),
	}
	entry3 := hostEntry{
		HostSettings: modules.HostSettings{IPAddress: fakeAddr(3)},
		weight:       types.NewCurrency64(3),
	}
	hdb.insertNode(&entry1)
	hdb.insertNode(&entry2)
	hdb.insertNode(&entry3)

	if len(hdb.activeHosts) != 3 {
		t.Error("wrong number of hosts")
	}
	if hdb.hostTree.weight.Cmp(types.NewCurrency64(6)) != 0 {
		t.Error("unexpected weight at initialization")
		t.Error(hdb.hostTree.weight)
	}

	// Grab 1 random host.
	randHosts := hdb.RandomHosts(1)
	if len(randHosts) != 1 {
		t.Error("didn't get 1 hosts")
	}
	if len(hdb.activeHosts) != 3 {
		t.Error("wrong number of hosts")
	}
	if hdb.hostTree.weight.Cmp(types.NewCurrency64(6)) != 0 {
		t.Error("unexpected weight at initialization")
	}

	// Grab 2 random hosts.
	randHosts = hdb.RandomHosts(2)
	if len(randHosts) != 2 {
		t.Error("didn't get 2 hosts")
	}
	if len(hdb.activeHosts) != 3 {
		t.Error("wrong number of hosts")
	}
	if hdb.hostTree.weight.Cmp(types.NewCurrency64(6)) != 0 {
		t.Error("unexpected weight at initialization")
	}
	if randHosts[0].IPAddress == randHosts[1].IPAddress {
		t.Error("doubled up")
	}

	// Grab 3 random hosts.
	randHosts = hdb.RandomHosts(3)
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}
	if len(hdb.activeHosts) != 3 {
		t.Error("wrong number of hosts")
	}
	if hdb.hostTree.weight.Cmp(types.NewCurrency64(6)) != 0 {
		t.Error("unexpected weight at initialization")
	}
	if randHosts[0].IPAddress == randHosts[1].IPAddress || randHosts[0].IPAddress == randHosts[2].IPAddress || randHosts[1].IPAddress == randHosts[2].IPAddress {
		t.Error("doubled up")
	}

	// Grab 4 random hosts. 3 should be returned.
	randHosts = hdb.RandomHosts(4)
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}
	if len(hdb.activeHosts) != 3 {
		t.Error("wrong number of hosts")
	}
	if hdb.hostTree.weight.Cmp(types.NewCurrency64(6)) != 0 {
		t.Error("unexpected weight at initialization")
	}
	if randHosts[0].IPAddress == randHosts[1].IPAddress || randHosts[0].IPAddress == randHosts[2].IPAddress || randHosts[1].IPAddress == randHosts[2].IPAddress {
		t.Error("doubled up")
	}
}
