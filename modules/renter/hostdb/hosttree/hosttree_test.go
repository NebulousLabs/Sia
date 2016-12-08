package hosttree

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

func verifyTree(tree *HostTree, nentries int) error {
	expectedWeight := tree.entry.Weight.Mul64(uint64(nentries))
	if tree.weight.Cmp(expectedWeight) != 0 {
		return fmt.Errorf("expected weight is incorrect: got %v wanted %v\n", tree.weight, expectedWeight)
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
			entries, err := tree.Fetch(1, nil)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return errors.New("no hosts")
			}
			selectionMap[entries[0].PublicKey.String()]++
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
	var removedEntries []*HostEntry
	for {
		if tree.weight.IsZero() {
			break
		}
		randWeight, err := rand.Int(rand.Reader, tree.weight.Big())
		if err != nil {
			break
		}
		node, err := tree.nodeAtWeight(types.NewCurrency(randWeight))
		if err != nil {
			break
		}
		node.remove()
		delete(tree.hosts, node.entry.PublicKey.String())

		// remove the entry from the hostdb so it won't be selected as a
		// repeat
		removedEntries = append(removedEntries, node.entry)
	}
	for _, entry := range removedEntries {
		tree.Insert(entry)
	}
	return nil
}

// makeHostEntry makes a new host entry with a random public key and the weight
// provided to `weight`.
func makeHostEntry(weight types.Currency) *HostEntry {
	pk := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       make([]byte, 32),
	}
	_, err := io.ReadFull(rand.Reader, pk.Key)
	if err != nil {
		panic(err)
	}

	dbe := modules.HostDBEntry{}
	dbe.AcceptingContracts = true
	dbe.PublicKey = pk

	return &HostEntry{
		HostDBEntry: dbe,
		Weight:      weight,
	}
}

