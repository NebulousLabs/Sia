package contractor

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
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
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// Check that all fields were restored
	_, ok0 := c.oldContracts[types.FileContractID{0}]
	_, ok1 := c.oldContracts[types.FileContractID{1}]
	_, ok2 := c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}
	// use stdPersist instead of mock
	c.persist = NewPersist(build.TempDir("contractor", t.Name()))
	os.MkdirAll(build.TempDir("contractor", t.Name()), 0700)

	// save, clear, and reload
	err = c.save()
	if err != nil {
		t.Fatal(err)
	}
	c.oldContracts = make(map[types.FileContractID]modules.RenterContract)
	err = c.load()
	if err != nil {
		t.Fatal(err)
	}
	// check that all fields were restored
	_, ok0 = c.oldContracts[types.FileContractID{0}]
	_, ok1 = c.oldContracts[types.FileContractID{1}]
	_, ok2 = c.oldContracts[types.FileContractID{2}]
	if !ok0 || !ok1 || !ok2 {
		t.Fatal("oldContracts were not restored properly:", c.oldContracts)
	}
}

// TestConvertPersist tests that contracts previously stored in the
// .journal format can be converted to the .contract format.
func TestConvertPersist(t *testing.T) {
	dir := build.TempDir(filepath.Join("contractor", t.Name()))
	os.MkdirAll(dir, 0700)
	// copy the test data into the temp folder
	testdata, err := ioutil.ReadFile(filepath.Join("testdata", "TestConvertPersist.journal"))
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(filepath.Join(dir, "contractor.journal"), testdata, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// convert the journal
	err = convertPersist(dir)
	if err != nil {
		t.Fatal(err)
	}

	// load the persist
	var p contractorPersist
	err = NewPersist(dir).load(&p)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Allowance.Funds.Equals64(10) || p.Allowance.Hosts != 7 || p.Allowance.Period != 3 || p.Allowance.RenewWindow != 20 {
		t.Fatal("recovered allowance was wrong:", p.Allowance)
	}

	// load the contracts
	cs, err := proto.NewContractSet(filepath.Join(dir, "contracts"), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	if cs.Len() != 1 {
		t.Fatal("expected 1 contract, got", cs.Len())
	}
	m := cs.ViewAll()[0]
	if m.ID.String() != "792b5eec683819d78416a9e80cba454ebcb5a52eeac4f17b443d177bd425fc5c" {
		t.Fatal("recovered contract has wrong ID", m.ID)
	}
}
