package api

// renterhost_test.go sets up larger integration tests between renters and
// hosts, checking that the whole storage ecosystem is functioning cohesively.

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestRenterLocalRepair verifies that the renter will use the local file to
// repair if the file exists locally
func TestRenterLocalRepair(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	stH1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	defer stH1.server.Close()
	testGroup := []*serverTester{st, stH1}

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

	// Add storage to every host.
	err = addStorageToAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance with two hosts.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "50000000000000000000000000000") // 50k SC
	allowanceValues.Set("hosts", "2")
	allowanceValues.Set("period", "10")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file to upload.
	filesize := int(1024)
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, filesize)
	if err != nil {
		t.Fatal(err)
	}

	// upload the file
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// redundancy should reach 2
	var rf RenterFiles
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 2 {
			return nil
		}
		return errors.New("file not uploaded")
	})
	if err != nil {
		t.Fatal(err)
	}

	// download spending should not have increased
	var rg RenterGET
	err = st.getAPI("/renter", &rg)
	if err != nil {
		t.Fatal(err)
	}
	if rg.FinancialMetrics.DownloadSpending.Cmp(types.NewCurrency64(0)) > 0 {
		t.Fatalf("expected no download spending, got %v instead\n", rg.FinancialMetrics.DownloadSpending)
	}

	// take down one of the hosts
	err = stH1.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	// wait for the redundancy to decrement
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 1 {
			return nil
		}
		return errors.New("file redundancy not decremented")
	})
	if err != nil {
		t.Fatal(err)
	}

	// bring up a new host
	stNewHost, err := blankServerTester(t.Name() + "-newhost")
	if err != nil {
		t.Fatal(err)
	}
	defer stNewHost.server.Close()

	testGroup = []*serverTester{st, stNewHost}

	// Connect the testers to eachother so that they are all on the same
	// blockchain.
	err = fullyConnectNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	_, err = synchronizationCheck(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure that every wallet has money in it.
	err = fundAllNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	err = stNewHost.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = stNewHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	err = waitForBlock(stNewHost.cs.CurrentBlock().ID(), st)
	if err != nil {
		t.Fatal(err)
	}

	// add a few new blocks in order to cause the renter to form contracts with the new host
	for i := 0; i < 10; i++ {
		b, err := stNewHost.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		for _, tester := range testGroup {
			err = waitForBlock(b.ID(), tester)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// redundancy should increment back to 2 as the renter uploads to the new
	// host using the download-to-upload strategy
	err = retry(240, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 2 && rf.Files[0].Available {
			return nil
		}
		return errors.New("file redundancy not incremented")
	})
	if err != nil {
		t.Fatal(err)
	}
	if rg.FinancialMetrics.DownloadSpending.Cmp(types.NewCurrency64(0)) > 0 {
		t.Fatalf("expected no download spending, got %v instead\n", rg.FinancialMetrics.DownloadSpending)
	}
}

// TestRemoteFileRepair verifies that if a trackedFile is made unavailable
// locally by being deleted, the repair loop will download the necessary chunks
// from the living hosts and upload them to new hosts.
func TestRemoteFileRepair(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	stH1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	defer stH1.server.Close()
	testGroup := []*serverTester{st, stH1}

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

	// Add storage to every host.
	err = addStorageToAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance with two hosts.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "50000000000000000000000000000") // 50k SC
	allowanceValues.Set("hosts", "2")
	allowanceValues.Set("period", "10")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file to upload.
	filesize := int(45678)
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, filesize)
	if err != nil {
		t.Fatal(err)
	}

	// upload the file
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// redundancy should reach 2
	var rf RenterFiles
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 2 {
			return nil
		}
		return errors.New("file not uploaded")
	})
	if err != nil {
		t.Fatal(err)
	}

	// verify we can download
	downloadPath := filepath.Join(st.dir, "test-downloaded-verify.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err != nil {
		t.Fatal(err)
	}

	// save a copy of the file contents in memory for verification later
	orig, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// remove the local copy of the file
	err = os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}

	// take down one of the hosts
	err = stH1.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	// wait for the redundancy to decrement
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 1 {
			return nil
		}
		return errors.New("file redundancy not decremented")
	})
	if err != nil {
		t.Fatal(err)
	}

	// verify we still can download
	downloadPath = filepath.Join(st.dir, "test-downloaded-verify2.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err != nil {
		t.Fatal(err)
	}

	// bring up a new host
	stNewHost, err := blankServerTester(t.Name() + "-newhost")
	if err != nil {
		t.Fatal(err)
	}
	defer stNewHost.server.Close()

	testGroup = []*serverTester{st, stNewHost}

	// Connect the testers to eachother so that they are all on the same
	// blockchain.
	err = fullyConnectNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	_, err = synchronizationCheck(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure that every wallet has money in it.
	err = fundAllNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	err = stNewHost.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = stNewHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	err = waitForBlock(stNewHost.cs.CurrentBlock().ID(), st)
	if err != nil {
		t.Fatal(err)
	}

	// add a few new blocks in order to cause the renter to form contracts with the new host
	for i := 0; i < 10; i++ {
		b, err := stNewHost.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		for _, tester := range testGroup {
			err = waitForBlock(b.ID(), tester)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// redundancy should increment back to 2 as the renter uploads to the new
	// host using the download-to-upload strategy
	err = retry(240, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 2 && rf.Files[0].Available {
			return nil
		}
		return errors.New("file redundancy not incremented")
	})
	if err != nil {
		t.Fatal(err)
	}

	// we have to wait a bit for the download loop to update with the new
	// contracts. retry the download for up to 90 seconds.
	downloadPath = filepath.Join(st.dir, "test-downloaded.dat")
	err = retry(90, time.Second, func() error {
		return st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check that the download has the right contents.
	downloaded, err := ioutil.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orig, downloaded) {
		t.Fatal("data mismatch when downloading a file")
	}
}

// TestHostAndRentVanilla sets up an integration test where a host and renter
// do basic uploads and downloads.
func TestHostAndRentVanilla(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// Announce the host and start accepting contracts.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}
	err = st.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = st.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "20"
	testPeriodInt := 20
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
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

	// Check the host, who should now be reporting file contracts.
	//
	// TODO: Switch to using an API call.
	obligations := st.host.StorageObligations()
	if len(obligations) != 1 {
		t.Error("Host has wrong number of obligations:", len(obligations))
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

	// Mine two blocks, which should cause the host to submit the storage
	// obligation to the blockchain.
	for i := 0; i < 2; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond * 200)
	}

	// Check that the host was able to get the file contract confirmed on the
	// blockchain.
	obligations = st.host.StorageObligations()
	if len(obligations) != 1 {
		t.Error("Host has wrong number of obligations:", len(obligations))
	}
	if !obligations[0].OriginConfirmed {
		t.Error("host has not seen the file contract on the blockchain")
	}

	// Mine blocks until the host should have submitted a storage proof.
	for i := 0; i <= testPeriodInt+5; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond * 200)
	}

	success := false
	obligations = st.host.StorageObligations()
	for _, obligation := range obligations {
		if obligation.ProofConfirmed {
			success = true
			break
		}
	}
	if !success {
		t.Error("does not seem like the host has submitted a storage proof successfully to the network")
	}
}