func TestHostTree(t *testing.T) {
	tree := New()

	// Create a bunch of host entries of equal weight.
	firstInsertions := 64
	var keys []types.SiaPublicKey
	for i := 0; i < firstInsertions; i++ {
		entry := makeHostEntry(types.NewCurrency64(10))
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
		shouldRemove := func() bool {
			n, err := rand.Int(rand.Reader, big.NewInt(1))
			if err != nil {
				t.Fatal(err)
			}
			if n.Cmp(big.NewInt(0)) == 0 {
				return true
			}
			return false
		}()

		if shouldRemove {
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
		entry := makeHostEntry(types.NewCurrency64(10))
		tree.Insert(entry)
	}
	err = verifyTree(tree, firstInsertions-len(removed)+secondInsertions)
	if err != nil {
		t.Error(err)
	}
}

func TestHostTreeModify(t *testing.T) {
	tree := New()

	treeSize := 100
	var keys []types.SiaPublicKey
	for i := 0; i < treeSize; i++ {
		entry := makeHostEntry(types.NewCurrency64(20))
		keys = append(keys, entry.PublicKey)
		err := tree.Insert(entry)
		if err != nil {
			t.Fatal(err)
		}
	}

	randIndex, err := rand.Int(rand.Reader, big.NewInt(int64(treeSize)))
	if err != nil {
		t.Fatal(err)
	}

	// should fail with a nonexistent key
	err = tree.Modify(&HostEntry{})
	if err != ErrNoSuchHost {
		t.Fatalf("modify should fail with ErrNoSuchHost when provided a nonexistent public key. Got error: %v\n", err)
	}

	targetKey := keys[randIndex.Uint64()]

	oldEntry := tree.hosts[targetKey.String()].entry
	newEntry := makeHostEntry(types.NewCurrency64(30))
	newEntry.AcceptingContracts = false
	newEntry.PublicKey = oldEntry.PublicKey

	err = tree.Modify(newEntry)
	if err != nil {
		t.Fatal(err)
	}

	if tree.hosts[targetKey.String()].entry.AcceptingContracts {
		t.Fatal("modify did not update host entry")
	}
}

// TestVariedWeights runs broad statistical tests on selecting hosts with
// multiple different weights.
func TestVariedWeights(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tree := New()

	// insert i hosts with the weights 0, 1, ..., i-1. 100e3 selections will be made
	// per weight added to the tree, the total number of selections necessary
	// will be tallied up as hosts are created.
	var dbe modules.HostDBEntry
	dbe.AcceptingContracts = true
	hostCount := 5
	expectedPerWeight := int(10e3)
	selections := 0
	for i := 0; i < hostCount; i++ {
		entry := makeHostEntry(types.NewCurrency64(uint64(i)))
		tree.Insert(entry)
		selections += i * expectedPerWeight
	}

	// Perform many random selections, noting which host was selected each
	// time.
	selectionMap := make(map[string]int)
	for i := 0; i < selections; i++ {
		randEntry, err := tree.Fetch(1, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(randEntry) == 0 {
			t.Fatal("no hosts!")
		}
		node, exists := tree.hosts[randEntry[0].PublicKey.String()]
		if !exists {
			t.Fatal("can't find randomly selected node in tree")
		}
		selectionMap[node.entry.Weight.String()]++
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

	tree := New()

	entry1 := makeHostEntry(types.NewCurrency64(1))
	entry2 := entry1

	tree.Insert(entry1)

	entry2.Weight = types.NewCurrency64(100)

	tree.Insert(entry2)
	if len(tree.hosts) != 1 {
		t.Error("insterting the same entry twice should result in only 1 entry")
	}
}

// TestNodeAtWeight tests the nodeAtWeight method.
func TestNodeAtWeight(t *testing.T) {
	// create hostTree
	tree := New()

	entry := makeHostEntry(types.NewCurrency64(100))
	err := tree.Insert(entry)
	if err != nil {
		t.Fatal(err)
	}

	// overweight
	_, err = tree.nodeAtWeight(entry.Weight.Mul64(2))
	if err != ErrWeightTooHeavy {
		t.Errorf("expected %v, got %v", ErrWeightTooHeavy, err)
	}

	h, err := tree.nodeAtWeight(entry.Weight)
	if err != nil {
		t.Error(err)
	} else if h.entry != entry {
		t.Errorf("nodeAtWeight returned wrong node: expected %v, got %v", entry, h.entry)
	}
}

// TestRandomHosts probes the Fetch method.
func TestRandomHosts(t *testing.T) {
	// Create the tree.
	tree := New()

	// Empty.
	hosts, err := tree.Fetch(1, nil)
	if len(hosts) != 0 {
		t.Errorf("empty hostdb returns %v hosts: %v", len(hosts), hosts)
	}
	if err != nil {
		t.Fatal(err)
	}

	// Insert 3 hosts to be selected.
	entry1 := makeHostEntry(types.NewCurrency64(1))
	entry2 := makeHostEntry(types.NewCurrency64(2))
	entry3 := makeHostEntry(types.NewCurrency64(3))

	if err = tree.Insert(entry1); err != nil {
		t.Fatal(err)
	}
	if err = tree.Insert(entry2); err != nil {
		t.Fatal(err)
	}
	if err = tree.Insert(entry3); err != nil {
		t.Fatal(err)
	}

	if len(tree.hosts) != 3 {
		t.Error("wrong number of hosts")
	}
	if tree.weight.Cmp(types.NewCurrency64(6)) != 0 {
		t.Error("unexpected weight at initialization")
		t.Error(tree.weight)
	}

	// Grab 1 random host.
	randHosts, err := tree.Fetch(1, nil)
	if len(randHosts) != 1 {
		t.Error("didn't get 1 hosts")
	}
	if err != nil {
		t.Fatal(err)
	}

	// Grab 2 random hosts.
	randHosts, err = tree.Fetch(2, nil)
	if len(randHosts) != 2 {
		t.Error("didn't get 2 hosts")
	}
	if err != nil {
		t.Fatal(err)
	}
	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() {
		t.Error("doubled up")
	}

	// Grab 3 random hosts.
	randHosts, err = tree.Fetch(3, nil)
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}
	if err != nil {
		t.Fatal(err)
	}

	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() || randHosts[0].PublicKey.String() == randHosts[2].PublicKey.String() || randHosts[1].PublicKey.String() == randHosts[2].PublicKey.String() {
		t.Error("doubled up")
	}

	// Grab 4 random hosts. 3 should be returned.
	randHosts, err = tree.Fetch(4, nil)
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}
	if err != nil {
		t.Fatal(err)
	}

	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() || randHosts[0].PublicKey.String() == randHosts[2].PublicKey.String() || randHosts[1].PublicKey.String() == randHosts[2].PublicKey.String() {
		t.Error("doubled up")
	}

	// Ask for 3 hosts that are not in randHosts. No hosts should be
	// returned.
	uniqueHosts, err := tree.Fetch(3, []types.SiaPublicKey{
		randHosts[0].PublicKey,
		randHosts[1].PublicKey,
		randHosts[2].PublicKey,
	})
	if len(uniqueHosts) != 0 {
		t.Error("didn't get 0 hosts")
	}
	if err != nil {
		t.Fatal(err)
	}

	// Ask for 3 hosts, blacklisting non-existent hosts. 3 should be returned.
	randHosts, err = tree.Fetch(3, []types.SiaPublicKey{{}, {}, {}})
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}
	if err != nil {
		t.Fatal(err)
	}

	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() || randHosts[0].PublicKey.String() == randHosts[2].PublicKey.String() || randHosts[1].PublicKey.String() == randHosts[2].PublicKey.String() {
		t.Error("doubled up")
	}

	// entry4 should not every be returned by RandomHosts because it is not
	// accepting contracts.
	entry4 := makeHostEntry(types.NewCurrency64(4))
	entry4.AcceptingContracts = false
	tree.Insert(entry4)

	// Grab 4 random hosts. 3 should be returned.
	randHosts, err = tree.Fetch(4, nil)
	if len(randHosts) != 3 {
		t.Error("didn't get 3 hosts")
	}
	if err != nil {
		t.Fatal(err)
	}
	if randHosts[0].PublicKey.String() == randHosts[1].PublicKey.String() || randHosts[0].PublicKey.String() == randHosts[2].PublicKey.String() || randHosts[1].PublicKey.String() == randHosts[2].PublicKey.String() {
		t.Error("doubled up")
	}
}
