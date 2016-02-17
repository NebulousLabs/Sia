package renter

import (
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/types"
)

// uploadContractor is a mocked hostContracto, contractor.HostPool, and
// contractor.Uploader. It is used for testing the uploading and repairing
// functions of the renter.
type uploadContractor struct {
	stubContractor
}

// NewPool returns a new mock HostPool. Since uploadContractor implements the
// HostPool interface, it can simply return itself.
func (hdb uploadContractor) NewPool(uint64, types.BlockHeight) (contractor.HostPool, error) {
	return hdb, nil
}

// UniqueHosts returns a set of mocked Uploaders. Since uploadContractor
// implements the Uploader interface, it can simply return itself.
func (hdb uploadContractor) UniqueHosts(n int, _ []modules.NetAddress) (ups []contractor.Uploader) {
	for i := 0; i < n; i++ {
		ups = append(ups, hdb)
	}
	return
}

// Upload simulates a successful data upload.
func (uploadContractor) Upload(data []byte) (uint64, error) { return uint64(len(data)), nil }

// stub implementations of the contractor.Uploader methods
func (uploadContractor) Address() modules.NetAddress      { return "" }
func (uploadContractor) ContractID() types.FileContractID { return types.FileContractID{} }
func (uploadContractor) EndHeight() types.BlockHeight     { return 10000 }
func (uploadContractor) Close() error                     { return nil }

// TestUpload tests the uploading and repairing functions. The hostDB is
// mocked, isolating the upload/repair logic from the negotation logic.
func TestUpload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// create renter
	rt, err := newRenterTester("TestUpload")
	if err != nil {
		t.Fatal(err)
	}
	// swap in our own contractor
	rt.renter.hostContractor = &uploadContractor{}

	// create a file
	source := filepath.Join(build.SiaTestingDir, "renter", "TestUpload", "test.dat")
	err = ioutil.WriteFile(source, []byte{1, 2, 3}, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// upload file
	rt.renter.Upload(modules.FileUploadParams{
		Source:  source,
		SiaPath: "foo",
		// Upload will use sane defaults for other params
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