// TestHostAndRentMultiHost sets up an integration test where three hosts and a
// renter do basic (parallel) uploads and downloads.
func TestHostAndRentMultiHost(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()
	stH1, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stH1.server.panicClose()
	stH2, err := blankServerTester(t.Name() + " - Host 3")
	if err != nil {
		t.Fatal(err)
	}
	defer stH2.server.panicClose()
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

	// Add storage to every host.
	err = addStorageToAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Announce every host.
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance with three hosts.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "50000000000000000000000000000") // 50k SC
	allowanceValues.Set("hosts", "3")
	allowanceValues.Set("period", "10")
	allowanceValues.Set("renewwindow", "2")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file to upload.
	filesize := int(45678)
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, filesize)
	if err != nil {
		t.Fatal(err)
	}

	// Upload a file with 2-of-6 redundancy.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("datapieces", "2")
	uploadValues.Set("paritypieces", "4")
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Three pieces should get uploaded.
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 50); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 50 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// Try downloading the file.
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
}

// TestHostAndRentManyFiles sets up an integration test where a single renter
// is uploading many files to the network.
func TestHostAndRentManyFiles(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()
	stH1, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stH1.server.panicClose()
	stH2, err := blankServerTester(t.Name() + " - Host 3")
	if err != nil {
		t.Fatal(err)
	}
	defer stH2.server.panicClose()
	stH3, err := blankServerTester(t.Name() + " - Host 4")
	if err != nil {
		t.Fatal(err)
	}
	defer stH3.server.panicClose()
	testGroup := []*serverTester{st, stH1, stH2, stH3}

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

	// Add storage to every host.
	err = addStorageToAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Announce every host.
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance with four hosts.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "50000000000000000000000000000") // 50k SC
	allowanceValues.Set("hosts", "4")
	allowanceValues.Set("period", "5")
	allowanceValues.Set("renewwindow", "2")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create 3 files to upload at the same time.
	filesize1 := int(12347)
	filesize2 := int(22343)
	filesize3 := int(32349)
	path1 := filepath.Join(st.dir, "test1.dat")
	path2 := filepath.Join(st.dir, "test2.dat")
	path3 := filepath.Join(st.dir, "test3.dat")
	err = createRandFile(path1, filesize1)
	if err != nil {
		t.Fatal(err)
	}
	err = createRandFile(path2, filesize2)
	if err != nil {
		t.Fatal(err)
	}
	err = createRandFile(path3, filesize3)
	if err != nil {
		t.Fatal(err)
	}

	// Concurrently upload a file with 1-of-4 redundancy, 2-of-4 redundancy,
	// and 3-of-4 redundancy.
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		uploadValues := url.Values{}
		uploadValues.Set("source", path1)
		uploadValues.Set("datapieces", "1")
		uploadValues.Set("paritypieces", "3")
		err := st.stdPostAPI("/renter/upload/test1", uploadValues)
		if err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		uploadValues := url.Values{}
		uploadValues.Set("source", path2)
		uploadValues.Set("datapieces", "2")
		uploadValues.Set("paritypieces", "2")
		err := st.stdPostAPI("/renter/upload/test2", uploadValues)
		if err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		uploadValues := url.Values{}
		uploadValues.Set("source", path3)
		uploadValues.Set("datapieces", "3")
		uploadValues.Set("paritypieces", "1")
		err := st.stdPostAPI("/renter/upload/test3", uploadValues)
		if err != nil {
			t.Error(err)
		}
	}()

	// Block until the upload call is complete for all three files.
	wg.Wait()

	// Block until all files hit 100% uploaded.
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 3 || rf.Files[0].UploadProgress < 100 || rf.Files[1].UploadProgress < 100 || rf.Files[2].UploadProgress < 100); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(500 * time.Millisecond)
	}
	if len(rf.Files) != 3 || rf.Files[0].UploadProgress < 100 || rf.Files[1].UploadProgress < 100 || rf.Files[2].UploadProgress < 100 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0], rf.Files[1], rf.Files[2])
	}

	// Download all three files in parallel.
	wg.Add(3)
	go func() {
		defer wg.Done()
		downpath := filepath.Join(st.dir, "testdown1.dat")
		err := st.stdGetAPI("/renter/download/test1?destination=" + downpath)
		if err != nil {
			t.Error(err)
		}
		// Check that the download has the right contents.
		orig, err := ioutil.ReadFile(path1)
		if err != nil {
			t.Error(err)
		}
		download, err := ioutil.ReadFile(downpath)
		if err != nil {
			t.Error(err)
		}
		if bytes.Compare(orig, download) != 0 {
			t.Error("data mismatch when downloading a file")
		}
	}()
	go func() {
		defer wg.Done()
		downpath := filepath.Join(st.dir, "testdown2.dat")
		err := st.stdGetAPI("/renter/download/test2?destination=" + downpath)
		if err != nil {
			t.Error(err)
		}
		// Check that the download has the right contents.
		orig, err := ioutil.ReadFile(path2)
		if err != nil {
			t.Error(err)
		}
		download, err := ioutil.ReadFile(downpath)
		if err != nil {
			t.Error(err)
		}
		if bytes.Compare(orig, download) != 0 {
			t.Error("data mismatch when downloading a file")
		}
	}()
	go func() {
		defer wg.Done()
		downpath := filepath.Join(st.dir, "testdown3.dat")
		err := st.stdGetAPI("/renter/download/test3?destination=" + downpath)
		if err != nil {
			t.Error(err)
		}
		// Check that the download has the right contents.
		orig, err := ioutil.ReadFile(path3)
		if err != nil {
			t.Error(err)
		}
		download, err := ioutil.ReadFile(downpath)
		if err != nil {
			t.Error(err)
		}
		if bytes.Compare(orig, download) != 0 {
			t.Error("data mismatch when downloading a file")
		}
	}()
	wg.Wait()

	// The renter's downloads queue should have 3 entries now.
	var queue RenterDownloadQueue
	if err = st.getAPI("/renter/downloads", &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Downloads) != 3 {
		t.Fatalf("expected renter to have 1 download in the queue; got %v", len(queue.Downloads))
	}
}

