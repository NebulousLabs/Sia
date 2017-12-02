package contractor

import (
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// memPersist implements the persister interface in-memory.
type memPersist contractorPersist

func (m *memPersist) save(data contractorPersist) error { *m = memPersist(data); return nil }
func (m memPersist) load(data *contractorPersist) error { *data = contractorPersist(m); return nil }

// TestSaveLoad tests that the contractor can save and load itself.
func TestSaveLoad(t *testing.T) {
	// create contractor with mocked persist dependency
	c := &Contractor{
		persist: new(memPersist),
	}

	c.renewedIDs = map[types.FileContractID]types.FileContractID{
		{0}: {1},
		{1}: {2},
		{2}: {3},
	}
	c.oldContracts = map[types.FileContractID]modules.RenterContract{
		{0}: {ID: types.FileContractID{0}, HostPublicKey: types.SiaPublicKey{Key: []byte("foo")}},
		{1}: {ID: types.FileContractID{1}, HostPublicKey: types.SiaPublicKey{Key: []byte("bar")}},
		{2}: {ID: types.FileContractID{2}, HostPublicKey: types.SiaPublicKey{Key: []byte("baz")}},
	}

	// save, clear, and reload
	err := c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.hdb = stubHostDB{}
	c.renewedIDs = make(map[types.FileContractID]types.FileContractID)
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that all fields were restored
	_, ok0 := c.renewedIDs[types.FileContractID{0}]
	_, ok1 := c.renewedIDs[types.FileContractID{1}]
	_, ok2 := c.renewedIDs[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("renewed IDs were not restored properly:", c.renewedIDs)
	}
	_, ok0 = c.oldContracts[types.FileContractID{0}]
	_, ok1 = c.oldContracts[types.FileContractID{1}]
	_, ok2 = c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}

	// use stdPersist instead of mock
	c.persist = newPersist(build.TempDir("contractor", t.Name()))
	os.MkdirAll(build.TempDir("contractor", t.Name()), 0700)

	// save, clear, and reload
	err = c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.renewedIDs = make(map[types.FileContractID]types.FileContractID)
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that all fields were restored
	_, ok0 = c.renewedIDs[types.FileContractID{0}]
	_, ok1 = c.renewedIDs[types.FileContractID{1}]
	_, ok2 = c.renewedIDs[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("renewed IDs were not restored properly:", c.renewedIDs)
	}
	_, ok0 = c.oldContracts[types.FileContractID{0}]
	_, ok1 = c.oldContracts[types.FileContractID{1}]
	_, ok2 = c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}
}
