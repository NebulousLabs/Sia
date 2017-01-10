package hostdb

import (
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
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
	var host1, host2, host3 hostEntry
	host1.NetAddress = "foo"
	host2.NetAddress = "bar"
	host3.NetAddress = "baz"
	hdb.allHosts = map[modules.NetAddress]*hostEntry{
		host1.NetAddress: &host1,
		host2.NetAddress: &host2,
		host3.NetAddress: &host3,
	}
	hdb.activeHosts = map[modules.NetAddress]*hostEntry{
		host1.NetAddress: &host1,
		host2.NetAddress: &host2,
		host3.NetAddress: &host3,
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
	hdb.activeHosts = map[modules.NetAddress]*hostEntry{
		host1.NetAddress: &host1,
		host2.NetAddress: &host2,
		host3.NetAddress: &host3,
	}

	// use a bogus change ID
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

	// Reload the hostdb using the same persist and the mocked consensus set.
	// The old change ID will be rejected, causing a rescan, which should
	// discover the new announcement.
	hdb, err = newHostDB(cs, stdDialer{}, stdSleeper{}, hdb.persist, hdb.log)
	if err != nil {
		t.Fatal(err)
	}
	if len(hdb.allHosts) != 1 {
		t.Fatal("hostdb rescan resulted in wrong host set:", hdb.allHosts)
	}
	if _, exists := hdb.allHosts["quux.com:1234"]; !exists {
		t.Fatal("hostdb rescan resulted in wrong host set:", hdb.allHosts)
	}
}
