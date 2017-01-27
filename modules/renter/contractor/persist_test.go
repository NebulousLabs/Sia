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

func (m *memPersist) save(data contractorPersist) error     { *m = memPersist(data); return nil }
func (m *memPersist) saveSync(data contractorPersist) error { *m = memPersist(data); return nil }
func (m memPersist) load(data *contractorPersist) error     { *data = contractorPersist(m); return nil }

// TestSaveLoad tests that the contractor can save and load itself.
func TestSaveLoad(t *testing.T) {
	// create contractor with mocked persist dependency
	c := &Contractor{
		persist: new(memPersist),
	}

	// add some fake contracts
	c.contracts = map[types.FileContractID]modules.RenterContract{
		{0}: {ID: types.FileContractID{0}, NetAddress: "foo"},
		{1}: {ID: types.FileContractID{1}, NetAddress: "bar"},
		{2}: {ID: types.FileContractID{2}, NetAddress: "baz"},
	}
	c.renewedIDs = map[types.FileContractID]types.FileContractID{
		{0}: {1},
		{1}: {2},
		{2}: {3},
	}
	c.cachedRevisions = map[types.FileContractID]cachedRevision{
		{0}: {Revision: types.FileContractRevision{ParentID: types.FileContractID{0}}},
		{1}: {Revision: types.FileContractRevision{ParentID: types.FileContractID{1}}},
		{2}: {Revision: types.FileContractRevision{ParentID: types.FileContractID{2}}},
	}
	c.oldContracts = map[types.FileContractID]modules.RenterContract{
		{0}: {ID: types.FileContractID{0}, NetAddress: "foo"},
		{1}: {ID: types.FileContractID{1}, NetAddress: "bar"},
		{2}: {ID: types.FileContractID{2}, NetAddress: "baz"},
	}

	// save, clear, and reload
	err := c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.hdb = stubHostDB{}
	c.contracts = make(map[types.FileContractID]modules.RenterContract)
	c.renewedIDs = make(map[types.FileContractID]types.FileContractID)
	c.cachedRevisions = make(map[types.FileContractID]cachedRevision)
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that all fields were restored
	_, ok0 := c.contracts[types.FileContractID{0}]
	_, ok1 := c.contracts[types.FileContractID{1}]
	_, ok2 := c.contracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("contracts were not restored properly:", c.contracts)
	}
	_, ok0 = c.renewedIDs[types.FileContractID{0}]
	_, ok1 = c.renewedIDs[types.FileContractID{1}]
	_, ok2 = c.renewedIDs[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("renewed IDs were not restored properly:", c.renewedIDs)
	}
	_, ok0 = c.cachedRevisions[types.FileContractID{0}]
	_, ok1 = c.cachedRevisions[types.FileContractID{1}]
	_, ok2 = c.cachedRevisions[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("cached revisions were not restored properly:", c.cachedRevisions)
	}
	_, ok0 = c.oldContracts[types.FileContractID{0}]
	_, ok1 = c.oldContracts[types.FileContractID{1}]
	_, ok2 = c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}

	// use stdPersist instead of mock
	c.persist = newPersist(build.TempDir("contractor", "TestSaveLoad"))
	os.MkdirAll(build.TempDir("contractor", "TestSaveLoad"), 0700)

	// save, clear, and reload
	err = c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.contracts = make(map[types.FileContractID]modules.RenterContract)
	c.renewedIDs = make(map[types.FileContractID]types.FileContractID)
	c.cachedRevisions = make(map[types.FileContractID]cachedRevision)
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that all fields were restored
	_, ok0 = c.contracts[types.FileContractID{0}]
	_, ok1 = c.contracts[types.FileContractID{1}]
	_, ok2 = c.contracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("contracts were not restored properly:", c.contracts)
	}
	_, ok0 = c.renewedIDs[types.FileContractID{0}]
	_, ok1 = c.renewedIDs[types.FileContractID{1}]
	_, ok2 = c.renewedIDs[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("renewed IDs were not restored properly:", c.renewedIDs)
	}
	_, ok0 = c.cachedRevisions[types.FileContractID{0}]
	_, ok1 = c.cachedRevisions[types.FileContractID{1}]
	_, ok2 = c.cachedRevisions[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("cached revisions were not restored properly:", c.cachedRevisions)
	}
	_, ok0 = c.oldContracts[types.FileContractID{0}]
	_, ok1 = c.oldContracts[types.FileContractID{1}]
	_, ok2 = c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}
}
