package hosttree

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/modules"
	siasync "gitlab.com/NebulousLabs/Sia/sync"
	"gitlab.com/NebulousLabs/Sia/types"
	"gitlab.com/NebulousLabs/fastrand"
)

func verifyTree(tree *HostTree, nentries int) error {
	expectedWeight := tree.root.entry.weight.Mul64(uint64(nentries))
	if tree.root.weight.Cmp(expectedWeight) != 0 {
		return fmt.Errorf("expected weight is incorrect: got %v wanted %v\n", tree.root.weight, expectedWeight)
	}

	// Check that the length of activeHosts and the count of hostTree are
	// consistent.
	if len(tree.hosts) != nentries {
		return fmt.Errorf("unexpected number of hosts: got %v wanted %v\n", len(tree.hosts), nentries)
	}

	// Select many random hosts and do naive statistical analysis on the
	// results.
	if !testing.Short() {
		// Pull a bunch of random hosts and count how many times we pull each
		// host.
		selectionMap := make(map[string]int)
		expected := 100
		for i := 0; i < expected*nentries; i++ {
			entries := tree.SelectRandom(1, nil)
			if len(entries) == 0 {
				return errors.New("no hosts")
			}
			selectionMap[string(entries[0].PublicKey.Key)]++
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
		if tree.root.weight.IsZero() {
			break
		}
		randWeight := fastrand.BigIntn(tree.root.weight.Big())
		node := tree.root.nodeAtWeight(types.NewCurrency(randWeight))
		node.remove()
		delete(tree.hosts, string(node.entry.PublicKey.Key))

		// remove the entry from the hostdb so it won't be selected as a
		// repeat
		removedEntries = append(removedEntries, node.entry)
	}
	for _, entry := range removedEntries {
		tree.Insert(entry.HostDBEntry)
	}
	return nil
}

// makeHostDBEntry makes a new host entry with a random public key and the weight
// provided to `weight`.
func makeHostDBEntry() modules.HostDBEntry {
	dbe := modules.HostDBEntry{}

	_, pk := crypto.GenerateKeyPair()
	dbe.AcceptingContracts = true
	dbe.PublicKey = types.Ed25519PublicKey(pk)
	dbe.ScanHistory = modules.HostDBScans{{
		Timestamp: time.Now(),
		Success:   true,
	}}

	return dbe
}

func TestHostTree(t *testing.T) {
	tree := New(func(hdbe modules.HostDBEntry) types.Currency {
		return types.NewCurrency64(20)
	})

	// Create a bunch of host entries of equal weight.
	firstInsertions := 64
	var keys []types.SiaPublicKey
	for i := 0; i < firstInsertions; i++ {
		entry := makeHostDBEntry()
		keys = append(keys, entry.PublicKey)
		err := tree.Insert(entry)
		if err != nil {
			t.Fatal(err)
		}
	}
	err := verifyTree(tree, firstInsertions)
	if err != nil {
		t.Error(err)
	}

	var removed []types.SiaPublicKey
	// Randomly remove hosts from the tree and check that it is still in order.
	for _, key := range keys {
		if fastrand.Intn(1) == 0 {
			err := tree.Remove(key)
			if err != nil {
				t.Fatal(err)
			}
			removed = append(removed, key)
		}
	}

	err = verifyTree(tree, firstInsertions-len(removed))
	if err != nil {
		t.Error(err)
	}

	// Do some more insertions.
	secondInsertions := 64
	for i := firstInsertions; i < firstInsertions+secondInsertions; i++ {
		entry := makeHostDBEntry()
		tree.Insert(entry)
	}
	err = verifyTree(tree, firstInsertions-len(removed)+secondInsertions)
	if err != nil {
		t.Error(err)
	}
}

// Verify that inserting, fetching, deleting, and modifying in parallel from
// the hosttree does not cause inconsistency.
func TestHostTreeParallel(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tree := New(func(dbe modules.HostDBEntry) types.Currency {
		return types.NewCurrency64(10)
	})

	// spin up 100 goroutines all randomly inserting, removing, modifying, and
	// fetching nodes from the tree.
	var tg siasync.ThreadGroup
	nthreads := 100
	nelements := 0
	var mu sync.Mutex
	for i := 0; i < nthreads; i++ {
		go func() {
			tg.Add()
			defer tg.Done()

			inserted := make(map[string]modules.HostDBEntry)
			randEntry := func() *modules.HostDBEntry {
				for _, entry := range inserted {
					return &entry
				}
				return nil
			}

			for {
				select {
				case <-tg.StopChan():
					return
				default:
					switch fastrand.Intn(4) {

					// INSERT
					case 0:
						entry := makeHostDBEntry()
						err := tree.Insert(entry)
						if err != nil {
							t.Error(err)
						}
						inserted[string(entry.PublicKey.Key)] = entry

						mu.Lock()
						nelements++
						mu.Unlock()

					// REMOVE
					case 1:
						entry := randEntry()
						if entry == nil {
							continue
						}
						err := tree.Remove(entry.PublicKey)
						if err != nil {
							t.Error(err)
						}
						delete(inserted, string(entry.PublicKey.Key))

						mu.Lock()
						nelements--
						mu.Unlock()

					// MODIFY
					case 2:
						entry := randEntry()
						if entry == nil {
							continue
						}
						newentry := makeHostDBEntry()
						newentry.PublicKey = entry.PublicKey
						newentry.NetAddress = "127.0.0.1:31337"

						err := tree.Modify(newentry)
						if err != nil {
							t.Error(err)
						}
						inserted[string(entry.PublicKey.Key)] = newentry

					// FETCH
					case 3:
						tree.SelectRandom(3, nil)
					}
				}
			}
		}()
	}

	// let these goroutines operate on the tree for 5 seconds
	time.Sleep(time.Second * 5)

	// stop the goroutines
	tg.Stop()

	// verify the consistency of the tree
	err := verifyTree(tree, int(nelements))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHostTreeModify(t *testing.T) {
	tree := New(func(dbe modules.HostDBEntry) types.Currency {
		return types.NewCurrency64(10)
	})

	treeSize := 100
	var keys []types.SiaPublicKey
	for i := 0; i < treeSize; i++ {
		entry := makeHostDBEntry()
		keys = append(keys, entry.PublicKey)
		err := tree.Insert(entry)
		if err != nil {
			t.Fatal(err)
		}
	}

	// should fail with a nonexistent key
	err := tree.Modify(modules.HostDBEntry{})
	if err != errNoSuchHost {
		t.Fatalf("modify should fail with ErrNoSuchHost when provided a nonexistent public key. Got error: %v\n", err)
	}

	targetKey := keys[fastrand.Intn(treeSize)]

	oldEntry := tree.hosts[string(targetKey.Key)].entry
	newEntry := makeHostDBEntry()
	newEntry.AcceptingContracts = false
	newEntry.PublicKey = oldEntry.PublicKey

	err = tree.Modify(newEntry)
	if err != nil {
		t.Fatal(err)
	}

	if tree.hosts[string(targetKey.Key)].entry.AcceptingContracts {
		t.Fatal("modify did not update host entry")
	}
}

// TestVariedWeights runs broad statistical tests on selecting hosts with
// multiple different weights.
func TestVariedWeights(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// insert i hosts with the weights 0, 1, ..., i-1. 100e3 selections will be made
	// per weight added to the tree, the total number of selections necessary
	// will be tallied up as hosts are created.
	i := 0

	tree := New(func(dbe modules.HostDBEntry) types.Currency {
		return types.NewCurrency64(uint64(i))
	})

	hostCount := 5
	expectedPerWeight := int(10e3)
	selections := 0
	for i = 0; i < hostCount; i++ {
		entry := makeHostDBEntry()
		tree.Insert(entry)
		selections += i * expectedPerWeight
	}

	// Perform many random selections, noting which host was selected each
	// time.
	selectionMap := make(map[string]int)
	for i := 0; i < selections; i++ {
		randEntry := tree.SelectRandom(1, nil)
		if len(randEntry) == 0 {
			t.Fatal("no hosts!")
		}
		node, exists := tree.hosts[string(randEntry[0].PublicKey.Key)]
		if !exists {
			t.Fatal("can't find randomly selected node in tree")
		}
		selectionMap[node.entry.weight.String()]++
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

// TestRepeatInsert inserts 2 hosts with the same public key.
func TestRepeatInsert(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tree := New(func(dbe modules.HostDBEntry) types.Currency {
		return types.NewCurrency64(10)
	})

	entry1 := makeHostDBEntry()
	entry2 := entry1

	tree.Insert(entry1)
	tree.Insert(entry2)
	if len(tree.hosts) != 1 {
		t.Error("insterting the same entry twice should result in only 1 entry")
	}
}

// TestNodeAtWeight tests the nodeAtWeight method.
func TestNodeAtWeight(t *testing.T) {
	weight := types.NewCurrency64(10)
	// create hostTree
	tree := New(func(dbe modules.HostDBEntry) types.Currency {
		return weight
	})

	entry := makeHostDBEntry()
	err := tree.Insert(entry)
	if err != nil {
		t.Fatal(err)
	}

	h := tree.root.nodeAtWeight(weight)
	if string(h.entry.HostDBEntry.PublicKey.Key) != string(entry.PublicKey.Key) {
		t.Errorf("nodeAtWeight returned wrong node: expected %v, got %v", entry, h.entry)
	}
}

// TestRandomHosts probes the SelectRandom method.
func TestRandomHosts(t *testing.T) {
	calls := 0
	// Create the tree.
	tree := New(func(dbe modules.HostDBEntry) types.Currency {
		calls++
		return types.NewCurrency64(uint64(calls))
	})

	// Empty.
	hosts := tree.SelectRandom(1, nil)
	if len(hosts) != 0 {
		t.Errorf("empty hostdb returns %v hosts: %v", len(hosts), hosts)
	}

	// Insert 3 hosts to be selected.
	entry1 := makeHostDBEntry()
	entry2 := makeHostDBEntry()
	entry3 := makeHostDBEntry()

	if err := tree.Insert(entry1); err != nil {
		t.Fatal(err)
	}
	if err := tree.Insert(entry2); err != nil {
		t.Fatal(err)
	}
	if err := tree.Insert(entry3); err != nil {
		t.Fatal(err)
	}

	if len(tree.hosts) != 3 {
		t.Error("wrong number of hosts")
	}
	if tree.root.weight.Cmp(types.NewCurrency64(6)) != 0 {
		t.Error("unexpected weight at initialization")
		t.Error(tree.root.weight)
	}

	// Grab 1 random host.
	randHosts := tree.SelectRandom(1, nil)
	if len(randHosts) != 1 {
		t.Error("didn't get 1 hosts")
	}

	// Grab 2 random hosts.
	randHosts = tree.SelectRandom(2, nil)
	if len(randHosts) != 2 {
		t.Error("didn't get 2 hosts")
	}
	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() {
		t.Error("doubled up")
	}

	// Grab 3 random hosts.
	randHosts = tree.SelectRandom(3, nil)
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}

	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() || randHosts[0].PublicKey.String() == randHosts[2].PublicKey.String() || randHosts[1].PublicKey.String() == randHosts[2].PublicKey.String() {
		t.Error("doubled up")
	}

	// Grab 4 random hosts. 3 should be returned.
	randHosts = tree.SelectRandom(4, nil)
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}

	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() || randHosts[0].PublicKey.String() == randHosts[2].PublicKey.String() || randHosts[1].PublicKey.String() == randHosts[2].PublicKey.String() {
		t.Error("doubled up")
	}

	// Ask for 3 hosts that are not in randHosts. No hosts should be
	// returned.
	uniqueHosts := tree.SelectRandom(3, []types.SiaPublicKey{
		randHosts[0].PublicKey,
		randHosts[1].PublicKey,
		randHosts[2].PublicKey,
	})
	if len(uniqueHosts) != 0 {
		t.Error("didn't get 0 hosts")
	}

	// Ask for 3 hosts, blacklisting non-existent hosts. 3 should be returned.
	randHosts = tree.SelectRandom(3, []types.SiaPublicKey{{}, {}, {}})
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}

	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() || randHosts[0].PublicKey.String() == randHosts[2].PublicKey.String() || randHosts[1].PublicKey.String() == randHosts[2].PublicKey.String() {
		t.Error("doubled up")
	}
}
