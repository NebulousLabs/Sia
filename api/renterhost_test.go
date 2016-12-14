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
	t.Skip("failing due to contractor changes")
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationHostAndRent")
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

	// The renter's downloads queue should have 1 entry now.
	var queue RenterDownloadQueue
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 1 {
		t.Fatalf("expected renter to have 1 download in the queue; got %v", len(queue.Downloads))
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

// TestIntegrationUploadDownload tests that downloading and uploading in
// parallel does not result in failures or stalling.
func TestIntegrationUploadDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationUploadDownload")
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

// TestIntegrationRenterMetrics tests that the renter's metrics are properly
// reported after uploading and downloading.
func TestIntegrationRenterMetrics(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestIntegrationRenterMetrics")
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

	// Renter metrics should be zero.
	var metrics RenterGET
	err = st.getAPI("/renter", &metrics)
	if err != nil {
		t.Fatal(err)
	}
	if !metrics.FinancialMetrics.ContractSpending.IsZero() || len(metrics.ContractMetrics) > 0 {
		t.Error("renter should not have formed any contracts yet:", metrics.FinancialMetrics)
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

	// Metrics should reflect new allowance.
	err = st.getAPI("/renter", &metrics)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.FinancialMetrics.ContractSpending.IsZero() || len(metrics.ContractMetrics) == 0 {
		t.Error("renter metrics should reflect contract spending:", metrics.FinancialMetrics)
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

	// Metrics should reflect upload spending and storage spending.
	err = st.getAPI("/renter", &metrics)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.FinancialMetrics.UploadSpending.IsZero() || !metrics.FinancialMetrics.UploadSpending.Equals(metrics.ContractMetrics[0].UploadSpending) {
		t.Error("renter metrics should reflect upload spending:", metrics.FinancialMetrics)
	}
	if metrics.FinancialMetrics.StorageSpending.IsZero() || !metrics.FinancialMetrics.StorageSpending.Equals(metrics.ContractMetrics[0].StorageSpending) {
		t.Error("renter metrics should reflect storage spending:", metrics.FinancialMetrics)
	}

	// Download the first file.
	err = st.stdGetAPI("/renter/download/test?destination=" + filepath.Join(st.dir, "testdown.dat"))
	if err != nil {
		t.Fatal(err)
	}

	// Metrics should reflect upload spending and storage spending.
	err = st.getAPI("/renter", &metrics)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.FinancialMetrics.DownloadSpending.IsZero() || !metrics.FinancialMetrics.DownloadSpending.Equals(metrics.ContractMetrics[0].DownloadSpending) {
		t.Error("renter metrics should reflect download spending:", metrics.FinancialMetrics)
	}
}
