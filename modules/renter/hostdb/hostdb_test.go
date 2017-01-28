package hostdb

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb/hosttree"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

type hdbTester struct {
	cs      modules.ConsensusSet
	gateway modules.Gateway

	hdb *HostDB

	persistDir string
}

// bareHostDB returns a HostDB with its fields initialized, but without any
// dependencies or scanning threads. It is only intended for use in unit tests.
func bareHostDB() *HostDB {
	hdb := &HostDB{
		log: persist.NewLogger(ioutil.Discard),

		scanPool: make(chan modules.HostDBEntry, scanPoolSize),
	}
	hdb.hostTree = hosttree.New(hdb.calculateHostWeight)
	return hdb
}

// newStub is used to test the New function. It implements all of the hostdb's
// dependencies.
type newStub struct {
	*gateway.Gateway
}

// consensus set stubs
func (newStub) ConsensusSetSubscribe(modules.ConsensusSetSubscriber, modules.ConsensusChangeID) error {
	return nil
}

// newHDBTester returns a tester object wrapping a HostDB and some extra
// information for testing.
func newHDBTester(name string) (*hdbTester, error) {
	if testing.Short() {
		panic("should not be calling newHDBTester during short tests")
	}
	testDir := build.TempDir("HostDB", name)

	g, err := gateway.New("localhost:0", false, filepath.Join(testDir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testDir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	hdb, err := New(g, cs, filepath.Join(testDir, modules.RenterDir))
	if err != nil {
		return nil, err
	}

	hdbt := &hdbTester{
		cs:      cs,
		gateway: g,

		hdb: hdb,

		persistDir: name,
	}
	return hdbt, nil
}

// TestNew tests the New function.
func TestNew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir("HostDB", "TestNew")
	g, err := gateway.New("localhost:0", false, filepath.Join(testDir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testDir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Vanilla HDB, nothing should go wrong.
	hdbName := filepath.Join(testDir, modules.RenterDir)
	_, err = New(g, cs, hdbName+"1")
	if err != nil {
		t.Fatal(err)
	}
	// Nil gateway.
	_, err = New(nil, cs, hdbName+"2")
	if err != errNilGateway {
		t.Fatalf("expected %v, got %v", errNilGateway, err)
	}
	// Nil consensus set.
	_, err = New(g, nil, hdbName+"3")
	if err != errNilCS {
		t.Fatalf("expected %v, got %v", errNilCS, err)
	}
	// Bad persistDir.
	_, err = New(g, cs, "")
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

	var exclusionAddresses []types.SiaPublicKey
	for _, exclusionHost := range exclude {
		exclusionAddresses = append(exclusionAddresses, exclusionHost.PublicKey)
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

// TestRemoveNonexistingHostFromHostTree checks that the host tree interface
// correctly responds to having a nonexisting host removed from the host tree.
func TestRemoveNonexistingHostFromHostTree(t *testing.T) {
	// Create a host tree.
	hdb := bareHostDB()
	ht := hosttree.New(hdb.calculateHostWeight)

	// Remove a host that doesn't exist from the tree.
	err := ht.Remove(types.SiaPublicKey{})
	if err == nil {
		t.Fatal("There should be an error, but not a panic:", err)
	}
}
