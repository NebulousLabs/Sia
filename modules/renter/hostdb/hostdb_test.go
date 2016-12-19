package hostdb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb/hosttree"
	"github.com/NebulousLabs/Sia/persist"
)

// bareHostDB returns a HostDB with its fields initialized, but without any
// dependencies or scanning threads. It is only intended for use in unit tests.
func bareHostDB() *HostDB {
	hdb := &HostDB{
		log: persist.NewLogger(ioutil.Discard),

		activeHosts: make(map[modules.NetAddress]*hostEntry),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),
	}
	hdb.hostTree = hosttree.New(hdb.calculateHostWeight())
	return hdb
}

// newStub is used to test the New function. It implements all of the hostdb's
// dependencies.
type newStub struct{}

// consensus set stubs
func (newStub) ConsensusSetSubscribe(modules.ConsensusSetSubscriber, modules.ConsensusChangeID) error {
	return nil
}

// TestNew tests the New function.
func TestNew(t *testing.T) {
	// Using a stub implementation of the dependencies is fine, as long as its
	// non-nil.
	var stub newStub
	dir := build.TempDir("hostdb", "TestNew")

	// Sane values.
	_, err := New(stub, dir)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Nil consensus set.
	_, err = New(nil, dir)
	if err != errNilCS {
		t.Fatalf("expected %v, got %v", errNilCS, err)
	}

	// Bad persistDir.
	_, err = New(stub, "")
	if !os.IsNotExist(err) {
		t.Fatalf("expected invalid directory, got %v", err)
	}
}

// TestRandomHosts tests the hostdb's exported RandomHosts method.
func TestRandomHosts(t *testing.T) {
	hdb := bareHostDB()

	var entries []modules.HostDBEntry
	nentries := 10
	for i := 0; i < nentries; i++ {
		entry := makeHostDBEntry()
		entry.NetAddress = fakeAddr(uint8(i))
		entries = append(entries, entry)
		hdb.activeHosts[entry.NetAddress] = &hostEntry{HostDBEntry: entry}
		hdb.hostTree.Insert(entry)
	}

	hosts := hdb.RandomHosts(nentries, nil)
	if len(hosts) != nentries {
		t.Fatalf("RandomHosts returned fewer entries than expected. got %v wanted %v\n", len(hosts), nentries)
	}

	hosts = hdb.RandomHosts(nentries/2, nil)
	if len(hosts) != nentries/2 {
		t.Fatalf("RandomHosts returned fewer entries than expected. got %v wanted %v\n", len(hosts), nentries/2)
	}

	// ignore every other entry and verify that the exclude list worked correctly
	var exclude []modules.HostDBEntry
	for idx, entry := range entries {
		if idx%2 == 0 {
			exclude = append(exclude, entry)
		}
	}

	var exclusionAddresses []modules.NetAddress
	for _, exclusionHost := range exclude {
		exclusionAddresses = append(exclusionAddresses, exclusionHost.NetAddress)
	}

	hosts = hdb.RandomHosts(nentries, exclusionAddresses)
	if len(hosts) != len(entries)/2 {
		t.Fatalf("hosts had wrong length after passing exclusion slice. got %v wanted %v\n", len(hosts), len(entries)/2)
	}
	for _, host := range hosts {
		for _, excluded := range exclude {
			if string(host.PublicKey.Key) == string(excluded.PublicKey.Key) {
				t.Fatal("RandomHosts returned an excluded host!")
			}
		}
	}
}
