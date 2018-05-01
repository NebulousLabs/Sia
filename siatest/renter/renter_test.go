package renter

import (
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
	"github.com/NebulousLabs/fastrand"
)

// TestRenter executes a number of subtests using the same TestGroup to
// save time on initialization
func TestRenter(t *testing.T) {
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

	// Specify subtests to run
	subTests := []struct {
		name string
		test func(*testing.T, *siatest.TestGroup)
	}{
		{"UploadDownload", testUploadDownload},
		{"DownloadMultipleLargeSectors", testDownloadMultipleLargeSectors},
		{"TestRenterLocalRepair", testRenterLocalRepair},
		{"TestRenterRemoteRepair", testRenterRemoteRepair},
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
	// Upload file, creating a piece for each host in the group
	dataPieces := uint64(1)
	parityPieces := uint64(len(tg.Hosts())) - dataPieces
	fileSize := 100 + siatest.Fuzz()
	localFile, remoteFile, err := renter.UploadNewFileBlocking(fileSize, dataPieces, parityPieces)
	if err != nil {
		t.Fatal("Failed to upload a file for testing: ", err)
	}
	// Download the file synchronously directly into memory
	_, err = renter.DownloadByStream(remoteFile)
	if err != nil {
		t.Fatal(err)
	}
	// Download the file synchronously to a file on disk
	_, err = renter.DownloadToDisk(remoteFile, false)
	if err != nil {
		t.Fatal(err)
	}
	// Download the file asynchronously and wait for the download to finish.
	localFile, err = renter.DownloadToDisk(remoteFile, true)
	if err != nil {
		t.Error(err)
	}
	if err := renter.WaitForDownload(localFile, remoteFile); err != nil {
		t.Error(err)
	}
	// Stream the file.
	_, err = renter.Stream(remoteFile)
	if err != nil {
		t.Fatal(err)
	}
	// Stream the file partially a few times. At least 1 byte is streamed.
	for i := 0; i < 5; i++ {
		from := fastrand.Intn(fileSize - 1)             // [0..fileSize-2]
		to := from + 1 + fastrand.Intn(fileSize-from-1) // [from+1..fileSize-1]
		_, err = renter.StreamPartial(remoteFile, localFile, uint64(from), uint64(to))
		if err != nil {
			t.Fatal(err)
		}
	}
}

// testDownloadMultipleLargeSectors downloads multiple large files (>5 Sectors)
// in parallel and makes sure that the downloads are blocking each other.
func testDownloadMultipleLargeSectors(t *testing.T, tg *siatest.TestGroup) {
	// parallelDownloads is the number of downloads that are run in parallel.
	parallelDownloads := 10
	// fileSize is the size of the downloaded file.
	fileSize := int(10*modules.SectorSize) + siatest.Fuzz()
	// set download limits and reset them after test.
	// uniqueRemoteFiles is the number of files that will be uploaded to the
	// network. Downloads will choose the remote file to download randomly.
	uniqueRemoteFiles := 5
	// Grab the first of the group's renters
	renter := tg.Renters()[0]
	// set download limits and reset them after test.
	if err := renter.RenterPostRateLimit(int64(fileSize)*2, 0); err != nil {
		t.Fatal("failed to set renter bandwidth limit", err)
	}
	defer func() {
		if err := renter.RenterPostRateLimit(0, 0); err != nil {
			t.Error("failed to reset renter bandwidth limit", err)
		}
	}()

	// Upload files
	dataPieces := uint64(len(tg.Hosts())) - 1
	parityPieces := uint64(1)
	remoteFiles := make([]*siatest.RemoteFile, 0, uniqueRemoteFiles)
	for i := 0; i < uniqueRemoteFiles; i++ {
		_, remoteFile, err := renter.UploadNewFileBlocking(fileSize, dataPieces, parityPieces)
		if err != nil {
			t.Fatal("Failed to upload a file for testing: ", err)
		}
		remoteFiles = append(remoteFiles, remoteFile)
	}

	// Randomly download using download to file and download to stream methods.
	wg := new(sync.WaitGroup)
	for i := 0; i < parallelDownloads; i++ {
		wg.Add(1)
		go func() {
			var err error
			var rf = remoteFiles[fastrand.Intn(len(remoteFiles))]
			if fastrand.Intn(2) == 0 {
				_, err = renter.DownloadByStream(rf)
			} else {
				_, err = renter.DownloadToDisk(rf, false)
			}
			if err != nil {
				t.Error("Download failed:", err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

// testRenterLocalRepair tests if a renter correctly repairs a file from disk
// after a host goes offline.
func testRenterLocalRepair(t *testing.T, tg *siatest.TestGroup) {
	// Grab the first of the group's renters
	renter := tg.Renters()[0]

	// Check that we have enough hosts for this test.
	if len(tg.Hosts()) < 2 {
		t.Fatal("This test requires at least 2 hosts")
	}

	// Set fileSize and redundancy for upload
	fileSize := int(modules.SectorSize)
	dataPieces := uint64(1)
	parityPieces := uint64(len(tg.Hosts())) - dataPieces

	// Upload file
	_, remoteFile, err := renter.UploadNewFileBlocking(fileSize, dataPieces, parityPieces)
	if err != nil {
		t.Fatal(err)
	}
	// Get the file info of the fully uploaded file. Tha way we can compare the
	// redundancieslater.
	fi, err := renter.FileInfo(remoteFile)
	if err != nil {
		t.Fatal("failed to get file info", err)
	}

	// Take down one of the hosts and check if redundancy decreases.
	if err := tg.RemoveNode(tg.Hosts()[0]); err != nil {
		t.Fatal("Failed to shutdown host", err)
	}
	expectedRedundancy := float64(dataPieces+parityPieces-1) / float64(dataPieces)
	if err := renter.WaitForDecreasingRedundancy(remoteFile, expectedRedundancy); err != nil {
		t.Fatal("Redundancy isn't decreasing", err)
	}
	// We should still be able to download
	if _, err := renter.DownloadByStream(remoteFile); err != nil {
		t.Fatal("Failed to download file", err)
	}
	// Bring up a new host and check if redundancy increments again.
	if err := tg.AddNodes(node.HostTemplate); err != nil {
		t.Fatal("Failed to create a new host", err)
	}
	if err := renter.WaitForUploadRedundancy(remoteFile, fi.Redundancy); err != nil {
		t.Fatal("File wasn't repaired", err)
	}
	// We should be able to download
	if _, err := renter.DownloadByStream(remoteFile); err != nil {
		t.Fatal("Failed to download file", err)
	}
}

// testRenterRemoteRepair tests if a renter correctly repairs a file by
// downloading it after a host goes offline.
func testRenterRemoteRepair(t *testing.T, tg *siatest.TestGroup) {
	// Grab the first of the group's renters
	r := tg.Renters()[0]

	// Check that we have enough hosts for this test.
	if len(tg.Hosts()) < 2 {
		t.Fatal("This test requires at least 2 hosts")
	}

	// Set fileSize and redundancy for upload
	fileSize := int(modules.SectorSize)
	dataPieces := uint64(1)
	parityPieces := uint64(len(tg.Hosts())) - dataPieces

	// Upload file
	localFile, remoteFile, err := r.UploadNewFileBlocking(fileSize, dataPieces, parityPieces)
	if err != nil {
		t.Fatal(err)
	}
	// Get the file info of the fully uploaded file. Tha way we can compare the
	// redundancieslater.
	fi, err := r.FileInfo(remoteFile)
	if err != nil {
		t.Fatal("failed to get file info", err)
	}

	// Delete the file locally.
	if err := localFile.Delete(); err != nil {
		t.Fatal("failed to delete local file", err)
	}

	// Take down all of the parity hosts and check if redundancy decreases.
	for i := uint64(0); i < parityPieces; i++ {
		if err := tg.RemoveNode(tg.Hosts()[0]); err != nil {
			t.Fatal("Failed to shutdown host", err)
		}
	}
	expectedRedundancy := float64(dataPieces+parityPieces-1) / float64(dataPieces)
	if err := r.WaitForDecreasingRedundancy(remoteFile, expectedRedundancy); err != nil {
		t.Fatal("Redundancy isn't decreasing", err)
	}
	// We should still be able to download
	if _, err := r.DownloadByStream(remoteFile); err != nil {
		t.Fatal("Failed to download file", err)
	}
	// Bring up new parity hosts and check if redundancy increments again.
	if err := tg.AddNodeN(node.HostTemplate, int(parityPieces)); err != nil {
		t.Fatal("Failed to create a new host", err)
	}
	// When doing remote repair the redundancy might not reach 100%.
	expectedRedundancy = (1.0 - renter.RemoteRepairDownloadThreshold) * fi.Redundancy
	if err := r.WaitForUploadRedundancy(remoteFile, expectedRedundancy); err != nil {
		t.Fatal("File wasn't repaired", err)
	}
	// We should be able to download
	if _, err := r.DownloadByStream(remoteFile); err != nil {
		t.Fatal("Failed to download file", err)
	}
}

// TestDownloadInterruptedBeforeSendingRevision runs testDownloadInterrupted
// with a dependency that interrupts the download before sending the signed
// revision to the host.
func TestDownloadInterruptedBeforeSendingRevision(t *testing.T) {
	testDownloadInterrupted(t, newDependencyInterruptDownloadBeforeSendingRevision())
}

// TestDownloadInterruptedAfterSendingRevision runs testDownloadInterrupted
// with a dependency that interrupts the download after sending the signed
// revision to the host.
func TestDownloadInterruptedAfterSendingRevision(t *testing.T) {
	testDownloadInterrupted(t, newDependencyInterruptDownloadAfterSendingRevision())
}

// TestUploadInterruptedBeforeSendingRevision runs testUploadInterrupted with a
// dependency that interrupts the upload before sending the signed revision to
// the host.
func TestUploadInterruptedBeforeSendingRevision(t *testing.T) {
	testUploadInterrupted(t, newDependencyInterruptUploadBeforeSendingRevision())
}

// TestUploadInterruptedAfterSendingRevision runs testUploadInterrupted with a
// dependency that interrupts the upload after sending the signed revision to
// the host.
func TestUploadInterruptedAfterSendingRevision(t *testing.T) {
	testUploadInterrupted(t, newDependencyInterruptUploadAfterSendingRevision())
}

// testDownloadInterrupted interrupts a download using the provided dependencies.
func testDownloadInterrupted(t *testing.T, deps *siatest.DependencyInterruptOnceOnKeyword) {
	if testing.Short() {
		t.SkipNow()
	}

	// Get a directory for testing.
	testDir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a group with a single renter and two hosts using the dependencies
	// for the renter.
	renterTemplate := node.Renter(testDir + "/renter")
	renterTemplate.ContractSetDeps = deps
	tg, err := siatest.NewGroup(renterTemplate, node.Host(testDir+"/host1"),
		node.Host(testDir+"/host2"), siatest.Miner(testDir+"/miner"))
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Upload a file that's 1 chunk large.
	renter := tg.Renters()[0]
	dataPieces := uint64(1)
	parityPieces := uint64(1)
	chunkSize := siatest.ChunkSize(uint64(dataPieces))
	_, remoteFile, err := renter.UploadNewFileBlocking(int(chunkSize), dataPieces, parityPieces)
	if err != nil {
		t.Fatal(err)
	}

	// Set the bandwidth limit to 1 chunk per second.
	if err := renter.RenterPostRateLimit(int64(chunkSize), int64(chunkSize)); err != nil {
		t.Fatal(err)
	}

	// Call fail on the dependency every 100 ms.
	cancel := make(chan struct{})
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		for {
			// Cause the next download to fail.
			deps.Fail()
			select {
			case <-cancel:
				wg.Done()
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	// Try downloading the file 5 times.
	for i := 0; i < 5; i++ {
		if _, err := renter.DownloadByStream(remoteFile); err == nil {
			t.Fatal("Download shouldn't suceed since it was interrupted")
		}
	}
	// Stop calling fail on the dependency.
	close(cancel)
	wg.Wait()
	deps.Disable()
	// Download the file once more successfully
	if _, err := renter.DownloadByStream(remoteFile); err != nil {
		t.Fatal("Failed to download the file", err)
	}
}

// testUploadInterrupted let's the upload fail using the provided dependencies
// and makes sure that this doesn't corrupt the contract.
func testUploadInterrupted(t *testing.T, deps *siatest.DependencyInterruptOnceOnKeyword) {
	if testing.Short() {
		t.SkipNow()
	}

	// Get a directory for testing.
	testDir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a group with a single renter and two hosts using the dependencies
	// for the renter.
	renterTemplate := node.Renter(testDir + "/renter")
	renterTemplate.ContractSetDeps = deps
	tg, err := siatest.NewGroup(renterTemplate, node.Host(testDir+"/host1"),
		node.Host(testDir+"/host2"), siatest.Miner(testDir+"/miner"))
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Set the bandwidth limit to 1 chunk per second.
	renter := tg.Renters()[0]
	dataPieces := uint64(1)
	parityPieces := uint64(1)
	chunkSize := siatest.ChunkSize(uint64(dataPieces))
	if err := renter.RenterPostRateLimit(int64(chunkSize), int64(chunkSize)); err != nil {
		t.Fatal(err)
	}

	// Call fail on the dependency every two seconds to allow some uploads to
	// finish.
	cancel := make(chan struct{})
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		// Loop until cancel was closed or we reach 5 iterations. Otherwise we
		// might end up blocking the upload for too long.
		for i := 0; i < 5; i++ {
			// Cause the next upload to fail.
			deps.Fail()
			select {
			case <-cancel:
				wg.Done()
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
		wg.Done()
	}()

	// Upload a file that's 1 chunk large.
	_, remoteFile, err := renter.UploadNewFileBlocking(int(chunkSize), dataPieces, parityPieces)
	if err != nil {
		t.Fatal(err)
	}
	// Stop calling fail on the dependency.
	close(cancel)
	wg.Wait()
	deps.Disable()
	// Download the file.
	if _, err := renter.DownloadByStream(remoteFile); err != nil {
		t.Fatal("Failed to download the file", err)
	}
}
