package api

import (
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// TestIntegrationHosting tests that the host correctly receives payment for
// hosting files.
func TestIntegrationHosting(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationHosting")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// create a file
	path := filepath.Join(build.SiaTestingDir, "api", "TestIntegrationHosting", "test.dat")
	err = createRandFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// upload to host
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("duration", "10")
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	var rf RenterFiles
	for i := 0; i < 150 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress != 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress != 10 {
		t.Fatal("the uploading is not succeeding for some reason")
	}

	// Mine blocks until the host recognizes profit. The host will wait for 12
	// blocks after the storage window has closed to report the profit, a total
	// of 40 blocks should be mined.
	for i := 0; i < 40; i++ {
		st.miner.AddBlock()
	}

	// check profit
	var hg HostGET
	err = st.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}
	expRevenue := "15307662222190387473"
	if hg.Revenue.String() != expRevenue {
		t.Fatalf("host's profit was not affected: expected %v, got %v", expRevenue, hg.Revenue)
	}
}

// TestIntegrationRenewing tests that the renter and host manage contract
// renewals properly.
func TestIntegrationRenewing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationRenewing")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// create a file
	path := filepath.Join(build.SiaTestingDir, "api", "TestIntegrationRenewing", "test.dat")
	err = createRandFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// upload to host, specifying that the file should be renewed
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	var rf RenterFiles
	for i := 0; i < 150 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress != 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress != 10 {
		t.Error(rf.Files[0].UploadProgress)
		t.Fatal("uploading has failed")
	}

	// default expiration is 20 blocks
	expExpiration := st.cs.Height() + 20
	if rf.Files[0].Expiration != expExpiration {
		t.Fatalf("expected expiration of %v, got %v", expExpiration, rf.Files[0].Expiration)
	}

	// mine blocks until we hit the renew threshold (default 10 blocks)
	for st.cs.Height() < expExpiration-10 {
		st.miner.AddBlock()
	}

	// renter should now renew the contract for another 20 blocks
	newExpiration := st.cs.Height() + 20
	for i := 0; i < 5 && rf.Files[0].Expiration != newExpiration; i++ {
		time.Sleep(1 * time.Second)
		st.getAPI("/renter/files", &rf)
	}
}
