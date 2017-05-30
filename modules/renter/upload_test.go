package renter

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestRenterSiapathValidate verifies that the validateSiapath function correctly validates SiaPaths.
func TestRenterSiapathValidate(t *testing.T) {
	var pathtests = []struct {
		in    string
		valid bool
	}{
		{"valid/siapath", true},
		{"../../../directory/traversal", false},
		{"testpath", true},
		{"valid/siapath/../with/directory/traversal", false},
		{"validpath/test", true},
		{"..validpath/..test", true},
		{"./invalid/path", false},
		{"test/path", true},
		{"/leading/slash", false},
		{"foo/./bar", false},
		{"", false},
	}
	for _, pathtest := range pathtests {
		err := validateSiapath(pathtest.in)
		if err != nil && pathtest.valid {
			t.Fatal("validateSiapath failed on valid path: ", pathtest.in)
		}
		if err == nil && !pathtest.valid {
			t.Fatal("validateSiapath succeeded on invalid path: ", pathtest.in)
		}
	}
}

// TestRenterUploadInode verifies that the renter returns an error if an inode
// is provided as the source of an upload.
func TestRenterUploadInode(t *testing.T) {
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
		t.Fatal("expected Upload to fail with empty inode as source")
	}
	if err != errUploadInode {
		t.Fatal("expected errUploadEmptyInode, got", err)
	}
}
