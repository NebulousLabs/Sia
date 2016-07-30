package api

// renterhost_test.go sets up larger integration tests between renters and
// hosts, checking that the whole storage ecosystem is functioning cohesively.

// TODO: There are a bunch of magic numbers in this file.

import (
	"bytes"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
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

	// The renter should not have any contracts yet.
	var contracts RenterContracts
	if err = st.getAPI("/renter/contracts", &contracts); err != nil {
		t.Fatal(err)
	}
	if len(contracts.Contracts) != 0 {
		t.Fatalf("expected renter to have 0 contracts; got %v", len(contracts.Contracts))
	}

	// The renter's downloads queue should be empty.
	var queue RenterDownloadQueue
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 0 {
		t.Fatalf("expected renter to have 0 downloads in the queue; got %v", len(queue.Downloads))
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

	// The renter should now have 1 contract.
	if err = st.getAPI("/renter/contracts", &contracts); err != nil {
		t.Fatal(err)
	}
	if len(contracts.Contracts) != 1 {
		t.Fatalf("expected renter to have 1 contract; got %v", len(contracts.Contracts))
	}

	// Check that a call to /renter returns the expected values.
	var get RenterGET
	if err = st.getAPI("/renter", &get); err != nil {
		t.Fatal(err)
	}
	// Check the renter's funds.
	expectedFunds, ok := scanAmount(testFunds)
	if !ok {
		t.Fatal("scanAmount failed")
	}
	if got := get.Settings.Allowance.Funds; got.Cmp(expectedFunds) != 0 {
		t.Fatalf("expected funds to be %v; got %v", expectedFunds, got)
	}
	// Check the renter's period.
	intPeriod, err := strconv.Atoi(testPeriod)
	if err != nil {
		t.Fatal(err)
	}
	expectedPeriod := types.BlockHeight(intPeriod)
	if got := get.Settings.Allowance.Period; got != expectedPeriod {
		t.Fatalf("expected period to be %v; got %v", expectedPeriod, got)
	}
	// Check the renter's renew window.
	expectedRenewWindow := expectedPeriod / 2
	if got := get.Settings.Allowance.RenewWindow; got != expectedRenewWindow {
		t.Fatalf("expected renew window to be %v; got %v", expectedRenewWindow, got)
	}
	// Check the renter's contract spending.
	var expectedContractSpending types.Currency
	for _, contract := range contracts.Contracts {
		expectedContractSpending = expectedContractSpending.Add(contract.RenterFunds)
	}
	if got := get.FinancialMetrics.ContractSpending; got.Cmp(expectedContractSpending) != 0 {
		t.Fatalf("expected contract spending to be %v; got %v", expectedContractSpending, got)
	}

	// Check that renterHandlerGET correctly handles invalid inputs.
	// Try an empty funds string.
	allowanceValues.Set("funds", "")
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || err.Error() != "Couldn't parse funds" {
		t.Errorf("expected error to be 'Couldn't parse funds'; got %v", err)
	}
	// Try an invalid funds string. Can't test a negative value since
	// ErrNegativeCurrency triggers a build.Critical, which calls a panic in
	// debug mode.
	allowanceValues.Set("funds", "0")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || err.Error() != contractor.ErrInsufficientAllowance.Error() {
		t.Errorf("expected error to be %v; got %v", contractor.ErrInsufficientAllowance, err)
	}
	// Try a empty period string.
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", "")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || !strings.HasPrefix(err.Error(), "Couldn't parse period: ") {
		t.Errorf("expected error to begin with 'Couldn't parse period: '; got %v", err)
	}
	// Try an invalid period string.
	allowanceValues.Set("period", "-1")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || err.Error()[:23] != "Couldn't parse period: " {
		t.Errorf("expected error to begin with 'Couldn't parse period: '; got %v", err)
	}
	// Try a period that will lead to a length-zero RenewWindow.
	allowanceValues.Set("period", "1")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || err.Error() != contractor.ErrAllowanceZeroWindow.Error() {
		t.Errorf("expected error to be %v, got %v", contractor.ErrAllowanceZeroWindow, err)
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
	// Try uploading a nonexistent file.
	path = filepath.Join(st.dir, "fake.dat")
	uploadValues = url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/fake", uploadValues)
	if err == nil || !strings.HasSuffix(err.Error(), "no such file or directory") {
		t.Errorf("expected error to end with 'no such file or directory'; got %v", err)
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

	// Try downloading a nonexistent file.
	err = st.stdGetAPI("/renter/download/fake?destination=" + downpath)
	if err == nil || err.Error() != "Download failed: no file with that path" {
		t.Errorf("expected error to be 'Download failed: no file with that path'; got %v instead", err)
	}

	// The downloads queue should now contain the second file's download.
	// Downloads are never removed from the queue within a siad session.
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 1 {
		t.Fatalf("expected renter to have 1 download in the queue; got %v", len(queue.Downloads))
	}

	// Rename the second file's entry in the renter's list of files.
	renameValues := url.Values{}
	renameValues.Set("newsiapath", "newtest2")
	if err = st.stdPostAPI("/renter/rename/test2", renameValues); err != nil {
		t.Fatal(err)
	}
	// Try renaming a nonexistent file.
	renameValues.Set("newsiapath", "newfake")
	err = st.stdPostAPI("/renter/rename/fake", renameValues)
	if err == nil || err.Error() != renter.ErrUnknownPath.Error() {
		t.Errorf("expected %v, got %v", renter.ErrUnknownPath, err)
	}
	// Try renaming the first file to a name that's already taken.
	renameValues.Set("newsiapath", "newtest2")
	err = st.stdPostAPI("/renter/rename/test", renameValues)
	if err == nil || err.Error() != renter.ErrPathOverload.Error() {
		t.Errorf("expected error to be %v, got %v", renter.ErrPathOverload, err)
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

	// Delete both files.
	if err = st.stdPostAPI("/renter/delete/test", url.Values{}); err != nil {
		t.Fatal(err)
	}
	if err = st.stdPostAPI("/renter/delete/newtest2", url.Values{}); err != nil {
		t.Fatal(err)
	}
	// The renter's list of files should now be empty.
	var files RenterFiles
	if err = st.getAPI("/renter/files", &files); err != nil {
		t.Fatal(err)
	}
	if len(files.Files) != 0 {
		t.Fatalf("renter's list of files should be empty; got %v instead", files)
	}
	// Try deleting a nonexistent file.
	if err = st.stdPostAPI("/renter/delete/dne", url.Values{}); err.Error() != renter.ErrUnknownPath.Error() {
		t.Errorf("expected error to be %v, got %v", renter.ErrUnknownPath, err)
	}
}
