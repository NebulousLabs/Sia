package host

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"
)

// TestRPCDownload checks that calls to download return the correct file.
func TestRPCDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRPCDownload")
	if err != nil {
		t.Fatal(err)
	}
	nickname := "TestRPCDownload1"
	uploadData, err := ht.uploadFile(nickname, renewDisabled)
	if err != nil {
		t.Fatal(err)
	}

	// Download the file and compare to the data.
	downloadPath := filepath.Join(ht.persistDir, nickname+".download")
	err = ht.renter.Download(nickname, downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	downloadData, err := ioutil.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uploadData, downloadData) {
		t.Error("uploaded and downloaded file do not match")
	}
}
