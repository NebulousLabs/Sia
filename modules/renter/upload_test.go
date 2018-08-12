package renter

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestRenterUploadDirectory verifies that the renter returns an error if a
// directory is provided as the source of an upload.
func TestRenterUploadInode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	testUploadPath, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testUploadPath)

	ec, err := NewRSCode(defaultDataPieces, defaultParityPieces)
	if err != nil {
		t.Fatal(err)
	}
	params := modules.FileUploadParams{
		Source:      testUploadPath,
		SiaPath:     "test",
		ErasureCode: ec,
	}
	err = rt.renter.Upload(params)
	if err == nil {
		t.Fatal("expected Upload to fail with empty directory as source")
	}
	if err != errUploadDirectory {
		t.Fatal("expected errUploadDirectory, got", err)
	}
}
