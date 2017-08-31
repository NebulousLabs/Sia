package renter

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenterDownloadFileWriterClose verifies that the renter's
// DownloadFileWriter closes the underlying file handle when the entire file
// has been written.
func TestRenterDownloadFileWriterClose(t *testing.T) {
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

	b := make([]byte, 100)
	_, err = df.WriteAt(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = df.f.Read(b)
	if err == nil {
		t.Fatal("expected read to fail after writing full length")
	}
	if !strings.Contains(err.Error(), "file already closed") {
		t.Fatal("expected read to return file already closed, got", err, "instead.")
	}
}
