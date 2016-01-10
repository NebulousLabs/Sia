package host

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"
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

	// Block until the renter is at 50 upload progress - it takes time for the
	// contract to confirm renter-side.
	complete := false
	for i := 0; i < 50 && !complete; i++ {
		fileInfos := ht.renter.FileList()
		for _, fileInfo := range fileInfos {
			if fileInfo.UploadProgress >= 50 {
				complete = true
			}
		}
		if complete {
			break
		}
		time.Sleep(time.Millisecond * 50)
	}
	if !complete {
		t.Error("Renter never registered the upload")
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
