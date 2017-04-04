package api

import (
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

const (
	testFunds  = "10000000000000000000000000000"
	testPeriod = "5"
)

// createRandFile creates a file on disk and fills it with random bytes.
func createRandFile(path string, size int) error {
	return ioutil.WriteFile(path, fastrand.Bytes(size), 0600)
}

// TestRenterDownloadError tests that the /renter/download route sets the download's error field if it fails.
func TestRenterDownloadError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
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
	testPeriod := "10"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(build.SiaTestingDir, "api", t.Name(), "test.dat")
	err = createRandFile(path, 1e4)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/test.dat", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// don't wait for the upload to complete, try to download immediately to intentionally cause a download error
	downpath := filepath.Join(st.dir, "down.dat")
	expectedErr := st.getAPI("/renter/download/test.dat?destination="+downpath, nil)
	if expectedErr == nil {
		t.Fatal("download unexpectedly succeeded")
	}

	// verify the file has the expected error
	var rdq RenterDownloadQueue
	err = st.getAPI("/renter/downloads", &rdq)
	if err != nil {
		t.Fatal(err)
	}
	for _, download := range rdq.Downloads {
		if download.SiaPath == "test.dat" && download.Received == download.Filesize && download.Error == expectedErr.Error() {
			t.Fatal("download had unexpected error: ", download.Error)
		}
	}
}

// TestRenterAsyncDownloadError tests that the /renter/asyncdownload route sets the download's error field if it fails.
func TestRenterAsyncDownloadError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
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
	testPeriod := "10"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(build.SiaTestingDir, "api", t.Name(), "test.dat")
	err = createRandFile(path, 1e4)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/test.dat", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// don't wait for the upload to complete, try to download immediately to intentionally cause a download error
	downpath := filepath.Join(st.dir, "asyncdown.dat")
	err = st.getAPI("/renter/downloadasync/test.dat?destination="+downpath, nil)
	if err != nil {
		t.Fatal(err)
	}

	// verify the file has an error
	var rdq RenterDownloadQueue
	err = st.getAPI("/renter/downloads", &rdq)
	if err != nil {
		t.Fatal(err)
	}
	for _, download := range rdq.Downloads {
		if download.SiaPath == "test.dat" && download.Received == download.Filesize && download.Error == "" {
			t.Fatal("download had nil error")
		}
	}
}

// TestRenterAsyncDownload tests that the /renter/downloadasync route works
// correctly.
func TestRenterAsyncDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
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
	testPeriod := "10"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(build.SiaTestingDir, "api", t.Name(), "test.dat")
	err = createRandFile(path, 1e4)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/test.dat", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// wait for the file to become available
	var rf RenterFiles
	for i := 0; i < 100 && (len(rf.Files) != 1 || !rf.Files[0].Available); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || !rf.Files[0].Available {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// download the file asynchronously
	downpath := filepath.Join(st.dir, "asyncdown.dat")
	err = st.getAPI("/renter/downloadasync/test.dat?destination="+downpath, nil)
	if err != nil {
		t.Fatal(err)
	}

	// verify the file is not currently downloaded
	var rdq RenterDownloadQueue
	err = st.getAPI("/renter/downloads", &rdq)
	if err != nil {
		t.Fatal(err)
	}
	for _, download := range rdq.Downloads {
		if download.SiaPath == "test.dat" && download.Received == download.Filesize {
			t.Fatal("download finished prematurely")
		}
	}

	// download should eventually complete
	success := false
	for start := time.Now(); time.Since(start) < 30*time.Second; time.Sleep(time.Millisecond * 10) {
		err = st.getAPI("/renter/downloads", &rdq)
		if err != nil {
			t.Fatal(err)
		}
		for _, download := range rdq.Downloads {
			if download.Received == download.Filesize && download.SiaPath == "test.dat" {
				success = true
			}
		}
		if success {
			break
		}
	}
	if !success {
		t.Fatal("/renter/downloadasync did not download our test file")
	}
}

