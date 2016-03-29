package hostdb

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// memPersist implements the persister interface in-memory.
type memPersist hdbPersist

func (m *memPersist) save(data hdbPersist) error { *m = memPersist(data); return nil }
func (m memPersist) load(data *hdbPersist) error { *data = hdbPersist(m); return nil }

// TestSaveLoad tests that the hostdb can save and load itself.
func TestSaveLoad(t *testing.T) {
	// create hostdb with mocked persist dependency
	hdb := bareHostDB()
	hdb.persist = new(memPersist)

	// add some fake hosts
	var host1, host2, host3 hostEntry
	host1.NetAddress = "foo"
	host2.NetAddress = "bar"
	host3.NetAddress = "baz"
	hdb.allHosts = map[modules.NetAddress]*hostEntry{
		host1.NetAddress: &host1,
		host2.NetAddress: &host2,
		host3.NetAddress: &host3,
	}
	hdb.activeHosts = map[modules.NetAddress]*hostNode{
		host1.NetAddress: &hostNode{hostEntry: &host1},
		host2.NetAddress: &hostNode{hostEntry: &host2},
		host3.NetAddress: &hostNode{hostEntry: &host3},
	}
	hdb.lastChange = modules.ConsensusChangeID{1, 2, 3}

	// save and reload
	err := hdb.save()
	if err != nil {
		t.Fatal(err)
	}
	err = hdb.load()
	if err != nil {
		t.Fatal(err)
	}

	// check that LastChange was loaded
	if hdb.lastChange != (modules.ConsensusChangeID{1, 2, 3}) {
		t.Error("wrong consensus change ID was loaded:", hdb.lastChange)
	}

	// check that AllHosts was loaded
	_, ok0 := hdb.allHosts[host1.NetAddress]
	_, ok1 := hdb.allHosts[host2.NetAddress]
	_, ok2 := hdb.allHosts[host3.NetAddress]
	if !ok0 || !ok1 || !ok2 || len(hdb.allHosts) != 3 {
		t.Fatal("allHosts was not restored properly:", hdb.allHosts)
	}

	// check that ActiveHosts was loaded
	_, ok0 = hdb.activeHosts[host1.NetAddress]
	_, ok1 = hdb.activeHosts[host2.NetAddress]
	_, ok2 = hdb.activeHosts[host3.NetAddress]
	if !ok0 || !ok1 || !ok2 || len(hdb.activeHosts) != 3 {
		t.Fatal("active was not restored properly:", hdb.activeHosts)
	}
}
