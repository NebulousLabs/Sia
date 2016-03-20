package host

// storageobligations_smoke_test.go performs smoke testing on the the storage
// obligation management. This includes adding valid storage obligations, and
// waiting until they expire, to see if the failure modes are all handled
// correctly.

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// randSector creates a random sector, returning the sector along with the
// Merkle root of the sector.
func randSector() (crypto.Hash, []byte, error) {
	sectorData, err := crypto.RandBytes(int(modules.SectorSize))
	if err != nil {
		return crypto.Hash{}, nil, err
	}
	sectorRoot := crypto.MerkleRoot(sectorData)
	return sectorRoot, sectorData, nil
}

// newTesterStorageObligation uses the wallet to create and fund a file
// contract that will form the foundation of a storage obligation.
func (ht *hostTester) newTesterStorageObligation() (*storageObligation, error) {
	// Create the file contract that will be used in the obligation.
	builder := ht.wallet.StartTransaction()
	// Fund the file contract with a payout. The payout needs to be big enough
	// that the expected revenue is larger than the fee that the host may end
	// up paying.
	payout := types.NewCurrency64(1e3).Mul(types.SiacoinPrecision)
	err := builder.FundSiacoins(payout)
	if err != nil {
		return nil, err
	}
	// Add the file contract that consumes the funds.
	_ = builder.AddFileContract(types.FileContract{
		// Because this file contract needs to be able to accept file contract
		// revisions, the expiration is put more than
		// 'revisionSubmissionBuffer' blocks into the future.
		WindowStart: ht.host.blockHeight + revisionSubmissionBuffer + 2,
		WindowEnd:   ht.host.blockHeight + revisionSubmissionBuffer + defaultWindowSize + 2,

		Payout: payout,
		ValidProofOutputs: []types.SiacoinOutput{
			{
				Value: types.PostTax(ht.host.blockHeight, payout),
			},
			{
				Value: types.ZeroCurrency,
			},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			{
				Value: types.PostTax(ht.host.blockHeight, payout),
			},
			{
				Value: types.ZeroCurrency,
			},
		},
		UnlockHash:     (types.UnlockConditions{}).UnlockHash(),
		RevisionNumber: 0,
	})
	// Sign the transaction.
	tSet, err := builder.Sign(true)
	if err != nil {
		return nil, err
	}

	// Assemble and return the storage obligation.
	so := &storageObligation{
		OriginTransactionSet: tSet,

		// TODO: There are no tracking values, because no fees were added.
	}
	return so, nil
}

// TestBlankStorageObligation checks that the host correctly manages a blank
// storage obligation.
func TestBlankStorageObligation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestBlankStorageObligation")
	if err != nil {
		t.Fatal(err)
	}

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.addStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Mine a block to confirm the transaction containing the storage
	// obligation.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}

	// Mine until the host would be submitting a storage proof. Check that the
	// host has cleared out the storage proof - the consensus code makes it
	// impossible to submit a storage proof for an empty file contract, so the
	// host should fail and give up by deleting the storage obligation.
	for i := types.BlockHeight(0); i <= revisionSubmissionBuffer*2+1; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != errNoStorageObligation {
		t.Fatal(err)
	}
}

