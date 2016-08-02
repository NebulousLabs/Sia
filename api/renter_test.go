package api

import (
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/types"
)

const (
	testFunds  = "10000000000000000000000000000"
	testPeriod = "5"
)

// createRandFile creates a file on disk and fills it with random bytes.
func createRandFile(path string, size int) error {
	data, err := crypto.RandBytes(size)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0600)
}

// TestRenterPaths tests that the /renter routes handle path parameters
// properly.
func TestRenterPaths(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestRenterPaths")
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
	path := filepath.Join(build.SiaTestingDir, "api", "TestRenterPaths", "test.dat")
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
	st, err := createServerTester("TestRenterConflicts")
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
	path := filepath.Join(build.SiaTestingDir, "api", "TestRenterConflicts", "test.dat")
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
	expectedErr := Error{"Upload failed: " + renter.ErrPathOverload.Error()}
	if err != expectedErr {
		t.Fatalf("expected %v, got %v", Error{"Upload failed: " + renter.ErrPathOverload.Error()}, err)
	}

	// Upload using nickname that conflicts with folder.
	err = st.stdPostAPI("/renter/upload/foo/bar", uploadValues)
	if err == nil {
		t.Fatal("expecting conflict error, got nil")
	}
}

// TestRenterHostsActiveHandler checks the behavior of the call to
// /hostdb/active.
func TestRenterHostsActiveHandler(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestRenterHostsActiveHandler")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Try the call with numhosts unset, and set to -1, 0, and 1.
	var ah ActiveHosts
	err = st.getAPI("/hostdb/active", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=-1", &ah)
	if err == nil {
		t.Fatal("expecting an error, got:", err)
	}
	err = st.getAPI("/hostdb/active?numhosts=0", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=1", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}

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

	// Try the call with with numhosts unset, and set to -1, 0, 1, and 2.
	err = st.getAPI("/hostdb/active", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=-1", &ah)
	if err == nil {
		t.Fatal("expecting an error, got:", err)
	}
	err = st.getAPI("/hostdb/active?numhosts=0", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=1", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatal(len(ah.Hosts))
	}
	err = st.getAPI("/hostdb/active?numhosts=2", &ah)
	if err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatal(len(ah.Hosts))
	}
}

// TestRenterHostsAllHandler checks that announcing a host adds it to the list
// of all hosts.
func TestRenterHostsAllHandler(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestRenterHostsAllHandler")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Try the call before any hosts have been declared.
	var ah AllHosts
	if err = st.getAPI("/hostdb/all", &ah); err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 0 {
		t.Fatalf("expected 0 hosts, got %v", len(ah.Hosts))
	}
	// Announce the host and try the call again.
	if err = st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err = st.getAPI("/hostdb/all", &ah); err != nil {
		t.Fatal(err)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}
}

// TestRenterHandlerContracts checks that contract formation between a host and
// renter behaves as expected, and that contract spending is the right amount.
func TestRenterHandlerContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestRenterHandlerContracts")
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
	var expectedContractSpending types.Currency
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

	st, err := createServerTester("TestRenterHandlerGetAndPost")
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
}

// TestRenterLoadNonexistent checks that attempting to upload or download a
// nonexistent file triggers the appropriate error.
func TestRenterLoadNonexistent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestRenterLoadNonexistent")
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
	if err == nil || !strings.HasSuffix(err.Error(), "no such file or directory") {
		t.Errorf("expected error to end with 'no such file or directory; got %v", err)
	}

	// Try downloading a nonexistent file.
	downpath := filepath.Join(st.dir, "dnedown.dat")
	err = st.stdGetAPI("/renter/download/dne?destination=" + downpath)
	if err == nil || err.Error() != "Download failed: no file with that path" {
		t.Errorf("expected error to be 'Download failed: no file with that path'; got %v instead", err)
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
// successful, and that invalid  calls fail with the appropriate error.
func TestRenterHandlerRename(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestRenterHandlerRename")
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

	// Create two files.
	path1 := filepath.Join(st.dir, "test1.dat")
	if err = createRandFile(path1, 512); err != nil {
		t.Fatal(err)
	}
	path2 := filepath.Join(st.dir, "test2.dat")
	if err = createRandFile(path2, 512); err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path1)
	if err = st.stdPostAPI("/renter/upload/test1", uploadValues); err != nil {
		t.Fatal(err)
	}
	uploadValues.Set("source", path2)
	if err = st.stdPostAPI("/renter/upload/test2", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Try renaming to a name that's already taken.
	renameValues := url.Values{}
	renameValues.Set("newsiapath", "test1")
	err = st.stdPostAPI("/renter/rename/test2", renameValues)
	if err == nil || err.Error() != renter.ErrPathOverload.Error() {
		t.Errorf("expected error to be %v; got %v", renter.ErrPathOverload, err)
	}

	// Try renaming to an empty string.
	renameValues.Set("newsiapath", "")
	err = st.stdPostAPI("/renter/rename/test1", renameValues)
	if err == nil || err.Error() != renter.ErrEmptyFilename.Error() {
		t.Fatalf("expected error to be %v; got %v", renter.ErrEmptyFilename, err)
	}

	// Rename the first file.
	renameValues.Set("newsiapath", "newtest1")
	if err = st.stdPostAPI("/renter/rename/test1", renameValues); err != nil {
		t.Fatal(err)
	}

	// Try renaming a nonexistent file.
	renameValues.Set("newsiapath", "newdne")
	err = st.stdPostAPI("/renter/rename/dne", renameValues)
	if err == nil || err.Error() != renter.ErrUnknownPath.Error() {
		t.Errorf("expected error to be %v; got %v", renter.ErrUnknownPath, err)
	}
}

// TestRenterHandlerDelete checks that deleting a valid file from the renter
// goes as planned and that attempting to delete a nonexistent file fails with
// the appropriate error.
func TestRenterHandlerDelete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	st, err := createServerTester("TestRenterHandlerDelete")
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
