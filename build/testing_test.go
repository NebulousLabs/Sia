package build

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"gitlab.com/NebulousLabs/fastrand"
)

// TestCopyDir checks that CopyDir copies directories as expected.
func TestCopyDir(t *testing.T) {
	// Create some nested folders to copy.
	os.MkdirAll(TempDir("build"), 0700)
	root := TempDir("build", t.Name())
	os.MkdirAll(root, 0700)

	data := make([][]byte, 2)
	for i := range data {
		data[i] = fastrand.Bytes(4e3)
	}

	// Create a file and a directory.
	err := ioutil.WriteFile(filepath.Join(root, "f1"), data[0], 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(filepath.Join(root, "d1"), 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(filepath.Join(root, "d1", "d1f1"), data[1], 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Copy the root directory.
	rootCopy := root + "-copied"
	err = CopyDir(root, rootCopy)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the two files, and dir with two files are all correctly
	// copied.
	f1, err := ioutil.ReadFile(filepath.Join(rootCopy, "f1"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f1, data[0]) {
		t.Error("f1 did not match")
	}
	d1f1, err := ioutil.ReadFile(filepath.Join(rootCopy, "d1", "d1f1"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d1f1, data[1]) {
		t.Error("f1 did not match")
	}
}

// TestExtractTarGz tests that ExtractTarGz can extract a valid .tar.gz file.
func TestExtractTarGz(t *testing.T) {
	dir := TempDir("build", t.Name())
	os.MkdirAll(dir, 0700)
	if err := ExtractTarGz("testdata/test.tar.gz", dir); err != nil {
		t.Fatal(err)
	}
	folder, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := folder.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	exp := []string{"1", "2", "3"}
	if !reflect.DeepEqual(files, exp) {
		t.Fatal("filenames do not match:", files, exp)
	}
}