// TestRenterUploadDownload tests that downloading and uploading in parallel
// does not result in failures or stalling.
func TestRenterUploadDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

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

	// Check financial metrics; coins should have been spent on contracts
	var rg RenterGET
	err = st.getAPI("/renter", &rg)
	if err != nil {
		t.Fatal(err)
	}
	spent := rg.Settings.Allowance.Funds.Sub(rg.FinancialMetrics.Unspent)
	if spent.IsZero() {
		t.Fatal("financial metrics do not reflect contract spending")
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

	// Check financial metrics; funds should have been spent on uploads/downloads
	err = st.getAPI("/renter", &rg)
	if err != nil {
		t.Fatal(err)
	}
	fm := rg.FinancialMetrics
	newSpent := rg.Settings.Allowance.Funds.Sub(fm.Unspent)
	// all new spending should be reflected in upload/download/storage spending
	diff := fm.UploadSpending.Add(fm.DownloadSpending).Add(fm.StorageSpending)
	if !diff.Equals(newSpent.Sub(spent)) {
		t.Fatal("all new spending should be reflected in metrics:", diff, newSpent.Sub(spent))
	}
}

// TestRenterCancelAllowance tests that setting an empty allowance causes
// uploads, downloads, and renewals to cease.
func TestRenterCancelAllowance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

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

	// Cancel the allowance
	allowanceValues = url.Values{}
	allowanceValues.Set("funds", "0")
	allowanceValues.Set("hosts", "0")
	allowanceValues.Set("period", "0")
	allowanceValues.Set("renewwindow", "0")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Try downloading the file; should fail
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err == nil || !strings.Contains(err.Error(), "insufficient hosts") {
		t.Fatal("expected insufficient hosts error, got", err)
	}
}

