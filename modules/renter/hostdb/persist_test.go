package hostdb

import (
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
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

	// add some fake contracts
	hdb.contracts = map[types.FileContractID]hostContract{
		{0}: {IP: "foo"},
		{1}: {IP: "bar"},
		{2}: {IP: "baz"},
	}
	// save and reload
	err := hdb.save()
	if err != nil {
		t.Fatal(err)
	}
	err = hdb.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that contracts were restored
	_, ok0 := hdb.contracts[types.FileContractID{0}]
	_, ok1 := hdb.contracts[types.FileContractID{1}]
	_, ok2 := hdb.contracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("contracts were not restored properly:", hdb.contracts)
	}

	// use stdPersist instead of mock
	hdb.persist = newPersist(build.TempDir("hostdb", "TestSaveLoad"))
	os.MkdirAll(build.TempDir("hostdb", "TestSaveLoad"), 0700)

	// save and reload
	err = hdb.save()
	if err != nil {
		t.Fatal(err)
	}
	err = hdb.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that contracts were restored
	_, ok0 = hdb.contracts[types.FileContractID{0}]
	_, ok1 = hdb.contracts[types.FileContractID{1}]
	_, ok2 = hdb.contracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("contracts were not restored properly:", hdb.contracts)
	}
}