// TestSingleSectorObligationStack checks that the host correctly manages a
// storage obligation with a single sector, the revision is created the same
// block as the file contract.
func TestSingleSectorStorageObligationStack(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestSingleSectorStorageObligationStack")
	if err != nil {
		t.Fatal(err)
	}

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.addStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData, err := randSector()
	if err != nil {
		t.Fatal(err)
	}
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.NewCurrency64(550).Mul(types.SiacoinPrecision)
	so.AnticipatedRevenue = so.AnticipatedRevenue.Add(sectorCost)
	ht.host.potentialStorageRevenue = ht.host.potentialStorageRevenue.Add(sectorCost)
	validPayouts, missedPayouts := so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost)
	revisionSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 1,

			NewFileSize:           uint64(len(sectorData)),
			NewFileMerkleRoot:     sectorRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot}, [][]byte{sectorData})
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Submit the revision set to the transaction pool.
	err = ht.tpool.AcceptTransactionSet(revisionSet)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block to confirm the transactions containing the file contract
	// and the file contract revision.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.RevisionConfirmed {
		t.Fatal("revision transaction for storage obligation was not confirmed after a block was mined")
	}

	// Mine until the host submits a storage proof.
	for i := types.BlockHeight(0); i <= revisionSubmissionBuffer+2+resubmissionTimeout; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.RevisionConfirmed {
		t.Fatal("revision transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.ProofConfirmed {
		t.Fatal("storage obligation is not saying that the storage proof was confirmed on the blockchain")
	}

	// Mine blocks until the storage proof has enough confirmations that the
	// host will delete the file entirely.
	for i := 0; i <= storageProofConfirmations; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != errNoStorageObligation {
		t.Fatal(err)
	}
	if ht.host.storageRevenue.Cmp(sectorCost) != 0 {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}

// TestMultiSectorObligationStack checks that the host correctly manages a
// storage obligation with a single sector, the revision is created the same
// block as the file contract.
//
// Unlike the SingleSector test, the multi sector test attempts to spread file
// contract revisions over multiple blocks.
func TestMultiSectorStorageObligationStack(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestMultiSectorStorageObligationStack")
	if err != nil {
		t.Fatal(err)
	}

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.addStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}
	// Deviation from SingleSector test - mine a block here to confirm the
	// storage obligation before a file contract revision is created.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData, err := randSector()
	if err != nil {
		t.Fatal(err)
	}
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.NewCurrency64(550).Mul(types.SiacoinPrecision)
	so.AnticipatedRevenue = so.AnticipatedRevenue.Add(sectorCost)
	ht.host.potentialStorageRevenue = ht.host.potentialStorageRevenue.Add(sectorCost)
	validPayouts, missedPayouts := so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost)
	revisionSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 1,

			NewFileSize:           uint64(len(sectorData)),
			NewFileMerkleRoot:     sectorRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot}, [][]byte{sectorData})
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Submit the revision set to the transaction pool.
	err = ht.tpool.AcceptTransactionSet(revisionSet)
	if err != nil {
		t.Fatal(err)
	}

	// Create a second file contract revision, which is going to be submitted
	// to the transaction pool after the first revision. Though, in practice
	// this should never happen, we want to check that the transaction pool is
	// correctly handling multiple file contract revisions being submitted in
	// the same block cycle. This test will additionally tell us whether or not
	// the host can correctly handle buildling storage proofs for files with
	// multiple sectors.
	sectorRoot2, sectorData2, err := randSector()
	if err != nil {
		t.Fatal(err)
	}
	so.SectorRoots = []crypto.Hash{sectorRoot, sectorRoot2}
	sectorCost2 := types.NewCurrency64(650).Mul(types.SiacoinPrecision)
	so.AnticipatedRevenue = so.AnticipatedRevenue.Add(sectorCost2)
	ht.host.potentialStorageRevenue = ht.host.potentialStorageRevenue.Add(sectorCost2)
	validPayouts, missedPayouts = so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost2)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost2)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost2)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost2)
	combinedSectors := append(sectorData, sectorData2...)
	combinedRoot := crypto.MerkleRoot(combinedSectors)
	revisionSet2 := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 2,

			NewFileSize:           uint64(len(sectorData) + len(sectorData2)),
			NewFileMerkleRoot:     combinedRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot2}, [][]byte{sectorData2})
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Submit the revision set to the transaction pool.
	err = ht.tpool.AcceptTransactionSet(revisionSet2)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block to confirm the transactions containing the file contract
	// and the file contract revision.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.RevisionConfirmed {
		t.Fatal("revision transaction for storage obligation was not confirmed after a block was mined")
	}

	// Mine until the host submits a storage proof.
	for i := types.BlockHeight(0); i <= revisionSubmissionBuffer+1+resubmissionTimeout; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.ProofConfirmed {
		t.Fatal("storage obligation is not saying that the storage proof was confirmed on the blockchain")
	}

	// Mine blocks until the storage proof has enough confirmations that the
	// host will delete the file entirely.
	for i := 0; i <= storageProofConfirmations; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != errNoStorageObligation {
		t.Fatal(err)
	}
	if ht.host.storageRevenue.Cmp(sectorCost.Add(sectorCost2)) != 0 {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}

// TestAutoRevisionSubmission checks that the host correctly submits a file
// contract revision to the consensus set.
func TestAutoRevisionSubmission(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestAutoRevisionSubmission")
	if err != nil {
		t.Fatal(err)
	}

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.addStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData, err := randSector()
	if err != nil {
		t.Fatal(err)
	}
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.NewCurrency64(550).Mul(types.SiacoinPrecision)
	so.AnticipatedRevenue = so.AnticipatedRevenue.Add(sectorCost)
	ht.host.potentialStorageRevenue = ht.host.potentialStorageRevenue.Add(sectorCost)
	validPayouts, missedPayouts := so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost)
	revisionSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 1,

			NewFileSize:           uint64(len(sectorData)),
			NewFileMerkleRoot:     sectorRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	so.RevisionTransactionSet = revisionSet
	err = ht.host.lockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot}, [][]byte{sectorData})
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.unlockStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	// Unlike the other tests, this test does not submit the file contract
	// revision to the transaction pool for the host, the host is expected to
	// do it automatically.

	// Mine until the host submits a storage proof.
	for i := types.BlockHeight(0); i <= revisionSubmissionBuffer+2+resubmissionTimeout; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after blocks were mined")
	}
	if !so.RevisionConfirmed {
		t.Fatal("revision transaction for storage obligation was not confirmed after blocks were mined")
	}
	if !so.ProofConfirmed {
		t.Fatal("storage obligation is not saying that the storage proof was confirmed on the blockchain")
	}

	// Mine blocks until the storage proof has enough confirmations that the
	// host will delete the file entirely.
	for i := 0; i <= storageProofConfirmations; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		*so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != errNoStorageObligation {
		t.Error(so.OriginConfirmed)
		t.Error(so.RevisionConfirmed)
		t.Error(so.ProofConfirmed)
		t.Fatal(err)
	}
	if ht.host.storageRevenue.Cmp(sectorCost) != 0 {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}
