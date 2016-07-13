package api

// renterhost_test.go sets up larger integration tests between renters and
// hosts, checking that the whole storage ecosystem is functioning cohesively.

// TODO: There are a bunch of magic numbers in this file.

import (
	"bytes"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationHostAndRent sets up an integration test where a host and
// renter participate in all of the actions related to simple renting and
// hosting.
func TestIntegrationHostAndRent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationHostAndRent")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// announce the host and start accepting contracts
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	err = st.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = st.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}

	// create contracts
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "10000000000000000000000000000") // 10k SC
	allowanceValues.Set("period", "5")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// create a file
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}

	// upload to host
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// On a second connection, upload another file.
	path2 := filepath.Join(st.dir, "test2.dat")
	test2Size := modules.SectorSize*2 + 1
	err = createRandFile(path2, int(test2Size))
	if err != nil {
		t.Fatal(err)
	}
	uploadValues = url.Values{}
	uploadValues.Set("source", path2)
	err = st.stdPostAPI("/renter/upload/test2", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	for i := 0; i < 200 && (len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0], rf.Files[1])
	}

	// Try downloading the second file.
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test2?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err := ioutil.ReadFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	download, err := ioutil.ReadFile(downpath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig, download) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}

	// Mine blocks until the host recognizes profit. The host will wait for 12
	// blocks after the storage window has closed to report the profit, a total
	// of 40 blocks should be mined.
	for i := 0; i < 40; i++ {
		st.miner.AddBlock()
	}
	// Check that the host is reporting a profit.
	var hg HostGET
	st.getAPI("/host", &hg)
	if hg.FinancialMetrics.StorageRevenue.Cmp(types.ZeroCurrency) <= 0 ||
		hg.FinancialMetrics.DownloadBandwidthRevenue.Cmp(types.ZeroCurrency) <= 0 {
		t.Log("Storage Revenue:", hg.FinancialMetrics.StorageRevenue)
		t.Log("Bandwidth Revenue:", hg.FinancialMetrics.DownloadBandwidthRevenue)
		t.Log("Full Financial Metrics:", hg.FinancialMetrics)
		t.Fatal("Host is not displaying revenue after resolving a storage proof.")
	}
}
