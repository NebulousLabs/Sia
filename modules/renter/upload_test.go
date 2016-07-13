package renter

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/types"
)

// uploadDownloadContractor is a mocked hostContractor, contractor.Editor, and contractor.Downloader. It is
// used for testing the uploading and downloading functions of the renter.
type uploadDownloadContractor struct {
	stubContractor
	sectors map[crypto.Hash][]byte
	mu      sync.Mutex
}

func (uc *uploadDownloadContractor) Contract(modules.NetAddress) (modules.RenterContract, bool) {
	return modules.RenterContract{}, true
}

func (uc *uploadDownloadContractor) Contracts() []modules.RenterContract {
	return make([]modules.RenterContract, 24) // exact number shouldn't matter, as long as its large enough
}

// Editor simply returns the uploadDownloadContractor, since it also implements the
// Editor interface.
func (uc *uploadDownloadContractor) Editor(modules.RenterContract) (contractor.Editor, error) {
	return uc, nil
}

// Downloader simply returns the uploadDownloadContractor, since it also
// implements the Downloader interface.
func (uc *uploadDownloadContractor) Downloader(modules.RenterContract) (contractor.Downloader, error) {
	return uc, nil
}

// Upload simulates a successful data upload.
func (uc *uploadDownloadContractor) Upload(data []byte) (crypto.Hash, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	root := crypto.MerkleRoot(data)
	uc.sectors[root] = data
	return root, nil
}

// Download simulates a successful data download.
func (uc *uploadDownloadContractor) Sector(root crypto.Hash) ([]byte, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	return uc.sectors[root], nil
}

// stub implementations of the contractor.Editor methods
func (*uploadDownloadContractor) Address() modules.NetAddress                           { return "" }
func (*uploadDownloadContractor) Delete(crypto.Hash) error                              { return nil }
func (*uploadDownloadContractor) Modify(crypto.Hash, crypto.Hash, uint64, []byte) error { return nil }
func (*uploadDownloadContractor) ContractID() types.FileContractID                      { return types.FileContractID{} }
func (*uploadDownloadContractor) EndHeight() types.BlockHeight                          { return 10000 }
func (*uploadDownloadContractor) Close() error                                          { return nil }

// TestUploadDownload tests the Upload and Download methods using a mock
// contractor.
func TestUploadDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// create renter with mocked contractor
	hc := &uploadDownloadContractor{
		sectors: make(map[crypto.Hash][]byte),
	}
	rt, err := newContractorTester("TestUploadDownload", nil, hc)
	if err != nil {
		t.Fatal(err)
	}

	// create a file
	data, err := crypto.RandBytes(777)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(build.SiaTestingDir, "renter", "TestUploadDownload", "test.dat")
	err = ioutil.WriteFile(source, data, 0600)
	if err != nil {
		t.Fatal(err)
	}

	// use 1-1 erasure code, because we'll only have one host
	rsc, _ := NewRSCode(1, 1)

	// upload file
	err = rt.renter.Upload(modules.FileUploadParams{
		Source:      source,
		SiaPath:     "foo",
		ErasureCode: rsc,
		// Upload will use sane defaults for other params
	})
	if err != nil {
		t.Fatal(err)
	}
	files := rt.renter.FileList()
	if len(files) != 1 {
		t.Fatal("expected 1 file, got", len(files))
	}

	// wait for repair loop for fully upload file
	for i := 0; i < 10 && !files[0].Available; i++ {
		files = rt.renter.FileList()
		time.Sleep(time.Second)
	}
	if !files[0].Available {
		t.Fatal("file did not reach full availability:", files[0].UploadProgress)
	}

	// download the file
	dest := filepath.Join(build.SiaTestingDir, "renter", "TestUploadDownload", "test.dat")
	err = rt.renter.Download("foo", dest)
	if err != nil {
		t.Fatal(err)
	}

	downData, err := ioutil.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downData, data) {
		t.Fatal("recovered data does not match original")
	}
}
