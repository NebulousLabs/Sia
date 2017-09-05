package renter

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenterDownloadFileWriter verifies that the renter's DownloadFileWriter
// has the correct behavior.
func TestRenterDownloadFileWriter(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	testPath, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	testPath = filepath.Join(testPath, "testfile")
	defer os.RemoveAll(testPath)
	df, err := NewDownloadFileWriter(testPath, 0, 100)
	if err != nil {
		t.Fatal(err)
	}

	// writing a too-large slice should fail
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected too-large slice to error")
			}
		}()
		df.WriteAt(make([]byte, 200), 0)
	}()

	// Close should close the file handle
	err = df.f.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = df.f.Read(make([]byte, 100))
	if err == nil {
		t.Fatal("expected read to fail after writing full length")
	}
	if !strings.Contains(err.Error(), "file already closed") {
		t.Fatal("expected read to return file already closed, got", err, "instead.")
	}
}
