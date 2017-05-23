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
	testFunds  = "10000000000000000000000000000" // 10k SC
	testPeriod = "5"
)

// createRandFile creates a file on disk and fills it with random bytes.
func createRandFile(path string, size int) error {
	return ioutil.WriteFile(path, fastrand.Bytes(size), 0600)
}

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
	allowanceValues.Set("funds", testFunds)
	allowanceValues.Set("period", testPeriod)
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
		err = retry(200, time.Second, func() error {
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

func waitForDownloadToComplete(t *testing.T, st *serverTester, siapath string, errmsg string) {
	var rdq RenterDownloadQueue

	// download should eventually complete
	success := false
	for start := time.Now(); time.Since(start) < 30*time.Second; time.Sleep(time.Millisecond * 10) {
		err := st.getAPI("/renter/downloads", &rdq)
		if err != nil {
			t.Fatal(err)
		}
		for _, download := range rdq.Downloads {
			if download.Received == download.Filesize && download.SiaPath == siapath {
				success = true
			}
		}
		if success {
			break
		}
	}
	if !success {
		t.Fatal(errmsg)
	}
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
		waitForDownloadToComplete(t, st, ulSiaPath, "/renter/download with offset failed.") // TODO: Fix error message.

		df, err := os.Open(downpath) // Downloaded file.
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
		return errors.New(fmt.Sprintf("downloaded file has incorrect size: %d, %d expected.", downbytes.Len(), length))
	}

	// should be byte-for-byte equal to the original uploaded file
	if bytes.Compare(originalBytes.Bytes(), downbytes.Bytes()) != 0 {
		return errors.New(fmt.Sprintf("downloaded content differs from original content"))
	}

	return nil
}

// TestRenterDownloadError tests that the /renter/download route sets the download's error field if it fails.
func TestRenterDownloadError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, _ := setupTestDownload(t, 1e4, "test.dat", false)
	defer st.server.Close()

	// don't wait for the upload to complete, try to download immediately to intentionally cause a download error
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

	blk := int64(modules.SectorSize)

	testParams := []struct {
		filesize,
		offset,
		length int64
		useHttpResp bool
		testName    string
	}{
		// File-backed tests.
		{blk, 40, blk - 40, false, "OffsetSingleChunk"},
		{blk * 2, 20, blk*2 - 20, false, "OffsetTwoChunk"},
		{int64(float64(blk) * 2.4), 20, int64(float64(blk)*2.4) - 20, false, "OffsetThreeChunk"},
		{blk, 0, blk / 2, false, "ShortLengthSingleChunk"},
		{blk, blk / 4, blk / 2, false, "ShortLengthAndOffsetSingleChunk"},
		{blk * 2, 0, int64(float64(blk) * 2 * 0.75), false, "ShortLengthTwoChunk"},
		{int64(float64(blk) * 2.7), 0, int64(2.2 * float64(blk)), false, "ShortLengthThreeChunkInThirdChunk"},
		{int64(float64(blk) * 2.7), 0, int64(1.6 * float64(blk)), false, "ShortLengthThreeChunkInSecondChunk"},
		{blk * 5, 0, int64(float64(blk*5) * 0.75), false, "ShortLengthMultiChunk"},
		{blk * 2, 50, int64(float64(blk*2) * 0.75), false, "ShortLengthAndOffsetTwoChunk"},
		{blk * 3, 50, int64(float64(blk*3) * 0.5), false, "ShortLengthAndOffsetThreeChunkInSecondChunk"},
		{blk * 3, 50, int64(float64(blk*3) * 0.75), false, "ShortLengthAndOffsetThreeChunkInThirdChunk"},

		// Http response tests.
		{blk, 40, blk - 40, true, "HttpRespOffsetSingleChunk"},
		{blk * 2, 40, blk*2 - 40, true, "HttpRespOffsetTwoChunk"},
		{blk * 5, 40, blk*5 - 40, true, "HttpRespOffsetManyChunks"},
		{blk, 40, 4 * blk / 5, true, "RespOffsetAndLengthSingleChunk"},
		{blk * 2, 80, 3 * (blk * 2) / 4, true, "RespOffsetAndLengthTwoChunk"},
		{blk * 5, 150, 3 * (blk * 5) / 4, true, "HttpRespOffsetAndLengthManyChunks"},
		{blk * 5, 150, blk * 5 / 4, true, "HttpRespOffsetAndLengthManyChunksSubsetOfChunks"},
	}

	for _, params := range testParams {
		err := runDownloadTest(t, params.filesize, params.offset, params.length, params.useHttpResp, params.testName)
		if err != nil {
			t.Fatalf("Test %s failed: %s", params.testName, err.Error())
		}
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
	if testing.Short() {
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

// TestRenterAsyncDownloadError tests that the /renter/asyncdownload route sets the download's error field if it fails.
func TestRenterAsyncDownloadError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, _ := setupTestDownload(t, 1e4, "test.dat", false)
	defer st.server.panicClose()

	// don't wait for the upload to complete, try to download immediately to intentionally cause a download error
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

func TestRenterAsyncSpecifyAsyncFalseError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, _ := setupTestDownload(t, 1e4, "test.dat", false)
	defer st.server.Close()

	// don't wait for the upload to complete, try to download immediately to intentionally cause a download error
	downpath := filepath.Join(st.dir, "asyncdown.dat")
	err := st.getAPI("/renter/downloadasync/test.dat?async=false&destination="+downpath, nil)
	if err == nil {
		t.Fatal("/downloadasync does not return error when passing `async=false`")
	}
}

// TestRenterAsyncDownload tests that the /renter/downloadasync route works
// correctly.
func TestRenterAsyncDownload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	st, _ := setupTestDownload(t, 4e5, "test.dat", true)
	defer st.server.panicClose()

	// Download the file asynchronously.
	downpath := filepath.Join(st.dir, "asyncdown.dat")
	err := st.getAPI("/renter/downloadasync/test.dat?destination="+downpath, nil)
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
