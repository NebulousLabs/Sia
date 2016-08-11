package api

import (
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/host/storagemanager"
)

var (
	// Various folder sizes for testing host storage folder resizing.
	// Must be provided as strings to the API call.
	minFolderSizeString    = strconv.FormatUint(storagemanager.MinimumStorageFolderSize(), 10)
	maxFolderSizeString    = strconv.FormatUint(storagemanager.MaximumStorageFolderSize(), 10)
	tooSmallFolderString   = strconv.FormatUint(storagemanager.MinimumStorageFolderSize()-1, 10)
	tooLargeFolderString   = strconv.FormatUint(storagemanager.MaximumStorageFolderSize()+1, 10)
	mediumSizeFolderString = strconv.FormatUint(3*storagemanager.MinimumStorageFolderSize(), 10)

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
		{"0", 0, storagemanager.ErrSmallStorageFolder},
		{tooSmallFolderString, storagemanager.MinimumStorageFolderSize() - 1, storagemanager.ErrSmallStorageFolder},
		{tooLargeFolderString, storagemanager.MaximumStorageFolderSize() + 1, storagemanager.ErrLargeStorageFolder},

		// valid sizes
		{minFolderSizeString, storagemanager.MinimumStorageFolderSize(), nil},
		{maxFolderSizeString, storagemanager.MaximumStorageFolderSize(), nil},
		{mediumSizeFolderString, 3 * storagemanager.MinimumStorageFolderSize(), nil},
	}
)

// TestResizeEmptyStorageFolder tests that invalid and valid calls to resize
// an empty storage folder are properly handled.
func TestResizeEmptyStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestResizeEmptyStorageFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
	if err == nil || err.Error() != storagemanager.ErrNoResize.Error() {
		t.Fatalf("expected error %v, got %v", storagemanager.ErrNoResize, err)
	}

	// Try resizing to a bunch of sizes (invalid ones first, valid ones second).
	// This ordering simplifies logic within the for loop.
	for _, test := range resizeTests {
		// Attempt to resize the host's storage folder.
		resizeValues.Set("newsize", test.sizeString)
		err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
		if (err == nil && test.err != nil) || (err != nil && err.Error() != test.err.Error()) {
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
			if got := sg.Folders[0].Capacity; got != test.size {
				t.Fatalf("expected folder to be resized to %v; got %v instead", test.size, got)
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
	st, err := createServerTester("TestResizeNonemptyStorageFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
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
	if err == nil || err.Error() != storagemanager.ErrNoResize.Error() {
		t.Fatalf("expected error %v, got %v", storagemanager.ErrNoResize, err)
	}

	// Try resizing to a bunch of sizes (invalid ones first, valid ones second).
	// This ordering simplifies logic within the for loop.
	for _, test := range resizeTests {
		// Attempt to resize the host's storage folder.
		resizeValues.Set("newsize", test.sizeString)
		err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
		if (err == nil && test.err != nil) || (err != nil && err.Error() != test.err.Error()) {
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
	st, err := createServerTester("TestResizeNonexistentFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

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

// TestResizeFolderNoPath checks that an API call to resize a storage folder fails
// if no path was provided.
func TestResizeFolderNoPath(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestResizeFolderNoPath")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// The call to resize should fail if no path has been provided.
	resizeValues := url.Values{}
	resizeValues.Set("newsize", mediumSizeFolderString)
	err = st.stdPostAPI("/host/storage/folders/resize", resizeValues)
	if err == nil || err.Error() != "path parameter is required" {
		t.Fatalf("expected error to be path parameter is required; got %v", err)
	}
}

/*
// TestIntegrationRenewing tests that the renter and host manage contract
// renewals properly.
func TestIntegrationRenewing(t *testing.T) {
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
*/
