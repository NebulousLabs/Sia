package hostdb

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb/hosttree"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// hdbTester contains a hostdb and all dependencies.
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

		scanPool: make(chan modules.HostDBEntry),
	}
	hdb.hostTree = hosttree.New(hdb.calculateHostWeight)
	return hdb
}

// makeHostDBEntry makes a new host entry with a random public key
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

// newHDBTester returns a tester object wrapping a HostDB and some extra
// information for testing.
func newHDBTester(name string) (*hdbTester, error) {
	return newHDBTesterDeps(name, prodDependencies{})
}

// newHDBTesterDeps returns a tester object wrapping a HostDB and some extra
// information for testing, using the provided dependencies for the hostdb.
func newHDBTesterDeps(name string, deps dependencies) (*hdbTester, error) {
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
	hdb, err := newHostDB(g, cs, filepath.Join(testDir, modules.RenterDir), deps)
	if err != nil {
		return nil, err
	}

	hdbt := &hdbTester{
		cs:      cs,
		gateway: g,

		hdb: hdb,

		persistDir: testDir,
	}
	return hdbt, nil
}

// TestAverageContractPrice tests the AverageContractPrice method, which also depends on the
// randomHosts method.
func TestAverageContractPrice(t *testing.T) {
	hdb := bareHostDB()

	// empty
	if avg := hdb.AverageContractPrice(); !avg.IsZero() {
		t.Error("average of empty hostdb should be zero:", avg)
	}

	// with one host
	h1 := makeHostDBEntry()
	h1.ContractPrice = types.NewCurrency64(100)
	hdb.hostTree.Insert(h1)
	if avg := hdb.AverageContractPrice(); avg.Cmp(h1.ContractPrice) != 0 {
		t.Error("average of one host should be that host's price:", avg)
	}

	// with two hosts
	h2 := makeHostDBEntry()
	h2.ContractPrice = types.NewCurrency64(300)
	hdb.hostTree.Insert(h2)
	if avg := hdb.AverageContractPrice(); avg.Cmp64(200) != 0 {
		t.Error("average of two hosts should be their sum/2:", avg)
	}
}

// TestNew tests the New function.
func TestNew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir("HostDB", t.Name())
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

// quitAfterLoadDeps will quit startup in newHostDB
type disableScanLoopDeps struct {
	prodDependencies
}

// Send a disrupt signal to the quitAfterLoad codebreak.
func (disableScanLoopDeps) disrupt(s string) bool {
	if s == "disableScanLoop" {
		return true
	}
	return false
}

