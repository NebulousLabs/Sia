package renter

import (
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/types"
)

// repairHostDB is a mocked hostDB, hostdb.HostPool, and hostdb.Uploader. It
// is used for testing the uploading and repairing functions of the renter.
type repairHostDB struct{}

// NewPool returns a new mock HostPool. Since repairHostDB implements the
// HostPool interface, it can simply return itself.
func (hdb repairHostDB) NewPool(uint64, types.BlockHeight) (hostdb.HostPool, error) {
	return hdb, nil
}

// UniqueHosts returns a set of mocked Uploaders. Since repairHostDB
// implements the Uploader interface, it can simply return itself.
func (hdb repairHostDB) UniqueHosts(n int, _ []modules.NetAddress) (ups []hostdb.Uploader) {
	for i := 0; i < n; i++ {
		ups = append(ups, hdb)
	}
	return
}

// Upload simulates a successful data upload.
func (repairHostDB) Upload(data []byte) (uint64, error) { return uint64(len(data)), nil }

// stub implementations of the hostdb.Uploader methods
func (repairHostDB) Address() modules.NetAddress      { return "" }
func (repairHostDB) ContractID() types.FileContractID { return types.FileContractID{} }
func (repairHostDB) EndHeight() types.BlockHeight     { return 10000 }
func (repairHostDB) Close() error                     { return nil }

// stub implementations of the hostDB methods
func (repairHostDB) ActiveHosts() []modules.HostSettings { return nil }
func (repairHostDB) AllHosts() []modules.HostSettings    { return nil }
func (repairHostDB) AveragePrice() types.Currency        { return types.Currency{} }
func (repairHostDB) Renew(types.FileContractID, types.BlockHeight) (types.FileContractID, error) {
	return types.FileContractID{}, nil
}

// TestRepairLoop tests the uploading and repairing functions. The hostDB is
// mocked, isolating the upload/repair logic from the negotation logic.
func TestRepairLoop(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// create renter
	rt, err := newRenterTester("TestRepairLoop")
	if err != nil {
		t.Fatal(err)
	}
	// swap in our own hostdb
	rt.renter.hostDB = &repairHostDB{}

	// create a file
	path := filepath.Join(build.SiaTestingDir, "renter", "TestRepairLoop", "test.dat")
	err = ioutil.WriteFile(path, []byte{1, 2, 3}, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// upload file
	rt.renter.Upload(modules.FileUploadParams{
		Filename: path,
		Nickname: "foo",
	})
	files := rt.renter.FileList()
	if len(files) != 1 {
		t.Fatal("expected 1 file, got", len(files))
	}

	// wait for repair loop for fully upload file
	for files[0].UploadProgress != 100 {
		files = rt.renter.FileList()
		time.Sleep(time.Second)
	}
}
