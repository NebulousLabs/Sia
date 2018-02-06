package renterhost

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/siatest"
)

// TestRenterHost executes a number of subtests using the same TestGroup to
// save time on initialization
func TestRenterHost(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a group for the subtests
	groupParams := siatest.GroupParams{
		Hosts:   5,
		Renters: 1,
		Miners:  1,
	}
	tg, err := siatest.NewGroupFromTemplate(groupParams)
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	// Specifiy subtests to run
	subTests := []struct {
		name string
		test func(*testing.T, *siatest.TestGroup)
	}{
		{"UploadDownload", testUploadDownload},
	}
	// Run subtests
	for _, subtest := range subTests {
		t.Run(subtest.name, func(t *testing.T) {
			subtest.test(t, tg)
		})
	}
}

// testUploadDownload is a subtest that uses an existing TestGroup to test if
// uploading and downloading a file works
func testUploadDownload(t *testing.T, tg *siatest.TestGroup) {
	// Grab the first of the group's renters
	renter := tg.Renters()[0]
	// Create file for upload
	file, err := siatest.NewFile(100)
	if err != nil {
		t.Fatal("Failed to create file for testing: ", err)
	}
	// Upload file, creating a parity piece for each host in the group
	err = renter.Upload(file, 1, uint64(len(tg.Hosts())))
	if err != nil {
		t.Fatal("Failed to start upload: ", err)
	}
	// Wait until upload reached 100% progress
	if err := renter.WaitForUploadProgress(file, 1); err != nil {
		t.Error(err)
	}
	// Wait until upload reaches len(tg.Hosts()) redundancy
	if err := renter.WaitForUploadRedundancy(file, float64(len(tg.Hosts()))); err != nil {
		t.Error(err)
	}
	// Download the file synchronously directly into memory and compare the
	// data to the original
	downloadedData, err := renter.Download(file)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(downloadedData, file.Bytes()) != 0 {
		t.Error("Downloaded data doesn't match original file's contents")
	}
	// Download the file synchronously to a file on disk and compare it to the
	// original
	downloadedFile, err := renter.DownloadToDisk(file, false)
	if err != nil {
		t.Fatal(err)
	}
	if file.Compare(downloadedFile) != 0 {
		t.Error("Downloaded file's contents do not equal the uploaded file's contents")
	}
	// Download the file  asynchronously, wait for the download to finish and
	// compare it to the original
	downloadedFile, err = renter.DownloadToDisk(file, true)
	if err != nil {
		t.Error(err)
	}
	if err := renter.WaitForDownload(file); err != nil {
		t.Error(err)
	}
	if file.Compare(downloadedFile) != 0 {
		t.Error("Downloaded file's contents do not equal the uploaded file's contents")
	}
}
