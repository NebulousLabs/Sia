package renter

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
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
		{"TestRenterStreamingCache", testRenterStreamingCache},
		{"TestUploadDownload", testUploadDownload},
		{"TestSingleFileGet", testSingleFileGet},
		{"TestDownloadMultipleLargeSectors", testDownloadMultipleLargeSectors},
		{"TestRenterDownloadAfterRenew", testRenterDownloadAfterRenew},
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

// testSingleFileGet is a subtest that uses an existing TestGroup to test if
// using the signle file API endpoint works
func testSingleFileGet(t *testing.T, tg *siatest.TestGroup) {
	// Grab the first of the group's renters
	renter := tg.Renters()[0]
	// Upload file, creating a piece for each host in the group
	dataPieces := uint64(1)
	parityPieces := uint64(len(tg.Hosts())) - dataPieces
	fileSize := 100 + siatest.Fuzz()
	_, _, err := renter.UploadNewFileBlocking(fileSize, dataPieces, parityPieces)
	if err != nil {
		t.Fatal("Failed to upload a file for testing: ", err)
	}

	files, err := renter.Files()
	if err != nil {
		t.Fatal("Failed to get renter files: ", err)
	}

	var file modules.FileInfo
	for _, f := range files {
		file, err = renter.File(f.SiaPath)
		if err != nil {
			t.Fatal("Failed to request single file", err)
		}
		if file != f {
			t.Fatal("Single file queries does not match file previously requested.")
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
	// redundancies later.
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

// testRenterStreamingCache checks if the chunk cache works correctly.
func testRenterStreamingCache(t *testing.T, tg *siatest.TestGroup) {
	// Grab the first of the group's renters
	r := tg.Renters()[0]

	// Set fileSize and redundancy for upload
	dataPieces := uint64(1)
	parityPieces := uint64(len(tg.Hosts())) - dataPieces

	// Set the bandwidth limit to 1 chunk per second.
	pieceSize := modules.SectorSize - crypto.TwofishOverhead
	chunkSize := int64(pieceSize * dataPieces)
	if err := r.RenterPostRateLimit(chunkSize, chunkSize); err != nil {
		t.Fatal(err)
	}

	rg, err := r.RenterGet()
	if err != nil {
		t.Fatal(err, "Could not request RenterGe()")
	}
	if rg.Settings.MaxDownloadSpeed != chunkSize {
		t.Fatal(errors.New("MaxDownloadSpeed doesn't match value set through RenterPostRateLimit"))
	}
	if rg.Settings.MaxUploadSpeed != chunkSize {
		t.Fatal(errors.New("MaxUploadSpeed doesn't match value set through RenterPostRateLimit"))
	}

	// Upload a file that is a single chunk big.
	_, remoteFile, err := r.UploadNewFileBlocking(int(chunkSize), dataPieces, parityPieces)
	if err != nil {
		t.Fatal(err)
	}

	// Download the same chunk 250 times. This should take at least 250 seconds
	// without caching but not more than 30 with caching.
	start := time.Now()
	for i := 0; i < 250; i++ {
		if _, err := r.Stream(remoteFile); err != nil {
			t.Fatal(err)
		}
		if time.Since(start) > time.Second*30 {
			t.Fatal("download took longer than 30 seconds")
		}
	}
}

// TestRenewFailing checks if a contract gets marked as !goodForRenew after
// failing multiple times in a row.
func TestRenewFailing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	renterDir, err := siatest.TestDir(filepath.Join(t.Name(), "renter"))
	if err != nil {
		t.Fatal(err)
	}

	// Create a group for the subtests
	groupParams := siatest.GroupParams{
		Hosts:  3,
		Miners: 1,
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

	// Add a renter with a custom allowance to give it plenty of time to renew
	// the contract later.
	renterParams := node.Renter(renterDir)
	renterParams.Allowance = siatest.DefaultAllowance
	renterParams.Allowance.Hosts = uint64(len(tg.Hosts()) - 1)
	renterParams.Allowance.Period = 100
	renterParams.Allowance.RenewWindow = 50
	if err = tg.AddNodes(renterParams); err != nil {
		t.Fatal(err)
	}
	renter := tg.Renters()[0]

	// Lock the wallet of one of the hosts to let the renew with that host
	// fail.
	if err := tg.Hosts()[0].WalletLockPost(); err != nil {
		t.Fatal(err)
	}

	// All the contracts of the renter should be goodForRenew.
	rcg, err := renter.RenterContractsGet()
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(rcg.Contracts)) != renterParams.Allowance.Hosts {
		t.Fatalf("renter had %v contracts but should have %v",
			len(rcg.Contracts), renterParams.Allowance.Hosts)
	}
	for _, c := range rcg.Contracts {
		if !c.GoodForRenew {
			t.Fatal("renter got a contract that is !goodForRenew")
		}
	}

	// Wait until the contract is supposed to be renewed.
	cg, err := renter.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	rg, err := renter.RenterGet()
	if err != nil {
		t.Fatal(err)
	}
	miner := tg.Miners()[0]
	blockHeight := cg.Height
	for blockHeight+rg.Settings.Allowance.RenewWindow < rcg.Contracts[0].EndHeight {
		if err := miner.MineBlock(); err != nil {
			t.Fatal(err)
		}
		blockHeight++
	}

	// contracts should still be goodForRenew.
	rcg, err = renter.RenterContractsGet()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rcg.Contracts {
		if !c.GoodForRenew {
			t.Fatal("renter got a contract that is !goodForRenew")
		}
	}

	// We should be within the renew window now. We keep mining blocks until
	// the host with the locked wallet has been replaced. This should happen
	// before we reach the endHeight of the contracts.
	replaced := false
	err = build.Retry(int(rcg.Contracts[0].EndHeight-blockHeight), 100*time.Millisecond, func() error {
		// contract should be !goodForRenew now.
		rcg, err = renter.RenterContractsGet()
		if err != nil {
			t.Fatal(err)
		}
		notGoodForRenew := 0
		goodForRenew := 0
		for _, c := range rcg.Contracts {
			if !c.GoodForRenew {
				notGoodForRenew++
			} else {
				goodForRenew++
			}
		}
		if !replaced && notGoodForRenew != 1 && goodForRenew != 1 {
			err := fmt.Errorf("there should be exactly 1 contract that is !goodForRenew but was %v",
				notGoodForRenew)
			return errors.Compose(miner.MineBlock(), err)
		}
		replaced = true
		if replaced && notGoodForRenew != 1 && goodForRenew != 2 {
			return fmt.Errorf("contract was set to !goodForRenew but hasn't been replaced yet")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// testRenterDownloadAfterRenew makes sure that we can still download a file
// after the contract period has ended.
func testRenterDownloadAfterRenew(t *testing.T, tg *siatest.TestGroup) {
	// Grab the first of the group's renters
	renter := tg.Renters()[0]
	// Upload file, creating a piece for each host in the group
	dataPieces := uint64(1)
	parityPieces := uint64(len(tg.Hosts())) - dataPieces
	fileSize := 100 + siatest.Fuzz()
	_, remoteFile, err := renter.UploadNewFileBlocking(fileSize, dataPieces, parityPieces)
	if err != nil {
		t.Fatal("Failed to upload a file for testing: ", err)
	}
	// Mine enough blocks for the next period to start. This means the
	// contracts should be renewed and the data should still be availeble for
	// download.
	miner := tg.Miners()[0]
	for i := types.BlockHeight(0); i < siatest.DefaultAllowance.Period; i++ {
		if err := miner.MineBlock(); err != nil {
			t.Fatal(err)
		}
	}
	// Download the file synchronously directly into memory.
	_, err = renter.DownloadByStream(remoteFile)
	if err != nil {
		t.Fatal(err)
	}
}