// TestRenterPaths tests that the /renter routes handle path parameters
// properly.
func TestRenterPaths(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(build.SiaTestingDir, "api", t.Name(), "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/foo/bar/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// File should be listed by the renter.
	var rf RenterFiles
	err = st.getAPI("/renter/files", &rf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Files) != 1 || rf.Files[0].SiaPath != "foo/bar/test" {
		t.Fatal("/renter/files did not return correct file:", rf)
	}
}

// TestRenterConflicts tests that the renter handles naming conflicts properly.
func TestRenterConflicts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(build.SiaTestingDir, "api", t.Name(), "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host, using a path designed to cause conflicts. The renter
	// should automatically create a folder called foo/bar.sia. Later, we'll
	// exploit this by uploading a file called foo/bar.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	err = st.stdPostAPI("/renter/upload/foo/bar.sia/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// File should be listed by the renter.
	var rf RenterFiles
	err = st.getAPI("/renter/files", &rf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Files) != 1 || rf.Files[0].SiaPath != "foo/bar.sia/test" {
		t.Fatal("/renter/files did not return correct file:", rf)
	}

	// Upload using the same nickname.
	err = st.stdPostAPI("/renter/upload/foo/bar.sia/test", uploadValues)
	expectedErr := Error{"upload failed: " + renter.ErrPathOverload.Error()}
	if err != expectedErr {
		t.Fatalf("expected %v, got %v", Error{"upload failed: " + renter.ErrPathOverload.Error()}, err)
	}

	// Upload using nickname that conflicts with folder.
	err = st.stdPostAPI("/renter/upload/foo/bar", uploadValues)
	if err == nil {
		t.Fatal("expecting conflict error, got nil")
	}
}

// TestRenterHandlerContracts checks that contract formation between a host and
// renter behaves as expected, and that contract spending is the right amount.
func TestRenterHandlerContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
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

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// The renter should now have 1 contract.
	if err = st.getAPI("/renter/contracts", &contracts); err != nil {
		t.Fatal(err)
	}
	if len(contracts.Contracts) != 1 {
		t.Fatalf("expected renter to have 1 contract; got %v", len(contracts.Contracts))
	}

	// Check the renter's contract spending.
	var get RenterGET
	if err = st.getAPI("/renter", &get); err != nil {
		t.Fatal(err)
	}
	expectedContractSpending := get.Settings.Allowance.Funds.Sub(get.FinancialMetrics.Unspent)
	for _, contract := range contracts.Contracts {
		expectedContractSpending = expectedContractSpending.Add(contract.RenterFunds)
	}
	if got := get.FinancialMetrics.ContractSpending; got.Cmp(expectedContractSpending) != 0 {
		t.Fatalf("expected contract spending to be %v; got %v", expectedContractSpending, got)
	}
}

// TestRenterHandlerGetAndPost checks that valid /renter calls successfully set
// allowance values, while /renter calls with invalid allowance values are
// correctly handled.
func TestRenterHandlerGetAndPost(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
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

	// Try an empty funds string.
	allowanceValues = url.Values{}
	allowanceValues.Set("funds", "")
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || err.Error() != "unable to parse funds" {
		t.Errorf("expected error to be 'unable to parse funds'; got %v", err)
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
	if err == nil || !strings.HasPrefix(err.Error(), "unable to parse period: ") {
		t.Errorf("expected error to begin with 'unable to parse period: '; got %v", err)
	}
	// Try an invalid period string.
	allowanceValues.Set("period", "-1")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || !strings.Contains(err.Error(), "unable to parse period") {
		t.Errorf("expected error to begin with 'unable to parse period'; got %v", err)
	}
	// Try a period that will lead to a length-zero RenewWindow.
	allowanceValues.Set("period", "1")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || err.Error() != contractor.ErrAllowanceZeroWindow.Error() {
		t.Errorf("expected error to be %v, got %v", contractor.ErrAllowanceZeroWindow, err)
	}
}

// TestRenterLoadNonexistent checks that attempting to upload or download a
// nonexistent file triggers the appropriate error.
func TestRenterLoadNonexistent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Try uploading a nonexistent file.
	fakepath := filepath.Join(st.dir, "dne.dat")
	uploadValues := url.Values{}
	uploadValues.Set("source", fakepath)
	err = st.stdPostAPI("/renter/upload/dne", uploadValues)
	if err == nil {
		t.Errorf("expected error when uploading nonexistent file")
	}

	// Try downloading a nonexistent file.
	downpath := filepath.Join(st.dir, "dnedown.dat")
	err = st.stdGetAPI("/renter/download/dne?destination=" + downpath)
	if err == nil || err.Error() != "download failed: no file with that path" {
		t.Errorf("expected error to be 'download failed: no file with that path'; got %v instead", err)
	}

	// The renter's downloads queue should be empty.
	var queue RenterDownloadQueue
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 0 {
		t.Fatalf("expected renter to have 0 downloads in the queue; got %v", len(queue.Downloads))
	}
}

