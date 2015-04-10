package api

import (
	"os"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/types"
)

// TestUploadAndDownload creates a network with a host and then uploads a file
// from the renter to the host, and then downloads it.
func TestUploadAndDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a server and add a host to the network.
	st := newServerTester("TestUploadAndDownload", t)
	st.announceHost()

	for len(st.hostdb.ActiveHosts()) == 0 {
		time.Sleep(time.Millisecond)
	}

	// Upload to the host.
	uploadName := "api.go"
	st.callAPI("/renter/upload?pieces=1&nickname=first&source=" + uploadName)

	// Wait for the upload to finish - this is necessary due to the
	// fact that zero-conf transactions aren't actually propagated properly.
	//
	// TODO: There should be some way to just spinblock until the download
	// completes. Except there's no exported function in the renter that will
	// indicate if a download has completed or not.
	time.Sleep(types.RenterZeroConfDelay + time.Second*10)

	files := st.renter.FileList()
	if len(files) != 1 || !files[0].Available() {
		t.Fatal("file is not uploaded")
	}

	// Try to download the file.
	downloadName := tester.TempDir("api", "TestUploadAndDownload", "downloadTestData")
	st.callAPI("/renter/download?nickname=first&destination=" + downloadName)
	time.Sleep(time.Second * 2)

	// Check that the downloaded file is equal to the uploaded file.
	upFile, err := os.Open(uploadName)
	if err != nil {
		t.Fatal(err)
	}
	defer upFile.Close()
	downFile, err := os.Open(downloadName)
	if err != nil {
		t.Fatal(err)
	}
	defer upFile.Close()
	upRoot, err := crypto.ReaderMerkleRoot(upFile)
	if err != nil {
		t.Fatal(err)
	}
	downRoot, err := crypto.ReaderMerkleRoot(downFile)
	if err != nil {
		t.Fatal(err)
	}
	if upRoot != downRoot {
		t.Error("uploaded and downloaded file have a hash mismatch")
	}
}
