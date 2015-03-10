package main

import (
	"os"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// TestUploadAndDownload creates a network with a host and then uploads a file
// from the renter to the host, and then downloads it.
func TestUploadAndDownload(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Create a daemon and add a host to the network.
	dt := newDaemonTester(t)
	dt.announceHost()

	for dt.hostdb.NumHosts() == 0 {
		time.Sleep(time.Millisecond)
	}

	// Upload to the host.
	dt.callAPI("/renter/uploadpath?Pieces=1&Source=api.go&Nickname=first")

	// Wait for the upload to finish - this is necessary due to the
	// fact that zero-conf transactions aren't actually propagated properly.
	time.Sleep(consensus.RenterZeroConfDelay + 1e9)

	rentInfo := dt.renter.Info()
	if len(rentInfo.Files) != 1 {
		t.Error("file is not uploaded")
	}

	// Try to download the file.
	dt.callAPI("/renter/download?Destination=renterTestDL_test&Nickname=first")
	time.Sleep(time.Second * 2)

	// Check that the downloaded file is equal to the uploaded file.
	upFile, err := os.Open("api.go")
	if err != nil {
		t.Fatal(err)
	}
	defer upFile.Close()
	downFile, err := os.Open("renterTestDL_test")
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