// TestRenterParallelDelete tests that uploading and deleting parallel does not
// result in failures or stalling.
func TestRenterParallelDelete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

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

	// Create two files.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	path2 := filepath.Join(st.dir, "test2.dat")
	err = createRandFile(path2, 1024)
	if err != nil {
		t.Fatal(err)
	}

	// Upload the first file to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// Wait for the first file to be registered in the renter.
	var rf RenterFiles
	for i := 0; i < 200 && len(rf.Files) != 1; i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 {
		t.Fatal("file is not being registered:", rf.Files)
	}

	// In parallel, start uploading the other file, and delete the first file.
	uploadValues = url.Values{}
	uploadValues.Set("source", path2)
	err = st.stdPostAPI("/renter/upload/test2", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	err = st.stdPostAPI("/renter/delete/test", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	// Only the second file should be present
	st.getAPI("/renter/files", &rf)
	if len(rf.Files) != 1 || rf.Files[0].SiaPath != "test2" {
		t.Fatal("file was not deleted properly:", rf.Files)
	}

	// Wait for the second upload to complete.
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(100 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files)
	}

	// In parallel, download and delete the second file.
	go st.stdPostAPI("/renter/delete/test2", url.Values{})
	time.Sleep(100 * time.Millisecond)
	downpath := filepath.Join(st.dir, "testdown.dat")
	err = st.stdGetAPI("/renter/download/test2?destination=" + downpath)
	if err == nil {
		t.Fatal("download should fail after delete")
	}

	// No files should be present
	st.getAPI("/renter/files", &rf)
	if len(rf.Files) != 0 {
		t.Fatal("file was not deleted properly:", rf.Files)
	}
}

