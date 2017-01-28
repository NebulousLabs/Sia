package hostdb

import (
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/types"
)

// memPersist implements the persister interface in-memory.
type memPersist hdbPersist

func (m *memPersist) save(data hdbPersist) error     { *m = memPersist(data); return nil }
func (m *memPersist) saveSync(data hdbPersist) error { *m = memPersist(data); return nil }
func (m memPersist) load(data *hdbPersist) error     { *data = hdbPersist(m); return nil }

// TestSaveLoad tests that the hostdb can save and load itself.
func TestSaveLoad(t *testing.T) {
	// create hostdb with mocked persist dependency
	hdb := bareHostDB()
	hdb.persist = new(memPersist)

	// add some fake hosts
	var host1, host2, host3 modules.HostDBEntry
	host1.PublicKey.Key = []byte("foo")
	host2.PublicKey.Key = []byte("bar")
	host3.PublicKey.Key = []byte("baz")
	hdb.hostTree.Insert(host1)
	hdb.hostTree.Insert(host2)
	hdb.hostTree.Insert(host3)
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
	_, ok0 := hdb.hostTree.Select(host1.PublicKey)
	_, ok1 := hdb.hostTree.Select(host2.PublicKey)
	_, ok2 := hdb.hostTree.Select(host3.PublicKey)
	if !ok0 || !ok1 || !ok2 || len(hdb.hostTree.All()) != 3 {
		t.Fatal("allHosts was not restored properly", ok0, ok1, ok2, len(hdb.hostTree.All()))
	}
}

// rescanCS is a barebones implementation of a consensus set that can be
// subscribed to.
type rescanCS struct {
	changes []modules.ConsensusChange
}

func (cs *rescanCS) addBlock(b types.Block) {
	cc := modules.ConsensusChange{
		AppliedBlocks: []types.Block{b},
	}
	rand.Read(cc.ID[:])
	cs.changes = append(cs.changes, cc)
}

func (cs *rescanCS) ConsensusSetSubscribe(s modules.ConsensusSetSubscriber, lastChange modules.ConsensusChangeID) error {
	var start int
	if lastChange != (modules.ConsensusChangeID{}) {
		start = -1
		for i, cc := range cs.changes {
			if cc.ID == lastChange {
				start = i
				break
			}
		}
		if start == -1 {
			return modules.ErrInvalidConsensusChangeID
		}
	}
	for _, cc := range cs.changes[start:] {
		s.ProcessConsensusChange(cc)
	}
	return nil
}

// TestRescan tests that the hostdb will rescan the blockchain properly.
func TestRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create hostdb with mocked persist dependency
	hdb := bareHostDB()
	hdb.persist = new(memPersist)

	// add some fake hosts
	var host1, host2, host3 modules.HostDBEntry
	host1.NetAddress = "foo"
	host2.NetAddress = "bar"
	host3.NetAddress = "baz"
	hdb.hostTree.Insert(host1)
	hdb.hostTree.Insert(host2)
	hdb.hostTree.Insert(host3)
	hdb.lastChange = modules.ConsensusChangeID{1, 2, 3}

	// save the hostdb
	err := hdb.save()
	if err != nil {
		t.Fatal(err)
	}

	// create a mocked consensus set with a different host announcement
	annBytes, err := makeSignedAnnouncement("quux.com:1234")
	if err != nil {
		t.Fatal(err)
	}
	announceBlock := types.Block{
		Transactions: []types.Transaction{{
			ArbitraryData: [][]byte{annBytes},
		}},
	}
	cs := new(rescanCS)
	cs.addBlock(announceBlock)

	testDir := build.TempDir("HostDB", "TestRescan")
	g, err := gateway.New("localhost:0", false, filepath.Join(testDir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}

	// Reload the hostdb using the same persist and the mocked consensus set.
	// The old change ID will be rejected, causing a rescan, which should
	// discover the new announcement.
	hdb, err = newHostDB(g, cs, stdDialer{}, stdSleeper{}, hdb.persist, hdb.log)
	if err != nil {
		t.Fatal(err)
	}
	if len(hdb.hostTree.All()) != 1 {
		t.Fatal("hostdb rescan resulted in wrong host set")
	}
}
