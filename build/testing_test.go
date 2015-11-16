package build

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// TestCopyDir checks that CopyDir copies directories as expected.
func TestCopyDir(t *testing.T) {
	// Create some nested folders to copy.
	os.MkdirAll(TempDir("build"), 0700)
	root := TempDir("build", "TestCopyDir")
	os.MkdirAll(root, 0700)

	data := make([][]byte, 2)
	for i := range data {
		data[i] = make([]byte, 4e3)
		_, err := rand.Read(data[i])
		if err != nil {
			t.Fatal(err)
		}
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
	if bytes.Compare(f1, data[0]) != 0 {
		t.Error("f1 did not match")
	}
	d1f1, err := ioutil.ReadFile(filepath.Join(rootCopy, "d1", "d1f1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(d1f1, data[1]) != 0 {
		t.Error("f1 did not match")
	}
}
