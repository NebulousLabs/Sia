package api

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/host/contractmanager"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// Various folder sizes for testing host storage folder resizing.
	// Must be provided as strings to the API call.
	minFolderSizeString    = strconv.FormatUint(modules.SectorSize*contractmanager.MinimumSectorsPerStorageFolder, 10)
	maxFolderSizeString    = strconv.FormatUint(modules.SectorSize*contractmanager.MaximumSectorsPerStorageFolder, 10)
	tooSmallFolderString   = strconv.FormatUint(modules.SectorSize*(contractmanager.MinimumSectorsPerStorageFolder-1), 10)
	tooLargeFolderString   = strconv.FormatUint(modules.SectorSize*(contractmanager.MaximumSectorsPerStorageFolder+1), 10)
	mediumSizeFolderString = strconv.FormatUint(modules.SectorSize*contractmanager.MinimumSectorsPerStorageFolder*3, 10)

	// Test cases for resizing a host's storage folder.
	// Running all the invalid cases before the valid ones simplifies some
	// logic in the tests that use resizeTests.
	resizeTests = []struct {
		sizeString string
		size       uint64
		err        error
	}{
		// invalid sizes
		{"", 0, io.EOF},
		{"0", 0, contractmanager.ErrSmallStorageFolder},
		{tooSmallFolderString, modules.SectorSize * (contractmanager.MinimumSectorsPerStorageFolder - 1), contractmanager.ErrSmallStorageFolder},
		{tooLargeFolderString, modules.SectorSize * (contractmanager.MaximumSectorsPerStorageFolder + 1), contractmanager.ErrLargeStorageFolder},

		// valid sizes
		//
		// TODO: Re-enable these when the host can support resizing into the
		// same folder.
		//
		// {minFolderSizeString, contractmanager.MinimumSectorsPerStorageFolder * modules.SectorSize, nil},
		// {maxFolderSizeString, contractmanager.MaximumSectorsPerStorageFolder * modules.SectorSize, nil},
		// {mediumSizeFolderString, 3 * contractmanager.MinimumSectorsPerStorageFolder * modules.SectorSize, nil},
	}
)

// TestEstimateWeight tests that /host/estimatescore works correctly.
func TestEstimateWeight(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// announce a host, create an allowance, upload some data.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err := st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	var eg HostEstimateScoreGET
	if err := st.getAPI("/host/estimatescore", &eg); err != nil {
		t.Fatal(err)
	}
	originalEstimate := eg.EstimatedScore

	// verify that the estimate is being correctly updated by setting a massively
	// increased min contract price and verifying that the score decreases.
	is := st.host.InternalSettings()
	is.MinContractPrice = is.MinContractPrice.Add(types.SiacoinPrecision.Mul64(9999999999))
	if err := st.host.SetInternalSettings(is); err != nil {
		t.Fatal(err)
	}
	if err := st.getAPI("/host/estimatescore", &eg); err != nil {
		t.Fatal(err)
	}
	if eg.EstimatedScore.Cmp(originalEstimate) != -1 {
		t.Fatal("score estimate did not decrease after incrementing mincontractprice")
	}

	// add a few hosts to the hostdb and verify that the conversion rate is
	// reflected correctly
	st2, err := blankServerTester(t.Name() + "-st2")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.panicClose()
	st3, err := blankServerTester(t.Name() + "-st3")
	if err != nil {
		t.Fatal(err)
	}
	defer st3.panicClose()
	st4, err := blankServerTester(t.Name() + "-st4")
	if err != nil {
		t.Fatal(err)
	}
	defer st4.panicClose()
	sts := []*serverTester{st, st2, st3, st4}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	for i, tester := range sts {
		is = tester.host.InternalSettings()
		is.MinContractPrice = types.SiacoinPrecision.Mul64(1000 + (1000 * uint64(i)))
		err = tester.host.SetInternalSettings(is)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = announceAllHosts(sts)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		price             types.Currency
		minConversionRate float64
	}{
		{types.SiacoinPrecision, 100},
		{types.SiacoinPrecision.Mul64(50), 98},
		{types.SiacoinPrecision.Mul64(2500), 50},
		{types.SiacoinPrecision.Mul64(3000), 10},
		{types.SiacoinPrecision.Mul64(30000), 0.00001},
	}
	for i, test := range tests {
		err = st.getAPI(fmt.Sprintf("/host/estimatescore?mincontractprice=%v", test.price.String()), &eg)
		if err != nil {
			t.Fatal("test", i, "failed:", err)
		}
		if eg.ConversionRate < test.minConversionRate {
			t.Fatalf("test %v: incorrect conversion rate: got %v wanted %v\n", i, eg.ConversionRate, test.minConversionRate)
		}
	}
}

