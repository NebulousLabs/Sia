package hostdb

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// repeatCheckHelper recursively goes through nodes in the host map and adds
// them to the repeat maps.
func (hn *hostNode) repeatCheckHelper(ipMap, pkMap map[string]struct{}) error {
	ipStr := string(hn.hostEntry.NetAddress)
	pkStr := crypto.HashObject(hn.hostEntry.PublicKey).String()
	_, exists := ipMap[ipStr]
	if exists && hn.taken {
		return errors.New("found a duplicate ip address in the hostdb: " + ipStr)
	}
	_, exists = pkMap[pkStr]
	if exists && hn.taken {
		return errors.New("found a duplicate pubkey in the hostdb: " + ipStr + " " + pkStr)
	}
	if hn.taken {
		ipMap[ipStr] = struct{}{}
		pkMap[pkStr] = struct{}{}
	}

	if hn.left != nil {
		err := hn.left.repeatCheckHelper(ipMap, pkMap)
		if err != nil {
			return err
		}
	}
	if hn.right != nil {
		err := hn.right.repeatCheckHelper(ipMap, pkMap)
		if err != nil {
			return err
		}
	}
	return nil
}

// repeatCheck will return an error if there are multiple hosts in the host
// tree with the same IP address or same public key.
func repeatCheck(hn *hostNode) error {
	if hn == nil {
		return nil
	}

	ipMap := make(map[string]struct{})
	pkMap := make(map[string]struct{})
	err := hn.repeatCheckHelper(ipMap, pkMap)
	if err != nil {
		return err
	}
	return nil
}

// uniformTreeVerification checks that everything makes sense in the tree given
// the number of entries that the tree is supposed to have and also given that
// every entropy has the same weight.
func uniformTreeVerification(hdb *HostDB, numEntries int) error {
	// Check that the weight of the hostTree is what is expected.
	expectedWeight := hdb.hostTree.hostEntry.Weight.Mul64(uint64(numEntries))
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
			entries := hdb.RandomHosts(1, nil)
			if len(entries) == 0 {
				return errors.New("no hosts")
			}
			selectionMap[entries[0].NetAddress]++
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
		delete(hdb.activeHosts, node.hostEntry.NetAddress)

		// remove the entry from the hostdb so it won't be selected as a
		// repeat.
		removedEntries = append(removedEntries, node.hostEntry)
	}
	for _, entry := range removedEntries {
		hdb.insertNode(entry)
	}
	return nil
}
