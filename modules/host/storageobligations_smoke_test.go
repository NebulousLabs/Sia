package host

// storageobligations_smoke_test.go performs smoke testing on the the storage
// obligation management. This includes adding valid storage obligations, and
// waiting until they expire, to see if the failure modes are all handled
// correctly.

import (
	"errors"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"

	"github.com/coreos/bbolt"
)

// randSector creates a random sector, returning the sector along with the
// Merkle root of the sector.
func randSector() (crypto.Hash, []byte) {
	sectorData := fastrand.Bytes(int(modules.SectorSize))
	sectorRoot := crypto.MerkleRoot(sectorData)
	return sectorRoot, sectorData
}

// newTesterStorageObligation uses the wallet to create and fund a file
// contract that will form the foundation of a storage obligation.
func (ht *hostTester) newTesterStorageObligation() (storageObligation, error) {
	// Create the file contract that will be used in the obligation.
	builder, err := ht.wallet.StartTransaction()
	if err != nil {
		return storageObligation{}, err
	}
	// Fund the file contract with a payout. The payout needs to be big enough
	// that the expected revenue is larger than the fee that the host may end
	// up paying.
	payout := types.SiacoinPrecision.Mul64(1e3)
	err = builder.FundSiacoins(payout)
	if err != nil {
		return storageObligation{}, err
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
		return storageObligation{}, err
	}

	// Assemble and return the storage obligation.
	so := storageObligation{
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
	defer ht.Close()

	// The number of contracts reported by the host should be zero.
	fm := ht.host.FinancialMetrics()
	if fm.ContractCount != 0 {
		t.Error("host does not start with 0 contracts:", fm.ContractCount)
	}

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}
	fm = ht.host.FinancialMetrics()
	if fm.ContractCount != 1 {
		t.Error("host should have 1 contract:", fm.ContractCount)
	}

	// Mine a block to confirm the transaction containing the storage
	// obligation.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, so.id())
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
		err = ht.host.tg.Flush()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fm = ht.host.FinancialMetrics()
	if fm.ContractCount != 0 {
		t.Error("host should have 0 contracts, the contracts were all completed:", fm.ContractCount)
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
	defer ht.Close()

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.SiacoinPrecision.Mul64(550)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost)
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost)
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
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot}, [][]byte{sectorData})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
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
		so, err = getStorageObligation(tx, so.id())
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
	for i := ht.host.blockHeight; i <= so.expiration()+resubmissionTimeout; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Flush the host - flush will block until the host has submitted the
	// storage proof to the transaction pool.
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}
	// Mine another block, to get the storage proof from the transaction pool
	// into the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Grab the storage proof and inspect the contents.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, so.id())
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
	// host will finalize the obligation.
	for i := 0; i <= int(defaultWindowSize); i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		if so.SectorRoots != nil {
			t.Error("sector roots were not cleared when the host finalized the obligation")
		}
		if so.ObligationStatus != obligationSucceeded {
			t.Error("obligation is not being reported as successful:", so.ObligationStatus)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ht.host.financialMetrics.StorageRevenue.Equals(sectorCost) {
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
	defer ht.Close()

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
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
		so, err = getStorageObligation(tx, so.id())
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
	sectorRoot, sectorData := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.SiacoinPrecision.Mul64(550)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost)
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost)
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
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot}, [][]byte{sectorData})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
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
	// the host can correctly handle building storage proofs for files with
	// multiple sectors.
	sectorRoot2, sectorData2 := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot, sectorRoot2}
	sectorCost2 := types.SiacoinPrecision.Mul64(650)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost2)
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost2)
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
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot2}, [][]byte{sectorData2})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
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
		so, err = getStorageObligation(tx, so.id())
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
	for i := ht.host.blockHeight; i <= so.expiration()+resubmissionTimeout; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Flush the host - flush will block until the host has submitted the
	// storage proof to the transaction pool.
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}
	// Mine another block, to get the storage proof from the transaction pool
	// into the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, so.id())
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
	for i := 0; i <= int(defaultWindowSize); i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		if so.SectorRoots != nil {
			t.Error("sector roots were not cleared out when the storage proof was finalized")
		}
		if so.ObligationStatus != obligationSucceeded {
			t.Error("storage obligation was not reported as a success")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ht.host.financialMetrics.StorageRevenue.Equals(sectorCost.Add(sectorCost2)) {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}

// TestAutoRevisionSubmission checks that the host correctly submits a file
// contract revision to the consensus set.
func TestAutoRevisionSubmission(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestAutoRevisionSubmission")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.SiacoinPrecision.Mul64(550)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost)
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost)
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
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.modifyStorageObligation(so, nil, []crypto.Hash{sectorRoot}, [][]byte{sectorData})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	err = ht.host.tg.Flush()
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
		err = ht.host.tg.Flush()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Flush the host - flush will block until the host has submitted the
	// storage proof to the transaction pool.
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}
	// Mine another block, to get the storage proof from the transaction pool
	// into the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}

	err = build.Retry(50, 250*time.Millisecond, func() error {
		err = ht.host.db.View(func(tx *bolt.Tx) error {
			so, err = getStorageObligation(tx, so.id())
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return (err)
		}
		if !so.OriginConfirmed {
			return errors.New("origin transaction for storage obligation was not confirmed after blocks were mined")
		}
		if !so.RevisionConfirmed {
			return errors.New("revision transaction for storage obligation was not confirmed after blocks were mined")
		}
		if !so.ProofConfirmed {
			return errors.New("storage obligation is not saying that the storage proof was confirmed on the blockchain")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine blocks until the storage proof has enough confirmations that the
	// host will delete the file entirely.
	for i := 0; i <= int(defaultWindowSize); i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = ht.host.tg.Flush()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		if so.SectorRoots != nil {
			t.Error("sector roots were not cleared out when the storage proof was finalized")
		}
		if so.ObligationStatus != obligationSucceeded {
			t.Error("storage obligation was not reported as a success")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ht.host.financialMetrics.StorageRevenue.Equals(sectorCost) {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}
