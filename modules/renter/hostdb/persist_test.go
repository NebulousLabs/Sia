package hostdb

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// quitAfterLoadDeps will quit startup in newHostDB
type quitAfterLoadDeps struct {
	modules.ProductionDependencies
}

// Send a disrupt signal to the quitAfterLoad codebreak.
func (*quitAfterLoadDeps) Disrupt(s string) bool {
	if s == "quitAfterLoad" {
		return true
	}
	return false
}

// TestSaveLoad tests that the hostdb can save and load itself.
func TestSaveLoad(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	hdbt, err := newHDBTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Mine two blocks to put the hdb height at 2.
	for i := 0; i < 2; i++ {
		_, err := hdbt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Verify that the hdb height is 2.
	if hdbt.hdb.blockHeight != 2 {
		t.Fatal("test setup incorrect, hdb height needs to be 2 for remainder of test")
	}

	// Add fake hosts and a fake consensus change. The fake consensus change
	// would normally be detected and routed around, but we stunt the loading
	// process to only load the persistent fields.
	var host1, host2, host3 modules.HostDBEntry
	host1.FirstSeen = 1
	host2.FirstSeen = 2
	host3.FirstSeen = 3
	host1.PublicKey.Key = []byte("foo")
	host2.PublicKey.Key = []byte("bar")
	host3.PublicKey.Key = []byte("baz")
	hdbt.hdb.hostTree.Insert(host1)
	hdbt.hdb.hostTree.Insert(host2)
	hdbt.hdb.hostTree.Insert(host3)

	// Save, close, and reload.
	hdbt.hdb.mu.Lock()
	hdbt.hdb.lastChange = modules.ConsensusChangeID{1, 2, 3}
	stashedLC := hdbt.hdb.lastChange
	err = hdbt.hdb.saveSync()
	hdbt.hdb.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	err = hdbt.hdb.Close()
	if err != nil {
		t.Fatal(err)
	}
	hdbt.hdb, err = NewCustomHostDB(hdbt.gateway, hdbt.cs, filepath.Join(hdbt.persistDir, modules.RenterDir), &quitAfterLoadDeps{})
	if err != nil {
		t.Fatal(err)
	}

	// Last change should have been reloaded.
	hdbt.hdb.mu.Lock()
	lastChange := hdbt.hdb.lastChange
	hdbt.hdb.mu.Unlock()
	if lastChange != stashedLC {
		t.Error("wrong consensus change ID was loaded:", hdbt.hdb.lastChange)
	}

	// Check that AllHosts was loaded.
	h1, ok0 := hdbt.hdb.hostTree.Select(host1.PublicKey)
	h2, ok1 := hdbt.hdb.hostTree.Select(host2.PublicKey)
	h3, ok2 := hdbt.hdb.hostTree.Select(host3.PublicKey)
	if !ok0 || !ok1 || !ok2 || len(hdbt.hdb.hostTree.All()) != 3 {
		t.Error("allHosts was not restored properly", ok0, ok1, ok2, len(hdbt.hdb.hostTree.All()))
	}
	if h1.FirstSeen != 1 {
		t.Error("h1 block height loaded incorrectly")
	}
	if h2.FirstSeen != 2 {
		t.Error("h1 block height loaded incorrectly")
	}
	if h3.FirstSeen != 2 {
		t.Error("h1 block height loaded incorrectly")
	}
}

// TestRescan tests that the hostdb will rescan the blockchain properly, picking
// up new hosts which appear in an alternate past.
func TestRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	_, err := newHDBTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	t.Skip("create two consensus sets with blocks + announcements")
}