// TestRenterHandlerRename checks that valid /renter/rename calls are
// successful, and that invalid calls fail with the appropriate error.
func TestRenterHandlerRename(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Try renaming a nonexistent file.
	renameValues := url.Values{}
	renameValues.Set("newsiapath", "newdne")
	err = st.stdPostAPI("/renter/rename/dne", renameValues)
	if err == nil || err.Error() != renter.ErrUnknownPath.Error() {
		t.Errorf("expected error to be %v; got %v", renter.ErrUnknownPath, err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path1 := filepath.Join(st.dir, "test1.dat")
	if err = createRandFile(path1, 512); err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path1)
	if err = st.stdPostAPI("/renter/upload/test1", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Try renaming to an empty string.
	renameValues.Set("newsiapath", "")
	err = st.stdPostAPI("/renter/rename/test1", renameValues)
	if err == nil || err.Error() != renter.ErrEmptyFilename.Error() {
		t.Fatalf("expected error to be %v; got %v", renter.ErrEmptyFilename, err)
	}

	// Rename the file.
	renameValues.Set("newsiapath", "newtest1")
	if err = st.stdPostAPI("/renter/rename/test1", renameValues); err != nil {
		t.Fatal(err)
	}

	// Should be able to continue uploading and downloading using the new name.
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("upload is not succeeding:", rf.Files[0])
	}
	err = st.stdGetAPI("/renter/download/newtest1?destination=" + filepath.Join(st.dir, "testdown2.dat"))
	if err != nil {
		t.Fatal(err)
	}

	// Create and upload another file.
	path2 := filepath.Join(st.dir, "test2.dat")
	if err = createRandFile(path2, 512); err != nil {
		t.Fatal(err)
	}
	uploadValues.Set("source", path2)
	if err = st.stdPostAPI("/renter/upload/test2", uploadValues); err != nil {
		t.Fatal(err)
	}
	// Try renaming to a name that's already taken.
	renameValues.Set("newsiapath", "newtest1")
	err = st.stdPostAPI("/renter/rename/test2", renameValues)
	if err == nil || err.Error() != renter.ErrPathOverload.Error() {
		t.Errorf("expected error to be %v; got %v", renter.ErrPathOverload, err)
	}
}

// TestRenterHandlerDelete checks that deleting a valid file from the renter
// goes as planned and that attempting to delete a nonexistent file fails with
// the appropriate error.
func TestRenterHandlerDelete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	if err = createRandFile(path, 1024); err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	if err = st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Delete the file.
	if err = st.stdPostAPI("/renter/delete/test", url.Values{}); err != nil {
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
	err = st.stdPostAPI("/renter/delete/dne", url.Values{})
	if err == nil || err.Error() != renter.ErrUnknownPath.Error() {
		t.Errorf("expected error to be %v, got %v", renter.ErrUnknownPath, err)
	}
}

// Tests that the /renter/upload call checks for relative paths.
func TestRenterRelativePathErrorUpload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	renterUploadAbsoluteError := "source must be an absolute path"

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	if err = createRandFile(path, 1024); err != nil {
		t.Fatal(err)
	}

	// This should fail.
	uploadValues := url.Values{}
	uploadValues.Set("source", "test.dat")
	if err = st.stdPostAPI("/renter/upload/test", uploadValues); err.Error() != renterUploadAbsoluteError {
		t.Fatal(err)
	}

	// As should this.
	uploadValues = url.Values{}
	uploadValues.Set("source", "../test.dat")
	if err = st.stdPostAPI("/renter/upload/test", uploadValues); err.Error() != renterUploadAbsoluteError {
		t.Fatal(err)
	}

	// This should succeed.
	uploadValues = url.Values{}
	uploadValues.Set("source", path)
	if err = st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}
}

// Tests that the /renter/download call checks for relative paths.
func TestRenterRelativePathErrorDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Anounce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err = st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	renterDownloadAbsoluteError := "destination must be an absolute path"

	// Create a file, and upload it.
	path := filepath.Join(st.dir, "test.dat")
	if err = createRandFile(path, 1024); err != nil {
		t.Fatal(err)
	}
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	if err = st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}
	var rf RenterFiles
	for i := 0; i < 100 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(200 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0], rf.Files[1])
	}

	// Use a relative destination, which should fail.
	downloadPath := "test1.dat"
	if err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath); err.Error() != renterDownloadAbsoluteError {
		t.Fatal(err)
	}

	// Relative destination stepping backwards should also fail.
	downloadPath = "../test1.dat"
	if err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath); err.Error() != renterDownloadAbsoluteError {
		t.Fatal(err)
	}

	// Long relative destination should also fail (just missing leading slash).
	downloadPath = filepath.Join(st.dir[1:], "test1.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err == nil {
		t.Fatal("expecting an error")
	}

	// Full destination should succeed.
	downloadPath = filepath.Join(st.dir, "test1.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err != nil {
		t.Fatal("expecting an error")
	}
}