// TestRenterRenew sets up an integration test where a renter renews a
// contract with a host.
func TestRenterRenew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

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
	var ah HostdbActiveGET
	for i := 0; i < 50; i++ {
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			t.Fatal(err)
		}
		if len(ah.Hosts) == 1 {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := 10
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", strconv.Itoa(testPeriod))
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

	// Get current contract ID.
	var rc RenterContracts
	err = st.getAPI("/renter/contracts", &rc)
	if err != nil {
		t.Fatal(err)
	}
	contractID := rc.Contracts[0].ID

	// Mine enough blocks to enter the renewal window.
	testWindow := testPeriod / 2
	for i := 0; i < testWindow+1; i++ {
		st.miner.AddBlock()
	}
	// Wait for the contract to be renewed.
	for i := 0; i < 200 && (len(rc.Contracts) != 1 || rc.Contracts[0].ID == contractID); i++ {
		st.getAPI("/renter/contracts", &rc)
		time.Sleep(100 * time.Millisecond)
	}
	if rc.Contracts[0].ID == contractID {
		t.Fatal("contract was not renewed:", rc.Contracts[0])
	}

	// Try downloading the file.
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
}

// TestRenterAllowance sets up an integration test where a renter attempts to
// download a file after changing the allowance.
func TestRenterAllowance(t *testing.T) {
	t.Skip("bypassing NDF")
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
	testFunds := types.SiacoinPrecision.Mul64(10000) // 10k SC
	testPeriod := 20
	allowanceValues.Set("funds", testFunds.String())
	allowanceValues.Set("period", strconv.Itoa(testPeriod))
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

	t.Skip("ndf - re-enable after contractor overhaul")

	// Try downloading the file after modifying the allowance in various ways.
	allowances := []struct {
		funds  types.Currency
		period int
	}{
		{testFunds.Mul64(10), testPeriod / 2},
		{testFunds, testPeriod / 2},
		{testFunds.Div64(10), testPeriod / 2},
		{testFunds.Mul64(10), testPeriod},
		{testFunds, testPeriod},
		{testFunds.Div64(10), testPeriod},
		{testFunds.Mul64(10), testPeriod * 2},
		{testFunds, testPeriod * 2},
		{testFunds.Div64(10), testPeriod * 2},
	}

	for _, a := range allowances {
		allowanceValues.Set("funds", a.funds.String())
		allowanceValues.Set("period", strconv.Itoa(a.period))
		err = st.stdPostAPI("/renter", allowanceValues)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)

		// Try downloading the file.
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
	}
}

