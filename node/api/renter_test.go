package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
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
	testFunds       = "10000000000000000000000000000" // 10k SC
	testPeriod      = "5"
	testRenewWindow = "2"
)

// createRandFile creates a file on disk and fills it with random bytes.
func createRandFile(path string, size int) error {
	return ioutil.WriteFile(path, fastrand.Bytes(size), 0600)
}

// setupTestDownload creates a server tester with an uploaded file of size
// `size` and name `name`.
func setupTestDownload(t *testing.T, size int, name string, waitOnAvailability bool) (*serverTester, string) {
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

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testFunds := testFunds
	testPeriod := "10"
	renewWindow := "5"
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
	allowanceValues.Set("renewwindow", renewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file.
	path := filepath.Join(build.SiaTestingDir, "api", t.Name(), name)
	err = createRandFile(path, size)
	if err != nil {
		t.Fatal(err)
	}

	// Upload to host.
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	uploadValues.Set("datapieces", "1")
	uploadValues.Set("paritypieces", "1")
	err = st.stdPostAPI("/renter/upload/"+name, uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	if waitOnAvailability {
		// wait for the file to become available
		err = build.Retry(200, time.Second, func() error {
			var rf RenterFiles
			st.getAPI("/renter/files", &rf)
			if len(rf.Files) != 1 || !rf.Files[0].Available {
				return fmt.Errorf("the uploading is not succeeding for some reason: %v\n", rf.Files[0])
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	return st, path
}

// runDownloadTest uploads a file and downloads it using the specified
// parameters, verifying that the parameters are applied correctly and the file
// is downloaded successfully.
func runDownloadTest(t *testing.T, filesize, offset, length int64, useHttpResp bool, testName string) error {
	ulSiaPath := testName + ".dat"
	st, path := setupTestDownload(t, int(filesize), ulSiaPath, true)
	defer func() {
		st.server.panicClose()
		os.Remove(path)
	}()

	// Read the section to be downloaded from the original file.
	uf, err := os.Open(path) // Uploaded file.
	if err != nil {
		return err
	}
	var originalBytes bytes.Buffer
	_, err = uf.Seek(offset, 0)
	if err != nil {
		return err
	}
	_, err = io.CopyN(&originalBytes, uf, length)
	if err != nil {
		return err
	}

	// Download the original file from the passed offsets.
	fname := testName + "-download.dat"
	downpath := filepath.Join(st.dir, fname)
	defer os.Remove(downpath)

	dlURL := fmt.Sprintf("/renter/download/%s?offset=%d&length=%d", ulSiaPath, offset, length)

	var downbytes bytes.Buffer

	if useHttpResp {
		dlURL += "&httpresp=true"
		// Make request.
		resp, err := HttpGET("http://" + st.server.listener.Addr().String() + dlURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		_, err = io.Copy(&downbytes, resp.Body)
		if err != nil {
			return err
		}
	} else {
		dlURL += "&destination=" + downpath
		err := st.getAPI(dlURL, nil)
		if err != nil {
			return err
		}
		// wait for the download to complete
		err = build.Retry(30, time.Second, func() error {
			var rdq RenterDownloadQueue
			err = st.getAPI("/renter/downloads", &rdq)
			if err != nil {
				return err
			}
			for _, download := range rdq.Downloads {
				if download.Received == download.Filesize && download.SiaPath == ulSiaPath {
					return nil
				}
			}
			return errors.New("file not downloaded")
		})
		if err != nil {
			t.Fatal(err)
		}

		// open the downloaded file
		df, err := os.Open(downpath)
		if err != nil {
			return err
		}
		defer df.Close()

		_, err = io.Copy(&downbytes, df)
		if err != nil {
			return err
		}
	}

	// should have correct length
	if int64(downbytes.Len()) != length {
		return fmt.Errorf("downloaded file has incorrect size: %d, %d expected", downbytes.Len(), length)
	}

	// should be byte-for-byte equal to the original uploaded file
	if !bytes.Equal(originalBytes.Bytes(), downbytes.Bytes()) {
		return fmt.Errorf("downloaded content differs from original content")
	}

	return nil
}

// TestRenterDownloadError tests that the /renter/download route sets the
// download's error field if it fails.
func TestRenterDownloadError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, _ := setupTestDownload(t, 1e4, "test.dat", false)
	defer st.server.Close()

	// don't wait for the upload to complete, try to download immediately to
	// intentionally cause a download error
	downpath := filepath.Join(st.dir, "down.dat")
	expectedErr := st.getAPI("/renter/download/test.dat?destination="+downpath, nil)
	if expectedErr == nil {
		t.Fatal("download unexpectedly succeeded")
	}

	// verify the file has the expected error
	var rdq RenterDownloadQueue
	err := st.getAPI("/renter/downloads", &rdq)
	if err != nil {
		t.Fatal(err)
	}
	for _, download := range rdq.Downloads {
		if download.SiaPath == "test.dat" && download.Received == download.Filesize && download.Error == expectedErr.Error() {
			t.Fatal("download had unexpected error: ", download.Error)
		}
	}
}

// TestValidDownloads tests valid and boundary parameter combinations.
func TestValidDownloads(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	sectorSize := int64(modules.SectorSize)

	testParams := []struct {
		filesize,
		offset,
		length int64
		useHttpResp bool
		testName    string
	}{
		// file-backed tests.
		{sectorSize, 40, sectorSize - 40, false, "OffsetSingleChunk"},
		{sectorSize * 2, 20, sectorSize*2 - 20, false, "OffsetTwoChunk"},
		{int64(float64(sectorSize) * 2.4), 20, int64(float64(sectorSize)*2.4) - 20, false, "OffsetThreeChunk"},
		{sectorSize, 0, sectorSize / 2, false, "ShortLengthSingleChunk"},
		{sectorSize, sectorSize / 4, sectorSize / 2, false, "ShortLengthAndOffsetSingleChunk"},
		{sectorSize * 2, 0, int64(float64(sectorSize) * 2 * 0.75), false, "ShortLengthTwoChunk"},
		{int64(float64(sectorSize) * 2.7), 0, int64(2.2 * float64(sectorSize)), false, "ShortLengthThreeChunkInThirdChunk"},
		{int64(float64(sectorSize) * 2.7), 0, int64(1.6 * float64(sectorSize)), false, "ShortLengthThreeChunkInSecondChunk"},
		{sectorSize * 5, 0, int64(float64(sectorSize*5) * 0.75), false, "ShortLengthMultiChunk"},
		{sectorSize * 2, 50, int64(float64(sectorSize*2) * 0.75), false, "ShortLengthAndOffsetTwoChunk"},
		{sectorSize * 3, 50, int64(float64(sectorSize*3) * 0.5), false, "ShortLengthAndOffsetThreeChunkInSecondChunk"},
		{sectorSize * 3, 50, int64(float64(sectorSize*3) * 0.75), false, "ShortLengthAndOffsetThreeChunkInThirdChunk"},

		// http response tests.
		{sectorSize, 40, sectorSize - 40, true, "HttpRespOffsetSingleChunk"},
		{sectorSize * 2, 40, sectorSize*2 - 40, true, "HttpRespOffsetTwoChunk"},
		{sectorSize * 5, 40, sectorSize*5 - 40, true, "HttpRespOffsetManyChunks"},
		{sectorSize, 40, 4 * sectorSize / 5, true, "RespOffsetAndLengthSingleChunk"},
		{sectorSize * 2, 80, 3 * (sectorSize * 2) / 4, true, "RespOffsetAndLengthTwoChunk"},
		{sectorSize * 5, 150, 3 * (sectorSize * 5) / 4, true, "HttpRespOffsetAndLengthManyChunks"},
		{sectorSize * 5, 150, sectorSize * 5 / 4, true, "HttpRespOffsetAndLengthManyChunksSubsetOfChunks"},
	}
	for i, params := range testParams {
		params := params
		t.Run(fmt.Sprintf("%v-%v", t.Name(), i), func(st *testing.T) {
			st.Parallel()
			err := runDownloadTest(st, params.filesize, params.offset, params.length, params.useHttpResp, params.testName)
			if err != nil {
				st.Fatal(err)
			}
		})
	}
}

func runDownloadParamTest(t *testing.T, length, offset, filesize int) error {
	ulSiaPath := "test.dat"

	st, _ := setupTestDownload(t, int(filesize), ulSiaPath, true)
	defer st.server.Close()

	// Download the original file from offset 40 and length 10.
	fname := "offsetsinglechunk.dat"
	downpath := filepath.Join(st.dir, fname)
	dlURL := fmt.Sprintf("/renter/download/%s?destination=%s", ulSiaPath, downpath)
	dlURL += fmt.Sprintf("&length=%d", length)
	dlURL += fmt.Sprintf("&offset=%d", offset)
	return st.getAPI(dlURL, nil)
}

func TestInvalidDownloadParameters(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()

	testParams := []struct {
		length   int
		offset   int
		filesize int
		errorMsg string
	}{
		{0, -10, 1e4, "/download not prompting error when passing negative offset."},
		{0, 1e4, 1e4, "/download not prompting error when passing offset equal to filesize."},
		{1e4 + 1, 0, 1e4, "/download not prompting error when passing length exceeding filesize."},
		{1e4 + 11, 10, 1e4, "/download not prompting error when passing length exceeding filesize with non-zero offset."},
		{-1, 0, 1e4, "/download not prompting error when passing negative length."},
	}

	for _, params := range testParams {
		err := runDownloadParamTest(t, params.length, params.offset, params.filesize)
		if err == nil {
			t.Fatal(params.errorMsg)
		}
	}
}

func TestRenterDownloadAsyncAndHttpRespError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	filesize := 1e4
	ulSiaPath := "test.dat"

	st, _ := setupTestDownload(t, int(filesize), ulSiaPath, true)
	defer st.server.Close()

	// Download the original file from offset 40 and length 10.
	fname := "offsetsinglechunk.dat"
	dlURL := fmt.Sprintf("/renter/download/%s?destination=%s&async=true&httpresp=true", ulSiaPath, fname)
	err := st.getAPI(dlURL, nil)
	if err == nil {
		t.Fatalf("/download not prompting error when only passing both async and httpresp fields.")
	}
}

func TestRenterDownloadAsyncNonexistentFile(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	downpath := filepath.Join(st.dir, "testfile")
	err = st.getAPI(fmt.Sprintf("/renter/downloadasync/doesntexist?destination=%v", downpath), nil)
	if err == nil || err.Error() != fmt.Sprintf("download failed: no file with that path: doesntexist") {
		t.Fatal("downloadasync did not return error on nonexistent file")
	}
}

func TestRenterDownloadAsyncAndNotDestinationError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	filesize := 1e4
	ulSiaPath := "test.dat"

	st, _ := setupTestDownload(t, int(filesize), ulSiaPath, true)
	defer st.server.Close()

	// Download the original file from offset 40 and length 10.
	dlURL := fmt.Sprintf("/renter/download/%s?async=true", ulSiaPath)
	err := st.getAPI(dlURL, nil)
	if err == nil {
		t.Fatal("/download not prompting error when async is specified but destination is empty.")
	}
}

func TestRenterDownloadHttpRespAndDestinationError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	filesize := 1e4
	ulSiaPath := "test.dat"

	st, _ := setupTestDownload(t, int(filesize), ulSiaPath, true)
	defer st.server.Close()

	// Download the original file from offset 40 and length 10.
	fname := "test.dat"
	dlURL := fmt.Sprintf("/renter/download/%s?destination=%shttpresp=true", ulSiaPath, fname)
	err := st.getAPI(dlURL, nil)
	if err == nil {
		t.Fatal("/download not prompting error when httpresp is specified and destination is non-empty.")
	}
}

// TestRenterAsyncDownloadError tests that the /renter/asyncdownload route sets
// the download's error field if it fails.
func TestRenterAsyncDownloadError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, _ := setupTestDownload(t, 1e4, "test.dat", false)
	defer st.server.panicClose()

	// don't wait for the upload to complete, try to download immediately to
	// intentionally cause a download error
	downpath := filepath.Join(st.dir, "asyncdown.dat")
	st.getAPI("/renter/downloadasync/test.dat?destination="+downpath, nil)

	// verify the file has an error
	var rdq RenterDownloadQueue
	err := st.getAPI("/renter/downloads", &rdq)
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

	st, _ := setupTestDownload(t, 1e4, "test.dat", true)
	defer st.server.panicClose()

	// Download the file asynchronously.
	downpath := filepath.Join(st.dir, "asyncdown.dat")
	err := st.getAPI("/renter/downloadasync/test.dat?destination="+downpath, nil)
	if err != nil {
		t.Fatal(err)
	}

	// download should eventually complete
	var rdq RenterDownloadQueue
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
	defer st.server.panicClose()

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
	defer st.server.panicClose()

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
	defer st.server.panicClose()

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

	// The renter should now have 1 contract.
	if err = st.getAPI("/renter/contracts", &contracts); err != nil {
		t.Fatal(err)
	}
	if len(contracts.Contracts) != 1 {
		t.Fatalf("expected renter to have 1 contract; got %v", len(contracts.Contracts))
	}
	if !contracts.Contracts[0].GoodForUpload || !contracts.Contracts[0].GoodForRenew {
		t.Errorf("expected contract to be good for upload and renew")
	}

	// Check the renter's contract spending.
	var get RenterGET
	if err = st.getAPI("/renter", &get); err != nil {
		t.Fatal(err)
	}
	expectedContractSpending := types.ZeroCurrency
	for _, contract := range contracts.Contracts {
		expectedContractSpending = expectedContractSpending.Add(contract.TotalCost)
	}
	if got := get.FinancialMetrics.TotalAllocated; got.Cmp(expectedContractSpending) != 0 {
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
	defer st.server.panicClose()

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
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
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
	// Try an invalid period string.
	allowanceValues.Set("period", "-1")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || !strings.Contains(err.Error(), "unable to parse period") {
		t.Errorf("expected error to begin with 'unable to parse period'; got %v", err)
	}
	// Try to set a zero renew window
	allowanceValues.Set("period", "2")
	allowanceValues.Set("renewwindow", "0")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil || err.Error() != contractor.ErrAllowanceZeroWindow.Error() {
		t.Errorf("expected error to be %v, got %v", contractor.ErrAllowanceZeroWindow, err)
	}
	// Try to set a negative bandwidth limit
	allowanceValues.Set("maxdownloadspeed", "-1")
	allowanceValues.Set("renewwindow", "1")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil {
		t.Errorf("expected error to be 'download/upload rate limit...'; got %v", err)
	}
	allowanceValues.Set("maxuploadspeed", "-1")
	err = st.stdPostAPI("/renter", allowanceValues)
	if err == nil {
		t.Errorf("expected error to be 'download/upload rate limit...'; got %v", err)
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
	defer st.server.panicClose()

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
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
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
	hasPrefix := strings.HasPrefix(err.Error(), "download failed: no file with that path")
	if err == nil || !hasPrefix {
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
	defer st.server.panicClose()

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
	defer st.server.panicClose()

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
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
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
	defer st.server.panicClose()

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
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
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
	defer st.server.panicClose()

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
	allowanceValues.Set("renewwindow", testRenewWindow)
	allowanceValues.Set("hosts", fmt.Sprint(recommendedHosts))
	if err = st.stdPostAPI("/renter", allowanceValues); err != nil {
		t.Fatal(err)
	}

	renterDownloadAbsoluteError := "download failed: destination must be an absolute path"

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
	for i := 0; i < 200 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(200 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress < 10 {
		t.Fatal("the uploading is not succeeding for some reason:", rf.Files[0])
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
	defer st.server.panicClose()

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
	defer stHost1.panicClose()
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost2.panicClose()

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
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

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
	defer stHost1.panicClose()
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost2.panicClose()

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
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

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
	defer stHost1.panicClose()
	stHost2, err := blankServerTester(t.Name() + " - Host 2")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost2.panicClose()
	stHost3, err := blankServerTester(t.Name() + " - Host 3")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost3.panicClose()
	stHost4, err := blankServerTester(t.Name() + " - Host 4")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost4.panicClose()
	stHost5, err := blankServerTester(t.Name() + " - Host 5")
	if err != nil {
		t.Fatal(err)
	}
	defer stHost5.panicClose()

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
	defer st.server.panicClose()

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

// TestContractorHostRemoval checks that the contractor properly migrates away
// from low quality hosts when there are higher quality hosts available.
func TestContractorHostRemoval(t *testing.T) {
	// Create a renter and 2 hosts. Connect to the hosts and start uploading.
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	st, err := createServerTester(t.Name() + "renter")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()
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
	// Raise the prices significantly for the two hosts.
	raisedPrice := url.Values{}
	raisedPrice.Set("mincontractprice", "5000000000000000000000000000") // 5 KS
	raisedPrice.Set("period", testPeriod)
	err = st.stdPostAPI("/host", raisedPrice)
	if err != nil {
		t.Fatal(err)
	}
	err = stH1.stdPostAPI("/host", raisedPrice)
	if err != nil {
		t.Fatal(err)
	}
	// Anounce the hosts.
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance with two hosts.
	allowanceValues := url.Values{}
	allowanceValues.Set("funds", "500000000000000000000000000000") // 500k SC
	allowanceValues.Set("hosts", "2")
	allowanceValues.Set("period", "15")
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
	origBytes, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// upload the file
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("datapieces", "1")
	uploadValues.Set("paritypieces", "1")
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}

	// redundancy should reach 2
	var rf RenterFiles
	err = build.Retry(120, 250*time.Millisecond, func() error {
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
	downloadBytes, err := ioutil.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downloadBytes, origBytes) {
		t.Fatal("downloaded file and uploaded file do not match")
	}

	// Get the values of the first and second contract.
	var rc RenterContracts
	err = st.getAPI("/renter/contracts", &rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Contracts) != 2 {
		t.Fatal("wrong contract count")
	}
	rc1Host := rc.Contracts[0].HostPublicKey.String()
	rc2Host := rc.Contracts[1].HostPublicKey.String()

	// Add 3 new hosts that will be competing with the expensive hosts.
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
	stH4, err := blankServerTester(t.Name() + " - Host 4")
	if err != nil {
		t.Fatal(err)
	}
	defer stH4.server.Close()
	testGroup = []*serverTester{st, stH1, stH2, stH3, stH4}
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
	err = addStorageToAllHosts([]*serverTester{stH2, stH3, stH4})
	if err != nil {
		t.Fatal(err)
	}
	// Anounce the hosts.
	err = announceAllHosts(testGroup)
	if err != nil {
		t.Fatal(err)
	}

	// Block until the hostdb reaches five hosts.
	err = build.Retry(150, time.Millisecond*250, func() error {
		var ah HostdbActiveGET
		err = st.getAPI("/hostdb/active", &ah)
		if err != nil {
			return err
		}
		if len(ah.Hosts) < 5 {
			return errors.New("new hosts never appeared in hostdb")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block to trigger a second run of threadedContractMaintenance.
	_, err = st.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Verify that st and stH1 are dropped in favor of the newer, better hosts.
	err = build.Retry(150, time.Millisecond*250, func() error {
		var newContracts int
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		hostMap := make(map[string]struct{})
		hostMap[rc1Host] = struct{}{}
		hostMap[rc2Host] = struct{}{}
		for _, contract := range rc.Contracts {
			_, exists := hostMap[contract.HostPublicKey.String()]
			if !exists {
				newContracts++
				hostMap[contract.HostPublicKey.String()] = struct{}{}
			}
		}
		if newContracts != 2 {
			return fmt.Errorf("not the right number of new contracts: %v", newContracts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Block until redundancy is restored to 2.
	err = build.Retry(120, 250*time.Millisecond, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) == 1 && rf.Files[0].Redundancy == 2 {
			return nil
		}
		return errors.New("file not uploaded to full redundancy")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Grab the old contracts, then mine blocks to trigger a renew, and then
	// wait until the renew is complete.
	err = st.getAPI("/renter/contracts", &rc)
	if err != nil {
		t.Fatal(err)
	}
	// Check the amount of data in each contract.
	for _, contract := range rc.Contracts {
		if contract.Size != modules.SectorSize {
			t.Error("Each contrat should have 1 sector:", contract.Size, contract.ID)
		}
	}
	// Mine blocks to force a contract renewal.
	for i := 0; i < 11; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		_, err = synchronizationCheck(testGroup)
		if err != nil {
			t.Fatal(err)
		}
		// Sleep to give the contractor some time to perform the renew.
		time.Sleep(time.Millisecond * 100)
	}
	// Give the renter time to renew. Two of the contracts should renew.
	var rc2 RenterContracts
	err = build.Retry(50, time.Millisecond*250, func() error {
		err = st.getAPI("/renter/contracts", &rc2)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}

		// Check that at least 2 contracts are different between rc and rc2.
		tracker := make(map[types.FileContractID]struct{})
		// Add all the contracts.
		for _, contract := range rc.Contracts {
			tracker[contract.ID] = struct{}{}
		}
		// Count the number of contracts that were not seen in the previous
		// batch of contracts, and check that the new contracts are not with the
		// expensive hosts.
		var unseen int
		for _, contract := range rc2.Contracts {
			_, exists := tracker[contract.ID]
			if !exists {
				unseen++
				tracker[contract.ID] = struct{}{}
				if contract.HostPublicKey.String() == rc1Host || contract.HostPublicKey.String() == rc2Host {
					return errors.New("the wrong contracts are being renewed")
				}
			}
		}
		if unseen != 2 {
			return fmt.Errorf("the wrong number of contracts seem to be getting renewed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The renewing process should not have resulted in additional data being
	// uploaded - it should be the same data in the contracts.
	for _, contract := range rc2.Contracts {
		if contract.Size != modules.SectorSize {
			t.Error("Contract has the wrong size:", contract.Size)
		}
	}

	// Try again to download the file we uploaded. It should still be
	// retrievable.
	downloadPath2 := filepath.Join(st.dir, "test-downloaded-verify-2.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath2)
	if err != nil {
		t.Fatal(err)
	}
	downloadBytes2, err := ioutil.ReadFile(downloadPath2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downloadBytes2, origBytes) {
		t.Fatal("downloaded file and uploaded file do not match")
	}

	// Mine out another set of the blocks so that the bad contracts expire.
	for i := 0; i < 11; i++ {
		_, err := st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		_, err = synchronizationCheck(testGroup)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond * 100)
	}

	// Should be back down to 2 contracts now, with the new hosts. Verify that
	// st and stH1 are dropped in favor of the newer, better hosts. The
	// contracts should also have data in them.
	err = build.Retry(50, time.Millisecond*250, func() error {
		err = st.getAPI("/renter/contracts", &rc)
		if err != nil {
			return errors.New("couldn't get renter stats")
		}
		if len(rc.Contracts) != 2 {
			return fmt.Errorf("renewing seems to have failed: %v", len(rc.Contracts))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rc.Contracts[0].HostPublicKey.String() == rc1Host || rc.Contracts[0].HostPublicKey.String() == rc2Host {
		t.Error("renter is renewing the wrong contracts", rc.Contracts[0].HostPublicKey.String())
	}
	if rc.Contracts[1].HostPublicKey.String() == rc1Host || rc.Contracts[1].HostPublicKey.String() == rc2Host {
		t.Error("renter is renewing the wrong contracts", rc.Contracts[1].HostPublicKey.String())
	}
	// The renewing process should not have resulted in additional data being
	// uploaded - it should be the same data in the contracts.
	for _, contract := range rc.Contracts {
		if contract.Size != modules.SectorSize {
			t.Error("Contract has the wrong size:", contract.Size)
		}
	}
	// Redundancy should still be 2.
	err = build.Retry(120, 250*time.Millisecond, func() error {
		st.getAPI("/renter/files", &rf)
		if len(rf.Files) >= 1 && rf.Files[0].Redundancy == 2 {
			return nil
		}
		return errors.New("file not uploaded to full redundancy")
	})
	if err != nil {
		t.Fatal(err, "::", rf.Files[0].Redundancy)
	}

	// Try again to download the file we uploaded. It should still be
	// retrievable.
	downloadPath3 := filepath.Join(st.dir, "test-downloaded-verify-3.dat")
	err = st.stdGetAPI("/renter/download/test?destination=" + downloadPath3)
	if err != nil {
		t.Error("Final download has failed:", err)
	}
	downloadBytes3, err := ioutil.ReadFile(downloadPath3)
	if err != nil {
		t.Error(err)
	}
	if !bytes.Equal(downloadBytes3, origBytes) {
		t.Error("downloaded file and uploaded file do not match")
	}
}

// TestExhaustedContracts verifies that the contractor renews contracts which
// run out of funds before the period elapses.
func TestExhaustedContracts(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// set a very high price for the host so we use up the entire funds of the
	// contract
	err = st.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = st.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	// Increase the storage and bandwidth prices to drain the allowance faster.
	settings := st.host.InternalSettings()
	settings.MinUploadBandwidthPrice = settings.MinUploadBandwidthPrice.Mul64(2)
	settings.MinStoragePrice = settings.MinUploadBandwidthPrice.Mul64(2)
	settings.MaxDuration = 1e6 // set a high max duration to allow an expensive storage price
	err = st.host.SetInternalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testPeriod := "950000" // large period to cause an expensive test, exhausting the allowance faster
	allowanceValues.Set("funds", types.SiacoinPrecision.Mul64(10).String())
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// wait until we have a contract
	err = build.Retry(500, time.Millisecond*50, func() error {
		if len(st.renter.Contracts()) >= 1 {
			return nil
		}
		return errors.New("no renter contracts")
	})
	if err != nil {
		t.Fatal(err)
	}
	initialContract := st.renter.Contracts()[0]

	// upload a file. the high upload cost will cause the underlying contract to
	// require premature renewal. If premature renewal never happens, the upload
	// will never complete.
	path := filepath.Join(st.dir, "randUploadFile")
	size := int(modules.SectorSize * 75)
	err = createRandFile(path, size)
	if err != nil {
		t.Fatal(err)
	}
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	uploadValues.Set("datapieces", "1")
	uploadValues.Set("paritypieces", "1")
	err = st.stdPostAPI("/renter/upload/"+filepath.Base(path), uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(100, time.Millisecond*500, func() error {
		// mine blocks each iteration to trigger contract maintenance
		_, err = st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}

		st.miner.AddBlock()
		if !st.renter.FileList()[0].Available {
			return errors.New("file did not complete uploading")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// verify that the window did not change and the contract ID did change
	newContract := st.renter.Contracts()[0]
	endHeight := newContract.EndHeight
	if newContract.ID == initialContract.ID {
		t.Error("renew did not occur")
	}
	if endHeight != initialContract.EndHeight {
		t.Error("contract end height changed, wanted", initialContract.EndHeight, "got", endHeight)
	}
}

// TestAdversarialPriceRenewal verifies that host cannot maliciously raise
// their storage price in order to trigger a premature file contract renewal.
func TestAdversarialPriceRenewal(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	// announce our host
	err = st.acceptContracts()
	if err != nil {
		t.Fatal(err)
	}
	err = st.setHostStorage()
	if err != nil {
		t.Fatal(err)
	}
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// Set an allowance for the renter, allowing a contract to be formed.
	allowanceValues := url.Values{}
	testPeriod := "10000"
	allowanceValues.Set("funds", types.SiacoinPrecision.Mul64(10000).String())
	allowanceValues.Set("period", testPeriod)
	err = st.stdPostAPI("/renter", allowanceValues)
	if err != nil {
		t.Fatal(err)
	}

	// wait until we have a contract
	err = build.Retry(500, time.Millisecond*50, func() error {
		if len(st.renter.Contracts()) >= 1 {
			return nil
		}
		return errors.New("no renter contracts")
	})
	if err != nil {
		t.Fatal(err)
	}

	// upload a file
	path := filepath.Join(st.dir, "randUploadFile")
	size := int(modules.SectorSize * 50)
	err = createRandFile(path, size)
	if err != nil {
		t.Fatal(err)
	}
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	uploadValues.Set("renew", "true")
	uploadValues.Set("datapieces", "1")
	uploadValues.Set("paritypieces", "1")
	err = st.stdPostAPI("/renter/upload/"+filepath.Base(path), uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(100, time.Millisecond*500, func() error {
		// mine blocks each iteration to trigger contract maintenance
		_, err = st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		if !st.renter.FileList()[0].Available {
			return errors.New("file did not complete uploading")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	initialID := st.renter.Contracts()[0].ID

	// jack up the host's storage price to try to trigger a renew
	settings := st.host.InternalSettings()
	settings.MinStoragePrice = settings.MinStoragePrice.Mul64(10800000000)
	err = st.host.SetInternalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		_, err = st.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		if st.renter.Contracts()[0].ID != initialID {
			t.Fatal("changing host price caused renew")
		}
		time.Sleep(time.Millisecond * 100)
	}
}