// TestRenterPricesHandler checks that the prices command returns reasonable
// values given the settings of the hosts.
func TestRenterPricesHandler(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host and then get the calculated prices for when there is a
	// single host.
	var rpeSingle modules.RenterPriceEstimation
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/renter/prices", &rpeSingle); err != nil {
		t.Fatal(err)
	}

	// Create several more hosts all using the default settings.
	stHost1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}

	// Connect all the nodes and announce all of the hosts.
	sts := []*serverTester{st, stHost1, stHost2}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Grab the price estimates for when there are a bunch of hosts with the
	// same stats.
	var rpeMulti modules.RenterPriceEstimation
	if err = st.getAPI("/renter/prices", &rpeMulti); err != nil {
		t.Fatal(err)
	}

	// Verify that the aggregate is the same.
	if !rpeMulti.DownloadTerabyte.Equals(rpeSingle.DownloadTerabyte) {
		t.Log(rpeMulti.DownloadTerabyte)
		t.Log(rpeSingle.DownloadTerabyte)
		t.Error("price changed from single to multi")
	}
	if !rpeMulti.FormContracts.Equals(rpeSingle.FormContracts) {
		t.Error("price changed from single to multi")
	}
	if !rpeMulti.StorageTerabyteMonth.Equals(rpeSingle.StorageTerabyteMonth) {
		t.Error("price changed from single to multi")
	}
	if !rpeMulti.UploadTerabyte.Equals(rpeSingle.UploadTerabyte) {
		t.Error("price changed from single to multi")
	}
}

// TestRenterPricesHandlerCheap checks that the prices command returns
// reasonable values given the settings of the hosts.
func TestRenterPricesHandlerCheap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host and then get the calculated prices for when there is a
	// single host.
	var rpeSingle modules.RenterPriceEstimation
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/renter/prices", &rpeSingle); err != nil {
		t.Fatal(err)
	}

	// Create several more hosts all using the default settings.
	stHost1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}

	var hg HostGET
	err = st.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost1.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost2.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}

	// Set host 5 to be cheaper than the rest by a substantial amount. This
	// should result in a reduction for the price estimation.
	vals := url.Values{}
	vals.Set("mincontractprice", "1")
	vals.Set("mindownloadbandwidthprice", "1")
	vals.Set("minstorageprice", "1")
	vals.Set("minuploadbandwidthprice", "1")
	err = stHost2.stdPostAPI("/host", vals)
	if err != nil {
		t.Fatal(err)
	}

	// Connect all the nodes and announce all of the hosts.
	sts := []*serverTester{st, stHost1, stHost2}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Grab the price estimates for when there are a bunch of hosts with the
	// same stats.
	var rpeMulti modules.RenterPriceEstimation
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/renter/prices", &rpeMulti); err != nil {
		t.Fatal(err)
	}

	// Verify that the aggregate is the same.
	if !(rpeMulti.DownloadTerabyte.Cmp(rpeSingle.DownloadTerabyte) < 0) {
		t.Log(rpeMulti.DownloadTerabyte)
		t.Log(rpeSingle.DownloadTerabyte)
		t.Error("price did not drop from single to multi")
	}
	if !(rpeMulti.FormContracts.Cmp(rpeSingle.FormContracts) < 0) {
		t.Error("price did not drop from single to multi")
	}
	if !(rpeMulti.StorageTerabyteMonth.Cmp(rpeSingle.StorageTerabyteMonth) < 0) {
		t.Error("price did not drop from single to multi")
	}
	if !(rpeMulti.UploadTerabyte.Cmp(rpeSingle.UploadTerabyte) < 0) {
		t.Error("price did not drop from single to multi")
	}
}