// TestRandomHosts tests the hostdb's exported RandomHosts method.
func TestRandomHosts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdbt, err := newHDBTesterDeps(t.Name(), disableScanLoopDeps{})
	if err != nil {
		t.Fatal(err)
	}

	entries := make(map[string]modules.HostDBEntry)
	nEntries := int(1e3)
	for i := 0; i < nEntries; i++ {
		entry := makeHostDBEntry()
		entries[string(entry.PublicKey.Key)] = entry
		err := hdbt.hdb.hostTree.Insert(entry)
		if err != nil {
			t.Error(err)
		}
	}

	// Check that all hosts can be queried.
	for i := 0; i < 25; i++ {
		hosts := hdbt.hdb.RandomHosts(nEntries, nil)
		if len(hosts) != nEntries {
			t.Errorf("RandomHosts returned few entries. got %v wanted %v\n", len(hosts), nEntries)
		}
		dupCheck := make(map[string]modules.HostDBEntry)
		for _, host := range hosts {
			_, exists := entries[string(host.PublicKey.Key)]
			if !exists {
				t.Error("hostdb returning host that doesn't exist.")
			}
			_, exists = dupCheck[string(host.PublicKey.Key)]
			if exists {
				t.Error("RandomHosts returning duplicates")
			}
			dupCheck[string(host.PublicKey.Key)] = host
		}
	}

	// Base case, fill out a map exposing hosts from a single RH query.
	dupCheck1 := make(map[string]modules.HostDBEntry)
	hosts := hdbt.hdb.RandomHosts(nEntries/2, nil)
	if len(hosts) != nEntries/2 {
		t.Fatalf("RandomHosts returned few entries. got %v wanted %v\n", len(hosts), nEntries/2)
	}
	for _, host := range hosts {
		_, exists := entries[string(host.PublicKey.Key)]
		if !exists {
			t.Error("hostdb returning host that doesn't exist.")
		}
		_, exists = dupCheck1[string(host.PublicKey.Key)]
		if exists {
			t.Error("RandomHosts returning duplicates")
		}
		dupCheck1[string(host.PublicKey.Key)] = host
	}

	// Iterative case. Check that every time you query for random hosts, you
	// get different responses.
	for i := 0; i < 10; i++ {
		dupCheck2 := make(map[string]modules.HostDBEntry)
		var overlap, disjoint bool
		hosts = hdbt.hdb.RandomHosts(nEntries/2, nil)
		if len(hosts) != nEntries/2 {
			t.Fatalf("RandomHosts returned few entries. got %v wanted %v\n", len(hosts), nEntries/2)
		}
		for _, host := range hosts {
			_, exists := entries[string(host.PublicKey.Key)]
			if !exists {
				t.Error("hostdb returning host that doesn't exist.")
			}
			_, exists = dupCheck2[string(host.PublicKey.Key)]
			if exists {
				t.Error("RandomHosts returning duplicates")
			}
			_, exists = dupCheck1[string(host.PublicKey.Key)]
			if exists {
				overlap = true
			} else {
				disjoint = true
			}
			dupCheck2[string(host.PublicKey.Key)] = host

		}
		if !overlap || !disjoint {
			t.Error("Random hosts does not seem to be random")
		}
		dupCheck1 = dupCheck2
	}

	// Try exclude list by excluding every host except for the last one, and
	// doing a random select.
	for i := 0; i < 25; i++ {
		hosts := hdbt.hdb.RandomHosts(nEntries, nil)
		var exclude []types.SiaPublicKey
		for j := 1; j < len(hosts); j++ {
			exclude = append(exclude, hosts[j].PublicKey)
		}
		rand := hdbt.hdb.RandomHosts(1, exclude)
		if len(rand) != 1 {
			t.Fatal("wrong number of hosts returned")
		}
		if string(rand[0].PublicKey.Key) != string(hosts[0].PublicKey.Key) {
			t.Error("exclude list seems to be excluding the wrong hosts.")
		}

		// Try again but request more hosts than are available.
		rand = hdbt.hdb.RandomHosts(5, exclude)
		if len(rand) != 1 {
			t.Fatal("wrong number of hosts returned")
		}
		if string(rand[0].PublicKey.Key) != string(hosts[0].PublicKey.Key) {
			t.Error("exclude list seems to be excluding the wrong hosts.")
		}

		// Create an include map, and decrease the number of excluded hosts.
		// Make sure all hosts returned by rand function are in the include
		// map.
		includeMap := make(map[string]struct{})
		for j := 0; j < 50; j++ {
			includeMap[string(hosts[j].PublicKey.Key)] = struct{}{}
		}
		exclude = exclude[49:]

		// Select only 20 hosts.
		dupCheck := make(map[string]struct{})
		rand = hdbt.hdb.RandomHosts(20, exclude)
		if len(rand) != 20 {
			t.Error("random hosts is returning the wrong number of hosts")
		}
		for _, host := range rand {
			_, exists := dupCheck[string(host.PublicKey.Key)]
			if exists {
				t.Error("RandomHosts is seleccting duplicates")
			}
			dupCheck[string(host.PublicKey.Key)] = struct{}{}
			_, exists = includeMap[string(host.PublicKey.Key)]
			if !exists {
				t.Error("RandomHosts returning excluded hosts")
			}
		}

		// Select exactly 50 hosts.
		dupCheck = make(map[string]struct{})
		rand = hdbt.hdb.RandomHosts(50, exclude)
		if len(rand) != 50 {
			t.Error("random hosts is returning the wrong number of hosts")
		}
		for _, host := range rand {
			_, exists := dupCheck[string(host.PublicKey.Key)]
			if exists {
				t.Error("RandomHosts is seleccting duplicates")
			}
			dupCheck[string(host.PublicKey.Key)] = struct{}{}
			_, exists = includeMap[string(host.PublicKey.Key)]
			if !exists {
				t.Error("RandomHosts returning excluded hosts")
			}
		}

		// Select 100 hosts.
		dupCheck = make(map[string]struct{})
		rand = hdbt.hdb.RandomHosts(100, exclude)
		if len(rand) != 50 {
			t.Error("random hosts is returning the wrong number of hosts")
		}
		for _, host := range rand {
			_, exists := dupCheck[string(host.PublicKey.Key)]
			if exists {
				t.Error("RandomHosts is seleccting duplicates")
			}
			dupCheck[string(host.PublicKey.Key)] = struct{}{}
			_, exists = includeMap[string(host.PublicKey.Key)]
			if !exists {
				t.Error("RandomHosts returning excluded hosts")
			}
		}
	}
}

// TestRemoveNonexistingHostFromHostTree checks that the host tree interface
// correctly responds to having a nonexisting host removed from the host tree.
func TestRemoveNonexistingHostFromHostTree(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	hdbt, err := newHDBTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Remove a host that doesn't exist from the tree.
	err = hdbt.hdb.hostTree.Remove(types.SiaPublicKey{})
	if err == nil {
		t.Fatal("There should be an error, but not a panic:", err)
	}
}
