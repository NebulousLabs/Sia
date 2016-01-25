package host

import (
	"bytes"
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
func (ht *hostTester) uploadFile(path string, renew bool) ([]byte, error) {
	// Check that renting is initialized properly.
	err := ht.initRenting()
	if err != nil {
		return nil, err
	}

	// Create a file to upload to the host.
	source := filepath.Join(ht.persistDir, path+".testfile")
	datasize := uint64(1024)
	data, err := crypto.RandBytes(int(datasize))
	if err != nil {
		return nil, err
	}
	dataMerkleRoot, err := crypto.ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(source, data, 0600)
	if err != nil {
		return nil, err
	}

	// Have the renter upload to the host.
	rsc, err := renter.NewRSCode(1, 1)
	if err != nil {
		return nil, err
	}
	fup := modules.FileUploadParams{
		Source:      source,
		SiaPath:     path,
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
				if dataMerkleRoot == ob.merkleRoot() {
					return true
				}
			}
			return false
		}() {
			break
		}
	}

	// Block until the renter is at 50 upload progress - it takes time for the
	// contract to confirm renter-side.
	complete := false
	for i := 0; i < 50 && !complete; i++ {
		fileInfos := ht.renter.FileList()
		for _, fileInfo := range fileInfos {
			if fileInfo.UploadProgress >= 50 {
				complete = true
			}
		}
		if complete {
			break
		}
		time.Sleep(time.Millisecond * 50)
	}
	if !complete {
		return nil, errors.New("renter never recognized that the upload completed")
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

	// Upload the file.
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
	for i := types.BlockHeight(0); i <= testUploadDuration+confirmationRequirement+defaultWindowSize; i++ {
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

	// Check that the file has been removed from the host directory.
	fileInfos, err := ioutil.ReadDir(filepath.Join(ht.persistDir, modules.HostDir))
	if len(fileInfos) != 2 {
		t.Error("too many files in directory after storage proof completed")
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.Name() != "host.log" && fileInfo.Name() != "settings.json" {
			t.Error("unexpected file after storage proof", fileInfo.Name())
		}
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
	for i := types.BlockHeight(0); i <= testUploadDuration+confirmationRequirement+defaultWindowSize; i++ {
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
	for i := types.BlockHeight(0); i <= testUploadDuration+defaultWindowSize+2; i++ {
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
		t.Error(rebootHost.lostRevenue)
		t.Error(expectedLostRevenue)
		t.Error("host did not correctly report lost revenue")
	}
}

// TestRestartSuccessObligation tests that a host who went offline for a few
// blocks is still able to successfully submit a storage proof.
func TestRestartSuccessObligation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRestartSuccessObligation")
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	baselineSpace := ht.host.spaceRemaining
	ht.host.mu.RUnlock()
	_, err = ht.uploadFile("TestRestartSuccessObligation - 1", renewDisabled)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	expectedRevenue := ht.host.anticipatedRevenue
	ht.host.mu.RUnlock()

	// Close the host, then mine some blocks, but not enough that the host
	// misses the storage proof.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 3; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Restart the host, and mine enough blocks that the host can submit a
	// successful storage proof.
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	if rebootHost.blockHeight != ht.cs.Height() {
		t.Error("Host block height does not match the cs block height")
	}
	for i := types.BlockHeight(0); i <= testUploadDuration+defaultWindowSize+confirmationRequirement-5; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Confirm that the storage proof was successful.
	rebootHost.mu.Lock()
	defer rebootHost.mu.Unlock()
	if len(rebootHost.obligationsByID) != 0 {
		t.Error("host did not delete a finished obligation")
	}
	if !rebootHost.anticipatedRevenue.IsZero() {
		t.Error("host did not subtract out anticipated revenue")
	}
	if rebootHost.spaceRemaining != baselineSpace {
		t.Error("host did not reallocate space after storage proof")
	}
	if rebootHost.revenue.Cmp(expectedRevenue) != 0 {
		t.Error("host did not correctly report revenue gains")
	}
}

// TestRestartCorruptSuccessObligation tests that a host who went offline for a
// few blocks, corrupted the consensus database, but is still able to correctly
// create a storage proof.
func TestRestartCorruptSuccessObligation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRestartCorruptSuccessObligation")
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	baselineSpace := ht.host.spaceRemaining
	ht.host.mu.RUnlock()
	_, err = ht.uploadFile("TestRestartCorruptSuccessObligation - 1", renewDisabled)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.RLock()
	expectedRevenue := ht.host.anticipatedRevenue
	ht.host.mu.RUnlock()

	// Corrupt the host's consensus tracking, close the host, then mine some
	// blocks, but not enough that the host misses the storage proof. The host
	// will need to perform a rescan and update its obligations correctly.
	ht.host.mu.Lock()
	ht.host.recentChange[0]++
	ht.host.mu.Unlock()
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 3; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Restart the host, and mine enough blocks that the host can submit a
	// successful storage proof.
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	if rebootHost.blockHeight != ht.cs.Height() {
		t.Error("Host block height does not match the cs block height")
	}
	if len(rebootHost.obligationsByID) == 0 {
		t.Error("host did not correctly reload its obligation")
	}
	for i := types.BlockHeight(0); i <= testUploadDuration+defaultWindowSize+confirmationRequirement-3; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Confirm that the storage proof was successful.
	rebootHost.mu.Lock()
	defer rebootHost.mu.Unlock()
	if len(rebootHost.obligationsByID) != 0 {
		t.Error("host did not delete a finished obligation")
	}
	if !rebootHost.anticipatedRevenue.IsZero() {
		t.Error("host did not subtract out anticipated revenue")
	}
	if rebootHost.spaceRemaining != baselineSpace {
		t.Error("host did not reallocate space after storage proof")
	}
	if rebootHost.revenue.Cmp(expectedRevenue) != 0 {
		t.Error("host did not correctly report revenue gains")
	}
	if rebootHost.lostRevenue.Cmp(expectedRevenue) == 0 {
		t.Error("host is reporting losses on the file contract")
	}
}

// TestUploadConstraints checks that file contract negotiation correctly
// rejects contracts that don't meet required criteria.
func TestUploadConstraints(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestUploadConstraints")
	if err != nil {
		t.Fatal(err)
	}
	h := ht.host
	settings := h.Settings()
	settings.TotalStorage = 10e3
	err = h.SetSettings(settings)
	if err != nil {
		t.Fatal(err)
	}

	// Create a valid file contract transaction.
	filesize := uint64(5e3)
	merkleRoot := crypto.Hash{51, 23}
	windowStart := ht.cs.Height() + 1 + settings.MinDuration
	windowEnd := ht.cs.Height() + 1 + settings.MinDuration + 1 + settings.WindowSize
	currencyDuration := types.NewCurrency64(1 + uint64(settings.MinDuration))
	payment := types.NewCurrency64(filesize).Mul(settings.Price).Mul(currencyDuration)
	payout := payment.Mul(types.NewCurrency64(20))
	refund := types.PostTax(ht.cs.Height(), payout).Sub(payment)
	renterKey := types.SiaPublicKey{}
	txn := types.Transaction{
		FileContracts: []types.FileContract{{
			FileSize:       filesize,
			FileMerkleRoot: merkleRoot,
			WindowStart:    windowStart,
			WindowEnd:      windowEnd,
			Payout:         payout,
			ValidProofOutputs: []types.SiacoinOutput{
				{
					Value:      refund,
					UnlockHash: types.UnlockHash{},
				},
				{
					Value:      payment,
					UnlockHash: settings.UnlockHash,
				},
			},
			MissedProofOutputs: []types.SiacoinOutput{
				{
					Value:      refund,
					UnlockHash: types.UnlockHash{},
				},
				{
					Value:      payment,
					UnlockHash: types.UnlockHash{},
				},
			},
			UnlockHash: types.UnlockConditions{
				PublicKeys:         []types.SiaPublicKey{renterKey, h.publicKey},
				SignaturesRequired: 2,
			}.UnlockHash(),
			RevisionNumber: 3,
		}},
	}
	err = h.considerContract(txn, renterKey, filesize, merkleRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Test that under-paid file contracts get rejected.
	underPayment := types.NewCurrency64(filesize * 5 / 6).Mul(settings.Price).Mul(currencyDuration)
	underRefund := types.PostTax(ht.cs.Height(), payout).Sub(underPayment)
	txn.FileContracts[0].ValidProofOutputs[0].Value = underRefund
	txn.FileContracts[0].ValidProofOutputs[1].Value = underPayment
	txn.FileContracts[0].MissedProofOutputs[0].Value = underRefund
	txn.FileContracts[0].MissedProofOutputs[1].Value = underPayment
	err = h.considerContract(txn, renterKey, filesize, merkleRoot)
	if err != ErrLowPayment {
		t.Fatal(err)
	}

	// Test that too-large files get rejected.
	largeFilesize := uint64(10001)
	largeFilePayment := types.NewCurrency64(largeFilesize).Mul(settings.Price).Mul(currencyDuration)
	largeFileRefund := types.PostTax(ht.cs.Height(), payout).Sub(largeFilePayment)
	txn.FileContracts[0].FileSize = largeFilesize
	txn.FileContracts[0].ValidProofOutputs[0].Value = largeFileRefund
	txn.FileContracts[0].ValidProofOutputs[1].Value = largeFilePayment
	txn.FileContracts[0].MissedProofOutputs[0].Value = largeFileRefund
	txn.FileContracts[0].MissedProofOutputs[1].Value = largeFilePayment
	err = h.considerContract(txn, renterKey, largeFilesize, merkleRoot)
	if err != ErrHostCapacity {
		t.Fatal(err)
	}

	// Reset the file contract to a working contract, and create an obligation
	// from the transaction.
	txn.FileContracts[0].FileSize = filesize
	txn.FileContracts[0].ValidProofOutputs[0].Value = refund
	txn.FileContracts[0].ValidProofOutputs[1].Value = payment
	txn.FileContracts[0].MissedProofOutputs[0].Value = refund
	txn.FileContracts[0].MissedProofOutputs[1].Value = payment
	obligation := &contractObligation{
		ID:                txn.FileContractID(0),
		OriginTransaction: txn,
	}

	// Create a legal revision transaction.
	newFileSize := filesize + uint64(4e3)
	revisedPayment := payment.Add(types.NewCurrency64(newFileSize - filesize).Mul(currencyDuration).Mul(settings.Price))
	revisedRefund := types.PostTax(ht.cs.Height(), payout).Sub(revisedPayment)
	revisionTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID: txn.FileContractID(0),
			UnlockConditions: types.UnlockConditions{
				PublicKeys:         []types.SiaPublicKey{renterKey, h.publicKey},
				SignaturesRequired: 2,
			},
			NewRevisionNumber: txn.FileContracts[0].RevisionNumber + 1,

			NewFileSize:       newFileSize,
			NewFileMerkleRoot: merkleRoot,
			NewWindowStart:    windowStart,
			NewWindowEnd:      windowEnd,
			NewValidProofOutputs: []types.SiacoinOutput{
				{
					Value:      revisedRefund,
					UnlockHash: types.UnlockHash{},
				},
				{
					Value:      revisedPayment,
					UnlockHash: settings.UnlockHash,
				},
			},
			NewMissedProofOutputs: []types.SiacoinOutput{
				{
					Value:      revisedRefund,
					UnlockHash: types.UnlockHash{},
				},
				{
					Value:      revisedPayment,
					UnlockHash: types.UnlockHash{},
				},
			},
			NewUnlockHash: txn.FileContracts[0].UnlockHash,
		}},
	}
	err = ht.host.considerRevision(revisionTxn, obligation)
	if err != nil {
		t.Fatal(err)
	}

	// Test that too large revisions get rejected.
	settings.TotalStorage = 3e3
	ht.host.SetSettings(settings)
	if ht.host.spaceRemaining != 3e3 {
		t.Fatal("host is not getting the correct space remaining")
	}
	err = ht.host.considerRevision(revisionTxn, obligation)
	if err != ErrHostCapacity {
		t.Fatal(err)
	}

	// Test that file revisions get accepted if the updated file size is too
	// large but just the added data is small enough (regression test).
	settings.TotalStorage = 8e3
	ht.host.SetSettings(settings)
	err = ht.host.considerRevision(revisionTxn, obligation)
	if err != nil {
		t.Fatal(err)
	}

	// Test that underpaid revisions get rejected.
	revisedUnderPayment := payment.Add(types.NewCurrency64(newFileSize - filesize - 1e3).Mul(currencyDuration).Mul(settings.Price))
	revisedUnderRefund := types.PostTax(ht.cs.Height(), payout).Sub(revisedUnderPayment)
	revisionTxn.FileContractRevisions[0].NewValidProofOutputs[0].Value = revisedUnderRefund
	revisionTxn.FileContractRevisions[0].NewValidProofOutputs[1].Value = revisedUnderPayment
	revisionTxn.FileContractRevisions[0].NewMissedProofOutputs[0].Value = revisedUnderRefund
	revisionTxn.FileContractRevisions[0].NewMissedProofOutputs[1].Value = revisedUnderPayment
	revisionTxn.FileContractRevisions[0].NewMissedProofOutputs[1].Value = revisedUnderPayment
	revisionTxn.FileContractRevisions[0].NewMissedProofOutputs[1].Value = revisedUnderPayment
	err = ht.host.considerRevision(revisionTxn, obligation)
	if err != ErrLowPayment {
		t.Fatal(err)
	}
}