// TestRenterPricesHandlerIgnorePricey checks that the prices command returns
// reasonable values given the settings of the hosts.
func TestRenterPricesHandlerIgnorePricey(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host and then get the calculated prices for when there is a
	// single host.
	var rpeSingle modules.RenterPriceEstimation
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/renter/prices", &rpeSingle); err != nil {
		t.Fatal(err)
	}

	// Create several more hosts all using the default settings.
	stHost1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	stHost3, err := blankServerTester(t.Name() + " - Host 3")
	if err != nil {
		t.Fatal(err)
	}
	stHost4, err := blankServerTester(t.Name() + " - Host 4")
	if err != nil {
		t.Fatal(err)
	}
	stHost5, err := blankServerTester(t.Name() + " - Host 5")
	if err != nil {
		t.Fatal(err)
	}

	// Set host 5 to be cheaper than the rest by a substantial amount. This
	// should result in a reduction for the price estimation.
	vals := url.Values{}
	vals.Set("mindownloadbandwidthprice", "100000000000000000000")
	vals.Set("mincontractprice", "1000000000000000000000000000")
	vals.Set("minstorageprice", "100000000000000000000")
	vals.Set("minuploadbandwidthprice", "100000000000000000000")
	err = stHost5.stdPostAPI("/host", vals)
	if err != nil {
		t.Fatal(err)
	}

	// Connect all the nodes and announce all of the hosts.
	sts := []*serverTester{st, stHost1, stHost2, stHost3, stHost4, stHost5}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Grab the price estimates for when there are a bunch of hosts with the
	// same stats.
	var rpeMulti modules.RenterPriceEstimation
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/renter/prices", &rpeMulti); err != nil {
		t.Fatal(err)
	}

	// Verify that the aggregate is the same - price should not have moved
	// because the expensive host will be ignored as there is only one.
	if !rpeMulti.DownloadTerabyte.Equals(rpeSingle.DownloadTerabyte) {
		t.Log(rpeMulti.DownloadTerabyte)
		t.Log(rpeSingle.DownloadTerabyte)
		t.Error("price changed from single to multi")
	}
	if !rpeMulti.FormContracts.Equals(rpeSingle.FormContracts) {
		t.Error("price changed from single to multi")
	}
	if !rpeMulti.StorageTerabyteMonth.Equals(rpeSingle.StorageTerabyteMonth) {
		t.Error("price changed from single to multi")
	}
	if !rpeMulti.UploadTerabyte.Equals(rpeSingle.UploadTerabyte) {
		t.Error("price changed from single to multi")
	}
}

// TestRenterPricesHandlerPricey checks that the prices command returns
// reasonable values given the settings of the hosts.
func TestRenterPricesHandlerPricey(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host and then get the calculated prices for when there is a
	// single host.
	var rpeSingle modules.RenterPriceEstimation
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/renter/prices", &rpeSingle); err != nil {
		t.Fatal(err)
	}

	// Create several more hosts all using the default settings.
	stHost1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}

	var hg HostGET
	err = st.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost1.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}
	err = stHost2.getAPI("/host", &hg)
	if err != nil {
		t.Fatal(err)
	}

	// Set host 5 to be cheaper than the rest by a substantial amount. This
	// should result in a reduction for the price estimation.
	vals := url.Values{}
	vals.Set("mindownloadbandwidthprice", "100000000000000000000")
	vals.Set("mincontractprice", "1000000000000000000000000000")
	vals.Set("minstorageprice", "100000000000000000000")
	vals.Set("minuploadbandwidthprice", "100000000000000000000")
	err = stHost2.stdPostAPI("/host", vals)
	if err != nil {
		t.Fatal(err)
	}

	// Connect all the nodes and announce all of the hosts.
	sts := []*serverTester{st, stHost1, stHost2}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Grab the price estimates for when there are a bunch of hosts with the
	// same stats.
	var rpeMulti modules.RenterPriceEstimation
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/renter/prices", &rpeMulti); err != nil {
		t.Fatal(err)
	}

	// Verify that the aggregate is the same.
	if !(rpeMulti.DownloadTerabyte.Cmp(rpeSingle.DownloadTerabyte) > 0) {
		t.Error("price did not drop from single to multi")
	}
	if !(rpeMulti.FormContracts.Cmp(rpeSingle.FormContracts) > 0) {
		t.Log(rpeMulti.FormContracts)
		t.Log(rpeSingle.FormContracts)
		t.Error("price did not drop from single to multi")
	}
	if !(rpeMulti.StorageTerabyteMonth.Cmp(rpeSingle.StorageTerabyteMonth) > 0) {
		t.Error("price did not drop from single to multi")
	}
	if !(rpeMulti.UploadTerabyte.Cmp(rpeSingle.UploadTerabyte) > 0) {
		t.Error("price did not drop from single to multi")
	}
}
