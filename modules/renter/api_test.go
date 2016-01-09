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

// repairUploader is a mocked hostdb.Uploader, used for testing the uploading
// and repairing functions of the renter.
type repairUploader struct{}

// Upload simulates a successful data upload.
func (repairUploader) Upload(data []byte) (uint64, error) { return uint64(len(data)), nil }

func (repairUploader) Address() modules.NetAddress      { return "" }
func (repairUploader) ContractID() types.FileContractID { return types.FileContractID{} }
func (repairUploader) EndHeight() types.BlockHeight     { return 10000 }
func (repairUploader) Close() error                     { return nil }

// repairHostPool is a mocked hostdb.HostPool, used for testing the uploading
// and repairing functions of the renter.
type repairHostPool struct{}

// UniqueHosts returns a set of mocked Uploaders.
func (repairHostPool) UniqueHosts(n int, _ []modules.NetAddress) (ups []hostdb.Uploader) {
	for i := 0; i < n; i++ {
		ups = append(ups, repairUploader{})
	}
	return
}

// Close is a stub implementation of the Close method.
func (repairHostPool) Close() error { return nil }

// repairHostDB is a mocked hostDB, used for testing the uploading and
// repairing functions of the renter.
type repairHostDB struct{}

// ActiveHosts is a stub implementation of the ActiveHosts method.
func (repairHostDB) ActiveHosts() []modules.HostSettings { return nil }

// AllHosts is a stub implementation of the AllHosts method.
func (repairHostDB) AllHosts() []modules.HostSettings { return nil }

// AveragePrice is a stub implementation of the AveragePrice method.
func (repairHostDB) AveragePrice() types.Currency { return types.Currency{} }

// NewPool returns a new mock HostPool.
func (hdb repairHostDB) NewPool(uint64, types.BlockHeight) (hostdb.HostPool, error) {
	return repairHostPool{}, nil
}

// Renew is a stub implementation of the Renew method.
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
