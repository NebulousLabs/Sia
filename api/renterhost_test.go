package api

// renterhost_test.go sets up larger integration tests between renters and
// hosts, checking that the whole storage ecosystem is functioning cohesively.

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

// TestHostAndRent sets up an integration test where a host and
// renter do basic uploads and downloads.
func TestHostAndRent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostAndRent")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host and start accepting contracts.
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

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "5"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}

	// Upload the file to the renter.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
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
	// Only one piece will be uploaded (10% at current redundancy).
	for i := 0; i < 200 && (len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0], rf.Files[1])
	}

	// Try downloading the first file.
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err := ioutil.ReadFile(path)
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

	// The renter's downloads queue should have 1 entry now.
	var queue RenterDownloadQueue
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 1 {
		t.Fatalf("expected renter to have 1 download in the queue; got %v", len(queue.Downloads))
	}

	// Try downloading the second file.
	downpath2 := filepath.Join(st.dir, "testdown2.dat")
	err = st.stdGetAPI("/renter/download/test2?destination=" + downpath2)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig2, err := ioutil.ReadFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	download2, err := ioutil.ReadFile(downpath2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig2, download2) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}

	// The renter's downloads queue should have 2 entries now.
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 2 {
		t.Fatalf("expected renter to have 1 download in the queue; got %v", len(queue.Downloads))
	}

	// Mine blocks until the host recognizes profit. The host will wait for 12
	// blocks after the storage window has closed to report the profit, a total
	// of 40 blocks should be mined.
	t.Skip("TODO: NEED TO GET THE CONTRACT STUFF WORKING AGAIN")
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

// TestHostAndRentMultiHost sets up an integration test where three hosts and a
// renter do basic (parallel) uploads and downloads.
func TestHostAndRentMultiHost(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestHostAndRentMultiHost")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	stH1, err := blankServerTester("TestHostAndRentMultiHost - H1")
	if err != nil {
		t.Fatal(err)
	}
	defer stH1.server.Close()
	stH2, err := blankServerTester("TestHostAndRentMultiHost - H2")
	if err != nil {
		t.Fatal(err)
	}
	defer stH2.server.Close()
	testGroup := []*serverTester{st, stH1, stH2}

	// Connect the testers to eachother so that they are all on the same
	// blockchain.
	err = fullyConnectNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure that every wallet has money in it.
	err = fundAllNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Announce every host.
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: Set an allowance that has files being uploaded with 2-of-6
	// redundancy, such that at least 2 hosts are required to upload and
	// download a file. This will do a good job of putting pressure on the
	// parallelization algorithms.

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "5"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}

	// Upload the file to the renter.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
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
	// Only one piece will be uploaded (10% at current redundancy).
	for i := 0; i < 200 && (len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0], rf.Files[1])
	}

	// Try downloading the first file.
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err := ioutil.ReadFile(path)
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

	// The renter's downloads queue should have 1 entry now.
	var queue RenterDownloadQueue
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 1 {
		t.Fatalf("expected renter to have 1 download in the queue; got %v", len(queue.Downloads))
	}

	// Try downloading the second file.
	downpath2 := filepath.Join(st.dir, "testdown2.dat")
	err = st.stdGetAPI("/renter/download/test2?destination=" + downpath2)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig2, err := ioutil.ReadFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	download2, err := ioutil.ReadFile(downpath2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig2, download2) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}

	// The renter's downloads queue should have 2 entries now.
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 2 {
		t.Fatalf("expected renter to have 1 download in the queue; got %v", len(queue.Downloads))
	}

	// Mine blocks until the host recognizes profit. The host will wait for 12
	// blocks after the storage window has closed to report the profit, a total
	// of 40 blocks should be mined.
	t.Skip("TODO: NEED TO GET THE CONTRACT STUFF WORKING AGAIN")
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

// TestUploadDownload tests that downloading and uploading in
// parallel does not result in failures or stalling.
func TestUploadDownload(t *testing.T) {
	t.Skip("uploading to the renter does not work")
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester("TestUploadDownload")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host and start accepting contracts.
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

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "5"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Only one piece will be uploaded (10% at current redundancy).
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// In parallel, upload another file and download the first file.
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
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err := ioutil.ReadFile(path)
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

	// Wait for upload to complete.
	for i := 0; i < 200 && (len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 2 || rf.Files[0].UploadProgress < 10 || rf.Files[1].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0], rf.Files[1])
	}
}