// TestHostSettingsHandlerParsing verifies that providing invalid host settings
// doesn't reset the host's settings.
func TestHostSettingsHandlerParsing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	settings := st.host.InternalSettings()
	settingsValues := url.Values{}
	settingsValues.Set("maxdownloadbatchsize", "foo")
	st.stdPostAPI("/host", settingsValues)
	newSettings := st.host.InternalSettings()
	if !reflect.DeepEqual(newSettings, settings) {
		t.Fatal("invalid acceptingcontracts value changed host settings! got", newSettings, "wanted", settings)
	}
}

// TestWorkingStatus tests that the host's WorkingStatus field is set
// correctly.
func TestWorkingStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// announce a host, create an allowance, upload some data.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err := st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Block until the allowance has finished forming contracts.
	err = build.Retry(50, time.Millisecond*250, func() error {
		var rc RenterContracts
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 1 {
			return errors.New("no contracts")
		}
		return nil
	})
	if err != nil {
		t.Fatal("allowance setting failed")
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	fileBytes := 1024
	if err := createRandFile(path, fileBytes); err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	if err := st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Only one piece will be uploaded (10% at current redundancy)
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Error(rf.Files[0].UploadProgress)
		t.Fatal("uploading has failed")
	}

	err = build.Retry(30, time.Second, func() error {
		var hg HostGET
		st.getAPI("/host", &hg)

		if hg.WorkingStatus != modules.HostWorkingStatusWorking {
			return errors.New("expected host to be working")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestConnectabilityStatus tests that the host's ConnectabilityStatus field is
// set correctly.
func TestConnectabilityStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create and announce a host
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}

	err = build.Retry(30, time.Second, func() error {
		var hg HostGET
		st.getAPI("/host", &hg)

		if hg.ConnectabilityStatus != modules.HostConnectabilityStatusConnectable {
			return errors.New("expected host to be connectable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStorageHandler tests that host storage is being reported correctly.
func TestStorageHandler(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err := st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Block until the allowance has finished forming contracts.
	err = build.Retry(50, time.Millisecond*250, func() error {
		var rc RenterContracts
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 1 {
			return errors.New("no contracts")
		}
		return nil
	})
	if err != nil {
		t.Fatal("allowance setting failed")
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	fileBytes := 1024
	if err := createRandFile(path, fileBytes); err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	if err := st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Only one piece will be uploaded (10% at current redundancy)
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Error(rf.Files[0].UploadProgress)
		t.Fatal("uploading has failed")
	}

	var sg StorageGET
	if err := st.getAPI("/host/storage", &sg); err != nil {
		t.Fatal(err)
	}

	// Uploading succeeded, so /host/storage should be reporting a successful
	// write.
	if sg.Folders[0].SuccessfulWrites != 1 {
		t.Fatalf("expected 1 successful write, got %v", sg.Folders[0].SuccessfulWrites)
	}
	if used := sg.Folders[0].Capacity - sg.Folders[0].CapacityRemaining; used != modules.SectorSize {
		t.Fatalf("expected used capacity to be the size of one sector (%v bytes), got %v bytes", modules.SectorSize, used)
	}
}

// TestAddFolderNoPath tests that an API call to add a storage folder fails if
// no path was provided.
func TestAddFolderNoPath(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Try adding a storage folder without setting "path" in the API call.
	addValues := url.Values{}
	addValues.Set("size", mediumSizeFolderString)
	err = st.stdPostAPI("/host/storage/folders/add", addValues)
	if err == nil {
		t.Fatal(err)
	}

	// Setting the path to an empty string should trigger the same error.
	addValues.Set("path", "")
	err = st.stdPostAPI("/host/storage/folders/add", addValues)
	if err == nil {
		t.Fatal(err)
	}
}

// TestAddFolderNoSize tests that an API call to add a storage folder fails if
// no path was provided.
func TestAddFolderNoSize(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Try adding a storage folder without setting "size" in the API call.
	addValues := url.Values{}
	addValues.Set("path", st.dir)
	err = st.stdPostAPI("/host/storage/folders/add", addValues)
	if err == nil || err.Error() != io.EOF.Error() {
		t.Fatalf("expected error to be %v, got %v", io.EOF, err)
	}
}

// TestAddSameFolderTwice tests that an API call that attempts to add a
// host storage folder that's already been added is handled gracefully.
func TestAddSameFolderTwice(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Make the call to add a storage folder twice.
	addValues := url.Values{}
	addValues.Set("path", st.dir)
	addValues.Set("size", mediumSizeFolderString)
	err = st.stdPostAPI("/host/storage/folders/add", addValues)
	if err != nil {
		t.Fatal(err)
	}
	err = st.stdPostAPI("/host/storage/folders/add", addValues)
	if err == nil || err.Error() != contractmanager.ErrRepeatFolder.Error() {
		t.Fatalf("expected err to be %v, got %v", err, contractmanager.ErrRepeatFolder)
	}
}

// TestResizeEmptyStorageFolder tests that invalid and valid calls to resize
// an empty storage folder are properly handled.
func TestResizeEmptyStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err := st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Find out how large the host's initial storage folder is.
	var sg StorageGET
	if err := st.getAPI("/host/storage", &sg); err != nil {
		t.Fatal(err)
	}
	defaultSize := sg.Folders[0].Capacity
	// Convert defaultSize (uint64) to a string for the API call.
	defaultSizeString := strconv.FormatUint(defaultSize, 10)

	resizeValues := url.Values{}
	resizeValues.Set("path", st.dir)
	resizeValues.Set("newsize", defaultSizeString)

	// Attempting to resize to the same size should return an error.
	err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
	if err == nil || err.Error() != contractmanager.ErrNoResize.Error() {
		t.Fatalf("expected error %v, got %v", contractmanager.ErrNoResize, err)
	}

	// Try resizing to a bunch of sizes (invalid ones first, valid ones second).
	// This ordering simplifies logic within the for loop.
	for i, test := range resizeTests {
		// Attempt to resize the host's storage folder.
		resizeValues.Set("newsize", test.sizeString)
		err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
		if (err == nil && test.err != nil) || (err != nil && err.Error() != test.err.Error()) {
			t.Fatalf("test %v: expected error to be %v, got %v", i, test.err, err)
		}

		// Find out if the resize call worked as expected.
		if err := st.getAPI("/host/storage", &sg); err != nil {
			t.Fatal(err)
		}
		// If the test size is valid, check that the folder has been resized
		// properly.
		if test.err == nil {
			// Check that the folder's total capacity has been updated.
			if got := sg.Folders[0].Capacity; got != test.size {
				t.Fatalf("test %v: expected folder to be resized to %v; got %v instead", i, test.size, got)
			}
			// Check that the folder's remaining capacity has been updated.
			if got := sg.Folders[0].CapacityRemaining; got != test.size {
				t.Fatalf("folder should be empty, but capacity remaining (%v) != total capacity (%v)", got, test.size)
			}
		} else {
			// If the test size is invalid, the folder should not have been
			// resized. The invalid test cases are all run before the valid ones,
			// so the folder size should still be defaultSize.
			if got := sg.Folders[0].Capacity; got != defaultSize {
				t.Fatalf("folder was resized to an invalid size (%v) in a test case that should have failed: %v", got, test)
			}
		}
	}
}

// TestResizeNonemptyStorageFolder tests that invalid and valid calls to resize
// a storage folder with one sector filled are properly handled.
// Ideally, we would also test a very full storage folder (including the case
// where the host tries to resize to a size smaller than the amount of data
// in the folder), but that would be a very expensive test.
func TestResizeNonemptyStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host and start accepting contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err := st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Block until the allowance has finished forming contracts.
	err = build.Retry(50, time.Millisecond*250, func() error {
		var rc RenterContracts
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 1 {
			return errors.New("no contracts")
		}
		return nil
	})
	if err != nil {
		t.Fatal("allowance setting failed")
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	fileBytes := 1024
	if err := createRandFile(path, fileBytes); err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	if err := st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Only one piece will be uploaded (10% at current redundancy)
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Error(rf.Files[0].UploadProgress)
		t.Fatal("uploading has failed")
	}

	// Find out how large the host's initial storage folder is.
	var sg StorageGET
	if err := st.getAPI("/host/storage", &sg); err != nil {
		t.Fatal(err)
	}
	defaultSize := sg.Folders[0].Capacity
	// Convert defaultSize (uint64) to a string for the API call.
	defaultSizeString := strconv.FormatUint(defaultSize, 10)

	resizeValues := url.Values{}
	resizeValues.Set("path", st.dir)
	resizeValues.Set("newsize", defaultSizeString)

	// Attempting to resize to the same size should return an error.
	err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
	if err == nil || err.Error() != contractmanager.ErrNoResize.Error() {
		t.Fatalf("expected error %v, got %v", contractmanager.ErrNoResize, err)
	}

	// Try resizing to a bunch of sizes (invalid ones first, valid ones second).
	// This ordering simplifies logic within the for loop.
	for _, test := range resizeTests {
		// Attempt to resize the host's storage folder.
		resizeValues.Set("newsize", test.sizeString)
		err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
		if (err == nil && test.err != nil) || (err != nil && test.err == nil) || (err != nil && err.Error() != test.err.Error()) {
			t.Fatalf("expected error to be %v, got %v", test.err, err)
		}

		// Find out if the resize call worked as expected.
		if err := st.getAPI("/host/storage", &sg); err != nil {
			t.Fatal(err)
		}
		// If the test size is valid, check that the folder has been resized
		// properly.
		if test.err == nil {
			// Check that the folder's total capacity has been updated.
			if sg.Folders[0].Capacity != test.size {
				t.Fatalf("expected folder to be resized to %v; got %v instead", test.size, sg.Folders[0].Capacity)
			}
			// Since one sector has been uploaded, the available capacity
			// should be one sector size smaller than the total capacity.
			if used := test.size - sg.Folders[0].CapacityRemaining; used != modules.SectorSize {
				t.Fatalf("used capacity (%v) != the size of 1 sector (%v)", used, modules.SectorSize)
			}
		} else {
			// If the test size is invalid, the folder should not have been
			// resized. The invalid test cases are all run before the valid
			// ones, so the folder size should still be defaultSize.
			if got := sg.Folders[0].Capacity; got != defaultSize {
				t.Fatalf("folder was resized to an invalid size (%v) in a test case that should have failed: %v", got, test)
			}
		}
	}
}

// TestResizeNonexistentFolder checks that an API call to resize a nonexistent
// folder triggers the appropriate error.
func TestResizeNonexistentFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// No folder has been created yet at st.dir, so using it as the path for
	// the resize call should trigger an error.
	resizeValues := url.Values{}
	resizeValues.Set("path", st.dir)
	resizeValues.Set("newsize", mediumSizeFolderString)
	err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
	if err == nil || err.Error() != errStorageFolderNotFound.Error() {
		t.Fatalf("expected error to be %v, got %v", errStorageFolderNotFound, err)
	}
}

// TestStorageFolderUnavailable simulates the situation where a storage folder
// is not available to the host when the host starts, verifying that it sets
// FailedWrites and FailedReads correctly and eventually finds the storage
// folder when it is made available to the host again.
func TestStorageFolderUnavailable(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// add a storage folder
	sfPath := build.TempDir(t.Name(), "storagefolder")
	err = os.MkdirAll(sfPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	sfValues := url.Values{}
	sfValues.Set("path", sfPath)
	sfValues.Set("size", "1048576")
	err = st.stdPostAPI("/host/storage/folders/add", sfValues)
	if err != nil {
		t.Fatal(err)
	}

	var sfs StorageGET
	err = st.getAPI("/host/storage", &sfs)
	if err != nil {
		t.Fatal(err)
	}

	if sfs.Folders[0].FailedReads != 0 || sfs.Folders[0].FailedWrites != 0 {
		t.Fatal("newly added folder has failed reads or writes")
	}

	// remove the folder on disk
	st.server.Close()
	sfPath2 := build.TempDir(t.Name(), "storagefolder-old")
	err = os.Rename(sfPath, sfPath2)
	if err != nil {
		t.Fatal(err)
	}

	// reload the host
	st, err = st.reloadedServerTester()
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	err = st.getAPI("/host/storage", &sfs)
	if err != nil {
		t.Fatal(err)
	}
	if sfs.Folders[0].FailedWrites < 999 {
		t.Fatal("storage folder should have lots of failed writes after being moved on disk")
	}
	if sfs.Folders[0].FailedReads < 999 {
		t.Fatal("storage folder should have lots of failed reads after being moved on disk")
	}

	// try some actions on the dead storage folder
	// resize
	sfValues.Set("size", "2097152")
	err = st.stdPostAPI("/host/storage/folders/resize", sfValues)
	if err == nil {
		t.Fatal("expected resize on unavailable storage folder to fail")
	}
	// remove
	err = st.stdPostAPI("/host/storage/folders/remove", sfValues)
	if err == nil {
		t.Fatal("expected remove on unavailable storage folder to fail")
	}

	// move the folder back
	err = os.Rename(sfPath2, sfPath)
	if err != nil {
		t.Fatal(err)
	}

	// wait for the contract manager to recheck the storage folder
	// NOTE: this is a hard-coded constant based on the contractmanager's maxFolderRecheckInterval constant.
	time.Sleep(time.Second * 10)

	// verify the storage folder is reset to normal
	err = st.getAPI("/host/storage", &sfs)
	if err != nil {
		t.Fatal(err)
	}
	if sfs.Folders[0].FailedWrites > 0 {
		t.Fatal("storage folder should have no failed writes after being moved back")
	}
	if sfs.Folders[0].FailedReads > 0 {
		t.Fatal("storage folder should have no failed reads after being moved back")
	}

	// reload the host and verify the storage folder is still good
	st.server.Close()
	st, err = st.reloadedServerTester()
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// storage folder should still be good
	err = st.getAPI("/host/storage", &sfs)
	if err != nil {
		t.Fatal(err)
	}
	if sfs.Folders[0].FailedWrites > 0 {
		t.Fatal("storage folder should have no failed writes after being moved back")
	}
	if sfs.Folders[0].FailedReads > 0 {
		t.Fatal("storage folder should have no failed reads after being moved back")
	}
}

// TestResizeFolderNoPath checks that an API call to resize a storage folder fails
// if no path was provided.
func TestResizeFolderNoPath(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// The call to resize should fail if no path has been provided.
	resizeValues := url.Values{}
	resizeValues.Set("newsize", mediumSizeFolderString)
	err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
	if err == nil || err.Error() != errNoPath.Error() {
		t.Fatalf("expected error to be %v; got %v", errNoPath, err)
	}
}

// TestRemoveEmptyStorageFolder checks that removing an empty storage folder
// succeeds -- even if the host is left with zero storage folders.
func TestRemoveEmptyStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Set up a storage folder for the host.
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Try to delete the host's empty storage folder.
	removeValues := url.Values{}
	removeValues.Set("path", st.dir)
	if err = st.stdPostAPI("/host/storage/folders/remove", removeValues); err != nil {
		t.Fatal(err)
	}
}

// TestRemoveStorageFolderError checks that invalid calls to
// /host/storage/folders/remove fail with the appropriate error.
func TestRemoveStorageFolderError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Set up a storage folder for the host.
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Try removing a nonexistent folder.
	removeValues := url.Values{}
	removeValues.Set("path", "/foo/bar")
	err = st.stdPostAPI("/host/storage/folders/remove", removeValues)
	if err == nil || err.Error() != errStorageFolderNotFound.Error() {
		t.Fatalf("expected error %v, got %v", errStorageFolderNotFound, err)
	}

	// The folder path can't be an empty string.
	removeValues.Set("path", "")
	err = st.stdPostAPI("/host/storage/folders/remove", removeValues)
	if err == nil || err.Error() != errNoPath.Error() {
		t.Fatalf("expected error to be %v; got %v", errNoPath, err)
	}
}

// TestRemoveStorageFolderForced checks that if a call to remove a storage
// folder will result in data loss, that call succeeds if and only if "force"
// has been set to "true".
func TestRemoveStorageFolderForced(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err := st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Block until the allowance has finished forming contracts.
	err = build.Retry(50, time.Millisecond*250, func() error {
		var rc RenterContracts
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 1 {
			return errors.New("no contracts")
		}
		return nil
	})
	if err != nil {
		t.Fatal("allowance setting failed")
	}

	// Create a file for upload.
	path := filepath.Join(st.dir, "test.dat")
	if err := createRandFile(path, 512); err != nil {
		t.Fatal(err)
	}
	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	if err := st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Only one piece will be uploaded (10%  at current redundancy)
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Error(rf.Files[0].UploadProgress)
		t.Fatal("uploading has failed")
	}

	// The host should not be able to remove its only folder without setting
	// "force" to "true", since this will result in data loss (there are no
	// other folders for the data to be redistributed to).
	removeValues := url.Values{}
	removeValues.Set("path", st.dir)
	err = st.stdPostAPI("/host/storage/folders/remove", removeValues)
	if err == nil || err.Error() != contractmanager.ErrPartialRelocation.Error() {
		t.Fatalf("expected err to be %v; got %v", contractmanager.ErrPartialRelocation, err)
	}
	// Forced removal of the folder should succeed, though.
	removeValues.Set("force", "true")
	err = st.stdPostAPI("/host/storage/folders/remove", removeValues)
	if err != nil {
		t.Fatal(err)
	}
}

// TestDeleteSector tests the call to delete a storage sector from the host.
func TestDeleteSector(t *testing.T) {
	t.Skip("broken because Merkle roots are no longer exposed")
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Set up the host for forming contracts.
	if err := st.announceHost(); err != nil {
		t.Fatal(err)
	}
	if err := st.acceptContracts(); err != nil {
		t.Fatal(err)
	}
	if err := st.setHostStorage(); err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing contracts to formed.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	// Block until the allowance has finished forming contracts.
	err = build.Retry(50, time.Millisecond*250, func() error {
		var rc RenterContracts
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 1 {
			return errors.New("no contracts")
		}
		return nil
	})
	if err != nil {
		t.Fatal("allowance setting failed")
	}

	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	if err := createRandFile(path, 1024); err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	if err = st.stdPostAPI("/renter/upload/test", uploadValues); err != nil {
		t.Fatal(err)
	}

	// Only one piece will be uploaded (10%  at current redundancy)
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Error(rf.Files[0].UploadProgress)
		t.Fatal("uploading has failed")
	}

	// Get the Merkle root of the piece that was uploaded.
	contracts := st.renter.Contracts()
	if len(contracts) != 1 {
		t.Fatalf("expected exactly 1 contract to have been formed; got %v instead", len(contracts))
	}
	// if len(contracts[0].MerkleRoots) < 1 {
	// 	t.Fatal("expected at least one merkle root")
	// }
	// sectorRoot := contracts[0].MerkleRoots[0].String()

	// if err = st.stdPostAPI("/host/storage/sectors/delete/"+sectorRoot, url.Values{}); err != nil {
	// 	t.Fatal(err)
	// }
}

// TestDeleteNonexistentSector checks that attempting to delete a storage
// sector that doesn't exist will fail with the appropriate error.
func TestDeleteNonexistentSector(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// These calls to delete imaginary sectors should fail for a few reasons:
	// - the given sector root strings are invalid
	// - the renter hasn't uploaded anything
	// - the host has no storage folders yet
	// Right now, the calls fail for the first reason. This test will report if that behavior changes.
	badHash := crypto.HashObject("fake object").String()
	err = st.stdPostAPI("/host/storage/sectors/delete/"+badHash, url.Values{})
	if err == nil || err.Error() != contractmanager.ErrSectorNotFound.Error() {
		t.Fatalf("expected error to be %v; got %v", contractmanager.ErrSectorNotFound, err)
	}
	wrongSize := "wrong size string"
	err = st.stdPostAPI("/host/storage/sectors/delete/"+wrongSize, url.Values{})
	if err == nil || err.Error() != crypto.ErrHashWrongLen.Error() {
		t.Fatalf("expected error to be %v; got %v", crypto.ErrHashWrongLen, err)
	}
}
