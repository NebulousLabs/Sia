package host

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/types"
)

const (
	testUploadDuration = 20 // Duration in blocks of a standard upload during testing.

	// Helper variables to indicate whether renew is being toggled as input to
	// uploadFile.
	renewEnabled  = true
	renewDisabled = false
)

// uploadFile uploads a file to the host from the tester's renter. The data
// used to make the file is returned. The nickname of the file in the renter is
// the same as the name provided as input.
func (ht *hostTester) uploadFile(name string, renew bool) ([]byte, error) {
	// Check that renting is initialized properly.
	err := ht.initRenting()
	if err != nil {
		return nil, err
	}

	// Create a file to upload to the host.
	filepath := filepath.Join(ht.persistDir, name+".testfile")
	datasize := uint64(1024)
	data, err := crypto.RandBytes(int(datasize))
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(filepath, data, 0600)
	if err != nil {
		return nil, err
	}

	// Have the renter upload to the host.
	rsc, err := renter.NewRSCode(1, 1)
	if err != nil {
		return nil, err
	}
	fup := modules.FileUploadParams{
		Filename:    filepath,
		Nickname:    name,
		Duration:    testUploadDuration,
		Renew:       renew,
		ErasureCode: rsc,
		PieceSize:   0,
	}
	err = ht.renter.Upload(fup)
	if err != nil {
		return nil, err
	}

	// Wait until the upload has finished.
	for i := 0; i < 100; i++ {
		time.Sleep(time.Millisecond * 100)

		// Asynchronous processes in the host access obligations by id,
		// therefore a lock is required to scan the set of obligations.
		if func() bool {
			ht.host.mu.Lock()
			defer ht.host.mu.Unlock()

			for _, ob := range ht.host.obligationsByID {
				if ob.fileSize() >= datasize {
					return true
				}
			}
			return false
		}() {
			break
		}
	}

	// The rest of the upload can be performed under lock.
	ht.host.mu.Lock()
	defer ht.host.mu.Unlock()

	if len(ht.host.obligationsByID) != 1 {
		return nil, errors.New("expecting a single obligation")
	}
	for _, ob := range ht.host.obligationsByID {
		if ob.fileSize() >= datasize {
			return data, nil
		}
	}
	return nil, errors.New("ht.uploadFile: upload failed")
}

// TestRPCUPload attempts to upload a file to the host, adding coverage to the
// upload function.
func TestRPCUpload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRPCUpload")
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	baselineAnticipatedRevenue := ht.host.anticipatedRevenue
	baselineSpace := ht.host.spaceRemaining
	ht.host.mu.RUnlock()
	_, err = ht.uploadFile("TestRPCUpload - 1", renewDisabled)
	if err != nil {
		t.Fatal(err)
	}

	var expectedRevenue types.Currency
	func() {
		ht.host.mu.RLock()
		defer ht.host.mu.RUnlock()

		if ht.host.anticipatedRevenue.Cmp(baselineAnticipatedRevenue) <= 0 {
			t.Error("Anticipated revenue did not increase after a file was uploaded")
		}
		if baselineSpace <= ht.host.spaceRemaining {
			t.Error("space remaining on the host does not seem to have decreased")
		}
		expectedRevenue = ht.host.anticipatedRevenue
	}()

	// Mine until the storage proof goes through, and the obligation gets
	// cleared.
	for i := 0; i <= testUploadDuration+confirmationRequirement+testingWindowSize; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check that the storage proof has succeeded.
	ht.host.mu.Lock()
	defer ht.host.mu.Unlock()
	if len(ht.host.obligationsByID) != 0 {
		t.Error("host still has obligation, when it should have completed the obligation and submitted a storage proof.")
	}
	if !ht.host.anticipatedRevenue.IsZero() {
		t.Error("host anticipated revenue was not set back to zero")
	}
	if ht.host.spaceRemaining != baselineSpace {
		t.Error("host does not seem to have reclaimed the space after a successful obligation")
	}
	if expectedRevenue.Cmp(ht.host.revenue) != 0 {
		t.Error("host's revenue was not moved from anticipated to expected")
	}
}

// TestRPCRenew attempts to upload a file to the host, adding coverage to the
// upload function.
func TestRPCRenew(t *testing.T) {
	t.Skip("test skipped because the renter renew function isn't block based")
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRPCRenew")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ht.uploadFile("TestRPCRenew- 1", renewEnabled)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	expectedRevenue := ht.host.anticipatedRevenue
	expectedSpaceRemaining := ht.host.spaceRemaining
	ht.host.mu.RUnlock()

	// Mine until the storage proof goes through, and the obligation gets
	// cleared.
	for i := 0; i <= testUploadDuration+confirmationRequirement+testingWindowSize; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check that the rewards for the first obligation went through, and that
	// there is another from the contract being renewed.
	ht.host.mu.Lock()
	defer ht.host.mu.Unlock()
	if len(ht.host.obligationsByID) != 1 {
		t.Error("file contract was not renenwed after being completed")
	}
	if ht.host.anticipatedRevenue.IsZero() {
		t.Error("host anticipated revenue should be nonzero")
	}
	if ht.host.spaceRemaining != expectedSpaceRemaining {
		t.Error("host space remaining changed after a renew happened")
	}
	if expectedRevenue.Cmp(ht.host.revenue) > 0 {
		t.Error("host's revenue was not increased though a proof was successful")
	}

	// TODO: Download the file that got renewed, see if the data is correct.
}

// TestFailedObligation tests that the host correctly handles missing a storage
// proof.
func TestFailedObligation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestFailedObligation")
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	baselineSpace := ht.host.spaceRemaining
	ht.host.mu.RUnlock()
	_, err = ht.uploadFile("TestFailedObligation - 1", renewDisabled)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	expectedLostRevenue := ht.host.anticipatedRevenue
	ht.host.mu.RUnlock()

	// Close the host, then mine enough blocks that the host has missed the
	// storage proof window.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= testUploadDuration+testingWindowSize; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Restart the host. While catching up, the host should realize that it
	// missed a storage proof, and should delete the obligation.
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	// Host should delete the obligation before finishing startup.
	rebootHost.mu.Lock()
	defer rebootHost.mu.Unlock()
	if len(rebootHost.obligationsByID) != 0 {
		t.Error("host did not delete a dead storage proof at startup")
	}
	if !rebootHost.anticipatedRevenue.IsZero() {
		t.Error("host did not subtract out anticipated revenue")
	}
	if rebootHost.spaceRemaining != baselineSpace {
		t.Error("host did not reallocate space after failed storage proof")
	}
	if rebootHost.lostRevenue.Cmp(expectedLostRevenue) != 0 {
		t.Error("host did not correctly report lost revenue")
	}
}