// TestHostAndRentReload sets up an integration test where a host and renter
// do basic uploads and downloads, with an intervening shutdown+startup.
func TestHostAndRentReload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

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
	// Mine a block so that the wallet reclaims refund outputs
	_, err = st.miner.AddBlock()
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

	// Try downloading the file.
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

	// close and reopen the server
	err = st.server.Close()
	if err != nil {
		t.Fatal(err)
	}
	st, err = assembleServerTester(st.walletKey, st.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Try downloading the file.
	err = st.stdGetAPI("/renter/download/test?destination=" + downpath)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the download has the right contents.
	orig, err = ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	download, err = ioutil.ReadFile(downpath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(orig, download) != 0 {
		t.Fatal("data mismatch when downloading a file")
	}
}

// TestHostAndRenterRenewInterrupt
func TestHostAndRenterRenewInterrupt(t *testing.T) {
	t.Skip("active test following contractor overhaul")
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	stHost, err := blankServerTester(t.Name() + "-Host")
	if err != nil {
		t.Fatal(err)
	}
	sts := []*serverTester{st, stHost}
	err = fullyConnectNodes(sts)
	if err != nil {
		t.Fatal(err)
	}
	err = fundAllNodes(sts)
	if err != nil {
		t.Fatal(err)
	}

	// Announce the host.
	err = stHost.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = stHost.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Wait for host to be seen in renter's hostdb
	var ah HostdbActiveGET
	for i := 0; i < 50; i++ {
		if err = st.getAPI("/hostdb/active", &ah); err != nil {
			t.Fatal(err)
		}
		if len(ah.Hosts) == 1 {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	if len(ah.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %v", len(ah.Hosts))
	}

	// Upload a file to the host.
	allowanceValues := url.Values{}
	testFunds := "10000000000000000000000000000" // 10k SC
	testPeriod := "10"
	testPeriodInt := 10
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}
	// Create a file.
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, 10e3)
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

	// Get current contract ID.
	var rc RenterContracts
	err = st.getAPI("/renter/contracts", &rc)
	if err != nil {
		t.Fatal(err)
	}
	contractID := rc.Contracts[0].ID

	// Mine enough blocks to enter the renewal window.
	testWindow := testPeriodInt / 2
	for i := 0; i < testWindow+1; i++ {
		st.miner.AddBlock()
	}
	// Wait for the contract to be renewed.
	for i := 0; i < 200 && (len(rc.Contracts) != 1 || rc.Contracts[0].ID == contractID); i++ {
		st.getAPI("/renter/contracts", &rc)
		time.Sleep(100 * time.Millisecond)
	}
	if rc.Contracts[0].ID == contractID {
		t.Fatal("contract was not renewed:", rc.Contracts[0])
	}

	// Only one piece will be uploaded (10% at current redundancy).
	var rf RenterFiles
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(1000 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
	}

	// Try downloading the file.
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
}

// TestRedundancyReporting verifies that redundancy reporting is accurate if
// contracts become offline.
func TestRedundancyReporting(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	stH1, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	testGroup := []*serverTester{st, stH1}

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
	// Add storage to every host.
	err = addStorageToAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	// Announce every host.
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance with two hosts.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "50000000000000000000000000000") // 50k SC
	allowanceValues.Set("hosts", "2")
	allowanceValues.Set("period", "10")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file to upload.
	filesize := int(1024)
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, filesize)
	if err != nil {
		t.Fatal(err)
	}

	// upload the file
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// redundancy should reach 2
	var rf RenterFiles
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 2 {
			return nil
		}
		return errors.New("file not uploaded")
	})
	if err != nil {
		t.Fatal(err)
	}

	// take down one of the hosts
	stH1.server.Close()

	// wait for the redundancy to decrement
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 1 {
			return nil
		}
		return errors.New("file redundancy not decremented")
	})
	if err != nil {
		t.Fatal(err)
	}

	// bring back the host
	stH1, err = assembleServerTester(stH1.walletKey, stH1.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stH1.server.Close()
	testGroup = []*serverTester{st, stH1}
	err = fullyConnectNodes(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Add a block to clear the transaction pool and give the host an output to
	// make an announcement, and then make the announcement.
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = synchronizationCheck(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Wait until the host shows back up in the hostdb.
	var ah HostdbActiveGET
	err = retry(200, 100*time.Millisecond, func() error {
		err := st.getAPI("/hostdb/active", &ah)
		if err != nil {
			return err
		}
		if len(ah.Hosts) != 2 {
			return errors.New("not enough hosts")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine another block so that the contract checker updates the IsGood status
	// of the contracts.
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = synchronizationCheck(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Redundancy should re-report at 2.
	err = retry(250, 100*time.Millisecond, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 2 {
			return nil
		}
		return errors.New("file redundancy not incremented")
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRenterMissingHosts verifies that if hosts are taken offline, downloads
// fail.
func TestRenterMissingHosts(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()
	stH1, err := blankServerTester(t.Name() + " - Host 1")
	if err != nil {
		t.Fatal(err)
	}
	defer stH1.server.Close()
	stH2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stH2.server.Close()
	stH3, err := blankServerTester(t.Name() + " - Host 3")
	if err != nil {
		t.Fatal(err)
	}
	defer stH3.server.Close()
	testGroup := []*serverTester{st, stH1, stH2, stH3}

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

	// Add storage to every host.
	err = addStorageToAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance with two hosts.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "50000000000000000000000000000") // 50k SC
	allowanceValues.Set("hosts", "3")
	allowanceValues.Set("period", "10")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file to upload.
	filesize := int(100)
	path := filepath.Join(st.dir, "test.dat")
	err = createRandFile(path, filesize)
	if err != nil {
		t.Fatal(err)
	}

	// upload the file
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("datapieces", "2")
	uploadValues.Set("paritypieces", "1")
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// redundancy should reach 1.5
	var rf RenterFiles
	err = retry(20, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 1.5 {
			return nil
		}
		return errors.New("file not uploaded")
	})
	if err != nil {
		t.Fatal(err)
	}

	// verify we can download
	downloadPath := filepath.Join(st.dir, "test-downloaded-verify.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err != nil {
		t.Fatal(err)
	}

	// take down one of the hosts
	err = stH1.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	// wait for the redundancy to decrement
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 1 {
			return nil
		}
		return errors.New("file redundancy not decremented: " + fmt.Sprint(rf.Files[0].Redundancy))
	})
	if err != nil {
		t.Log(err)
	}

	// verify we still can download
	downloadPath = filepath.Join(st.dir, "test-downloaded-verify2.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err != nil {
		t.Fatal(err)
	}

	// take down another host
	err = stH2.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	// wait for the redundancy to decrement
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 0.5 {
			return nil
		}
		return errors.New("file redundancy not decremented: " + fmt.Sprint(rf.Files[0].Redundancy))
	})
	if err != nil {
		t.Log(err)
	}

	// verify that the download fails
	downloadPath = filepath.Join(st.dir, "test-downloaded-verify3.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err == nil {
		t.Fatal("expected download to fail with redundancy <1")
	}

	// take down another host
	err = stH3.server.Close()
	if err != nil {
		t.Fatal(err)
	}

	// wait for the redundancy to decrement
	err = retry(60, time.Second, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 0 {
			return nil
		}
		return errors.New("file redundancy not decremented: " + fmt.Sprint(rf.Files[0].Redundancy))
	})
	if err != nil {
		t.Log(err)
	}

	// verify that the download fails
	downloadPath = filepath.Join(st.dir, "test-downloaded-verify4.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath)
	if err == nil {
		t.Fatal("expected download to fail with redundancy <1")
	}
}
