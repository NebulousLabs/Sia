package host

// storageobligations.go is responsible for managing the storage obligations
// within the host - making sure that any file contracts, transaction
// dependencies, file contract revisions, and storage proofs are making it into
// the blockchain in a reasonable time.
//
// NOTE: Currently, the code partially supports changing the storage proof
// window in file contract revisions, however the action item code will not
// handle it correctly. Until the action item code is improved (to also handle
// byzantine situations where the renter submits prior revisions), the host
// should not support changing the storage proof window, especially to further
// in the future.

// TODO: Have the storage obligations operations update the revenue metrics in
// the host, but let negotiation decide what the revenue metrics are inside of
// the storage obligation.

import (
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
	"github.com/NebulousLabs/merkletree"
)

var (
	// errDuplicateStorageObligation is returned when the storage obligation
	// database already has a storage obligation with the provided file
	// contract. This error should only happen in the event of a developer
	// mistake.
	errDuplicateStorageObligation = errors.New("storage obligation has a file contract which conflicts with an existing storage obligation")

	// errInsaneFileContractOutputCounts is returned when a file contract has
	// the wrong number of outputs for either the valid or missed payouts.
	errInsaneFileContractOutputCounts = errors.New("file contract has incorrect number of outputs for the valid or missed payouts")

	// errInsaneFileContractRevisionOutputCounts is returned when a file
	// contract has the wrong number of outputs for either the valid or missed
	// payouts.
	errInsaneFileContractRevisionOutputCounts = errors.New("file contract revision has incorrect number of outputs for the valid or missed payouts")

	// errInsaneOriginSetFileContract is returned is the final transaction of
	// the origin transaction set of a storage obligation does not have a file
	// contract in the final transaction - there should be a file contract
	// associated with every storage obligation.
	errInsaneOriginSetFileContract = errors.New("origin transaction set of storage obligation should have one file contract in the final transaction")

	// errInsaneOriginSetSize is returned if the origin transaction set of a
	// storage obligation is empty - there should be a file contract associated
	// with every storage obligation.
	errInsaneOriginSetSize = errors.New("origin transaction set of storage obligation is size zero")

	// errInsaneRevisionSetRevisionCount is returned if the final transaction
	// in the revision transaction set of a storage obligation has more or less
	// than one file contract revision.
	errInsaneRevisionSetRevisionCount = errors.New("revision transaction set of storage obligation should have one file contract revision in the final transaction")

	// errInsaneStorageObligationRevision is returned if there is an attempted
	// storage obligation revision which does not have sensical inputs.
	errInsaneStorageObligationRevision = errors.New("revision to storage obligation does not make sense")

	// errInsaneStorageObligationRevisionData is returned if there is an
	// attempted storage obligation revision which does not have sensical
	// inputs.
	errInsaneStorageObligationRevisionData = errors.New("revision to storage obligation has insane data")

	// errNoBuffer is returned if there is an attempted storage obligation
	// revision that is acting on a storage obligation which needs to have the
	// storage proof submitted in less than revisionSubmissionBuffer blocks.
	errNoBuffer = errors.New("file contract modification rejected because storage proof window is too close")
)

const (
	obligationConfused  = iota // Indicatees that an unitialized value was used.
	obligationRejected         // Indicates that the obligation never got started, no revenue gained or lost.
	obligationSucceeded        // Indicates that the obligation was completed, revenues were gained.
	obligationFailed           // Indicates that the obligation failed, revenues and collateral were lost.
)

// storageObligation contains all of the metadata related to a file contract
// and the storage contained by the file contract.
type storageObligation struct {
	// Storage obligations are broken up into ordered atomic sectors that are
	// exactly 4MiB each. By saving the roots of each sector, storage proofs
	// and modifications to the data can be made inexpensively by making use of
	// the merkletree.CachedTree. Sectors can be appended, modified, or deleted
	// and the host can recompute the Merkle root of the whole file without
	// much computational or I/O expense.
	SectorRoots []crypto.Hash

	// Variables about the file contract that enforces the storage obligation.
	// The origin an revision transaction are stored as a set, where the set
	// contains potentially unconfirmed transactions.
	//
	// Note: as of implementation, the transaction pool does not correctly
	// handle being given transaction sets where part of the transaction set
	// has already been confirmed on the blockchain. Therefore, when trying to
	// submit a transaction set it is important to try several subsets to rule
	// out the possibility that the transaction set is partially confirmed.
	AnticipatedRevenue          types.Currency
	TransactionFeesAdded        types.Currency
	TransactionFeesPaidByRenter types.Currency
	RiskedCollateral            types.Currency
	OriginTransactionSet        []types.Transaction
	RevisionTransactionSet      []types.Transaction

	// Variables indicating whether the critical transactions in a storage
	// obligation have been confirmed on the blockchain.
	OriginConfirmed   bool
	RevisionConfirmed bool
	ProofConfirmed    bool
}

// expiration returns the height at which the storage obligation expires.
func (so *storageObligation) expiration() types.BlockHeight {
	originExpiration := so.OriginTransactionSet[len(so.OriginTransactionSet)-1].FileContracts[0].WindowStart
	if len(so.RevisionTransactionSet) > 0 {
		expiration := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1].FileContractRevisions[0].NewWindowStart
		return expiration
	}
	return originExpiration
}

// id returns the id of the storage obligation, which is definied by the file
// contract id of the file contract that governs the storage contract.
func (so *storageObligation) id() types.FileContractID {
	return so.OriginTransactionSet[len(so.OriginTransactionSet)-1].FileContractID(0)
}

// isSane checks that required assumptions about the storage obligation are
// correct.
func (so *storageObligation) isSane() error {
	// There should be an origin transaction set.
	if len(so.OriginTransactionSet) == 0 {
		build.Critical("origin transaction set is empty")
		return errInsaneOriginSetSize
	}

	// The final transaction of the origin transaction set should have one file
	// contract.
	final := len(so.OriginTransactionSet) - 1
	fcCount := len(so.OriginTransactionSet[final].FileContracts)
	if fcCount != 1 {
		build.Critical("wrong number of file contracts associated with storage obligation:", fcCount)
		return errInsaneOriginSetFileContract
	}

	// The file contract in the final transaction of the origin transaction set
	// should have two valid proof outputs and two missed proof outputs.
	lenVPOs := len(so.OriginTransactionSet[final].FileContracts[0].ValidProofOutputs)
	lenMPOs := len(so.OriginTransactionSet[final].FileContracts[0].MissedProofOutputs)
	if lenVPOs != 2 || lenMPOs != 2 {
		build.Critical("file contract has wrong number of VPOs and MPOs, expecting 2 each:", lenVPOs, lenMPOs)
		return errInsaneFileContractOutputCounts
	}

	// If there is a revision transaction set, there should be one file
	// contract revision in the final transaction.
	if len(so.RevisionTransactionSet) > 0 {
		final = len(so.OriginTransactionSet) - 1
		fcrCount := len(so.OriginTransactionSet[final].FileContractRevisions)
		if fcrCount != 1 {
			build.Critical("wrong number of file contract revisions in final transaction of revision transaction set:", fcrCount)
			return errInsaneRevisionSetRevisionCount
		}

		// The file contract revision in the final transaction of the revision
		// transaction set should have two valid proof outputs and two missed
		// proof outputs.
		lenVPOs = len(so.RevisionTransactionSet[final].FileContractRevisions[0].NewValidProofOutputs)
		lenMPOs = len(so.RevisionTransactionSet[final].FileContractRevisions[0].NewMissedProofOutputs)
		if lenVPOs != 2 || lenMPOs != 2 {
			build.Critical("file contract has wrong number of VPOs and MPOs, expecting 2 each:", lenVPOs, lenMPOs)
			return errInsaneFileContractRevisionOutputCounts
		}
	}
	return nil
}

// proofDeadline returns the height by which the storage proof must be
// submitted.
func (so *storageObligation) proofDeadline() types.BlockHeight {
	originDeadline := so.OriginTransactionSet[len(so.OriginTransactionSet)-1].FileContracts[0].WindowEnd
	if len(so.RevisionTransactionSet) > 0 {
		return so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1].FileContractRevisions[0].NewWindowEnd
	}
	return originDeadline
}

// queueActionItem adds an action item to the host at the input height so that
// the host knows to perform maintenance on the associated storage obligation
// when that height is reached.
func (h *Host) queueActionItem(height types.BlockHeight, id types.FileContractID) error {
	return h.db.Update(func(tx *bolt.Tx) error {
		// Translate the height into a byte slice.
		heightBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(heightBytes, uint64(height))

		// Get the list of action items already at this height and extend it.
		bai := tx.Bucket(bucketActionItems)
		existingItems := bai.Get(heightBytes)
		existingItems = append(existingItems, id[:]...)
		return bai.Put(heightBytes, existingItems)
	})
}

// addStorageObligation adds a storage obligation to the host. Because this
// operation can return errors, the transactions should not be submitted to the
// blockchain until after this function has indicated success. All of the
// sectors that are present in the storage obligation should already be on
// disk, which means that addStorageObligation should be exclusively called
// when creating a new, empty file contract or when renewing an existing file
// contract.
func (h *Host) addStorageObligation(so *storageObligation) error {
	// Sanity check - 'addObligation' should not be adding an obligation that
	// has a revision.
	if len(so.RevisionTransactionSet) != 0 {
		h.log.Critical("addStroageObligation called with an obligation that has a revision")
	}
	// Sanity check - obligation should be under lock while being added.
	soid := so.id()
	_, exists := h.lockedStorageObligations[soid]
	if !exists {
		h.log.Critical("addStorageObligation called with an obligation that is not locked")
	}

	// Add the storage obligation information to the database. Different code
	// is used from 'addSector' because every single sector that's being added
	// is supposed to be a virtual sector, there should be an error if it is
	// not a virtual sector.
	err := h.db.Update(func(tx *bolt.Tx) error {
		// Sanity check - a storage obligation using the same file contract id
		// should not already exist. This situation can happen if the
		// transaction pool ejects a file contract and then a new one is
		// created. Though the file contract will have the same terms, some
		// other conditions might cause problems. The check for duplicate file
		// contract ids should happen during the negotiation phase, and not
		// during the 'addStorageObligation' phase.
		bso := tx.Bucket(bucketStorageObligations)
		soBytes := bso.Get(soid[:])
		if soBytes != nil {
			h.log.Critical("host already has a save storage obligation for this file contract")
			return errDuplicateStorageObligation
		}

		// Update all of the sectors in the database so that the new virtual
		// sector is added for this obligation.
		bsu := tx.Bucket(bucketSectorUsage)
		for _, root := range so.SectorRoots {
			var usage sectorUsage
			sectorUsageBytes := bsu.Get(h.sectorID(root[:]))
			if sectorUsageBytes == nil {
				h.log.Critical("host tried to add an obligation with a missing sector")
				return errSectorNotFound
			}
			err := json.Unmarshal(sectorUsageBytes, &usage)
			if err != nil {
				return err
			}
			if usage.Corrupted {
				h.log.Critical("host tried to add an obligation when one of the sectors is corrupted")
				return errSectorNotFound
			}
			usage.Expiry = append(usage.Expiry, so.expiration())
			usageBytes, err := json.Marshal(usage)
			if err != nil {
				return err
			}
			err = bsu.Put(h.sectorID(root[:]), usageBytes)
			if err != nil {
				return err
			}
		}

		// Add the storage obligation to the database.
		soBytes, err := json.Marshal(*so)
		if err != nil {
			return err
		}
		return bso.Put(soid[:], soBytes)
	})
	if err != nil {
		return err
	}

	// Set an action item that will have the host verify that the file contract
	// has been submitted to the blockchain, then another to submit the file
	// contract revision to the blockchain, and another to submit the storage
	// proof.
	err0 := h.tpool.AcceptTransactionSet(so.OriginTransactionSet)
	err1 := h.queueActionItem(h.blockHeight+resubmissionTimeout, soid)
	err2 := h.queueActionItem(so.expiration()-revisionSubmissionBuffer, soid)
	err3 := h.queueActionItem(so.expiration()+resubmissionTimeout, soid)
	err = composeErrors(err0, err1, err2, err3)
	if err != nil {
		return composeErrors(err, h.removeStorageObligation(so))
	}
	return nil
}

// modifyStorageObligation will take an updated storage obligation along with a
// list of sector changes and update the database to account for all of it.
func (h *Host) modifyStorageObligation(so *storageObligation, sectorsRemoved []crypto.Hash, sectorsGained []crypto.Hash, gainedSectorData [][]byte) error {
	// Sanity check - obligation should be under lock while being modified.
	soid := so.id()
	_, exists := h.lockedStorageObligations[soid]
	if !exists {
		h.log.Critical("modifyStorageObligation called with an obligation that is not locked")
	}
	// Sanity check - the height of the revision should be less than the
	// expiration minus the submission buffer.
	if so.expiration()-revisionSubmissionBuffer <= h.blockHeight {
		h.log.Critical("revision submission window was not verified before trying to modify a storage obligation")
		return errNoBuffer
	}
	// Sanity check - sectorsGained and gainedSectorData need to have the same length.
	if len(sectorsGained) != len(gainedSectorData) {
		h.log.Critical("modifying a revision with garbage sector data", len(sectorsGained), len(gainedSectorData))
		return errInsaneStorageObligationRevision
	}
	// Sanity check - all of the sector data should be sectorSize
	for _, data := range gainedSectorData {
		if uint64(len(data)) != sectorSize {
			h.log.Critical("modifying a revision with garbase sector sizes", len(data))
			return errInsaneStorageObligationRevision
		}
	}

	// Note, for safe error handling, the operation order should be: add
	// sectors, update database, remove sectors. If the adding or update fails,
	// the added sectors should be removed and the storage obligation shoud be
	// considered invalid. If the removing fails, this is okay, it's ignored
	// and left to consistency checks and user actions to fix (will reduce host
	// capacity, but will not inhibit the host's ability to submit storage
	// proofs)
	var i int
	var err error
	for i = range sectorsGained {
		err = h.addSector(sectorsGained[i], so.expiration(), gainedSectorData[i])
		if err != nil {
			break
		}
	}
	if err != nil {
		// Because there was an error, all of the sectors that got added need
		// to be revered.
		for j := 0; j < i; j++ {
			// Error is not checkeed because there's nothing useful that can be
			// done about an error.
			_ = h.removeSector(sectorsGained[j], so.expiration())
		}
		return err
	}
	// Update the database to contain the new storage obligation.
	err = h.db.Update(func(tx *bolt.Tx) error {
		soBytes, err := json.Marshal(*so)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketStorageObligations).Put(soid[:], soBytes)
	})
	if err != nil {
		// Because there was an error, all of the sectors that got added need
		// to be revered.
		for i := range sectorsGained {
			// Error is not checkeed because there's nothing useful that can be
			// done about an error.
			_ = h.removeSector(sectorsGained[i], so.expiration())
		}
		return err
	}
	// Call removeSector for all of the sectors that have been removed.
	for k := range sectorsRemoved {
		// Error is not checkeed because there's nothing useful that can be
		// done about an error. Failing to remove a sector is not a terrible
		// place to be, especially if the host can run consistency checks.
		_ = h.removeSector(sectorsRemoved[k], so.expiration())
	}
	return nil
}

// removeStorageObligation will remove a storage obligation from the host,
// either due to failure or success.
func (h *Host) removeStorageObligation(so *storageObligation) error {
	// Call removeSector for every sector in the storage obligation.
	for _, root := range so.SectorRoots {
		// Error is not checked, we want to call remove on every sector even if
		// there are problems - disk health information will be updated.
		_ = h.removeSector(root, so.expiration())
	}

	// Delete the storage obligation from the database.
	return h.db.Update(func(tx *bolt.Tx) error {
		soid := so.id()
		return tx.Bucket(bucketStorageObligations).Delete(soid[:])
	})
}

// handleActionItem will look at a storage obligation and determine which
// action is necessary for the storage obligation to succeed.
func (h *Host) handleActionItem(so *storageObligation) {
	// Check whether the file contract has been seen. If not, resubmit and
	// queue another action item. Check for death. (signature should have a
	// kill height)
	if !so.OriginConfirmed {
		// Submit the transaction set again, try to get the transaction
		// confirmed.
		err := h.tpool.AcceptTransactionSet(so.OriginTransactionSet)
		if err != nil {
			h.log.Println(err)
		}
		// Check if the transaction is invalid with the current consensus set.
		// If so, the transaction is highly unlikely to ever be confirmed, and
		// the storage obligation should be removed. This check should come
		// after logging the errror so that the function can quit.
		_, t := err.(modules.ConsensusConflict)
		if t {
			err = h.removeStorageObligation(so)
			if err != nil {
				h.log.Println(err)
			}
			return
		}

		// Queue another action item to check the status of the transaction.
		err = h.queueActionItem(h.blockHeight+resubmissionTimeout, so.id())
		if err != nil {
			h.log.Println(err)
		}
		return
	}

	// Check if the file contract revision is ready for submission. Check for death.
	if !so.RevisionConfirmed && so.expiration() < h.blockHeight-revisionSubmissionBuffer {
		// Sanity check - there should be a file contract revision.
		rtsLen := len(so.RevisionTransactionSet)
		if rtsLen < 1 || len(so.RevisionTransactionSet[rtsLen-1].FileContractRevisions) != 1 {
			h.log.Critical("transaction revision marked as unconfirmed, yet there is no transaction revision")
			return
		}

		// Check if the revision has failed to submit correctly.
		if so.expiration() > h.blockHeight {
			// TODO: Check this error.
			h.removeStorageObligation(so)
			return
		}

		// Queue another action item to check the status of the transaction.
		err := h.queueActionItem(h.blockHeight+resubmissionTimeout, so.id())
		if err != nil {
			h.log.Println(err)
		}

		// Add a miner fee to the transaction and submit it to the blockchain.
		revisionTxnIndex := len(so.RevisionTransactionSet) - 1
		revisionParents := so.RevisionTransactionSet[:revisionTxnIndex]
		revisionTxn := so.RevisionTransactionSet[revisionTxnIndex]
		builder := h.wallet.RegisterTransaction(revisionTxn, revisionParents)
		feeRecommendation, _ := h.tpool.FeeEstimation()
		if so.AnticipatedRevenue.Mul(types.NewCurrency64(2)).Cmp(feeRecommendation) < 0 {
			// There's no sense submitting the revision if the fee is more than
			// half of the anticipated revenue - fee market went up
			// unexpectedly, and the money that the renter paid to cover the
			// fees is no longer enough.
			return
		}
		txnSize := uint64(len(encoding.MarshalAll(so.RevisionTransactionSet)) + 300)
		requiredFee := feeRecommendation.Mul(types.NewCurrency64(txnSize))
		err = builder.FundSiacoins(requiredFee)
		if err != nil {
			h.log.Println(err)
		}
		builder.AddMinerFee(requiredFee)
		if err != nil {
			h.log.Println(err)
		}
		feeAddedRevisionTransactionSet, err := builder.Sign(true)
		if err != nil {
			h.log.Println(err)
		}
		err = h.tpool.AcceptTransactionSet(feeAddedRevisionTransactionSet)
		if err != nil {
			h.log.Println(err)
		}
		so.TransactionFeesAdded = so.TransactionFeesAdded.Add(requiredFee)
		return
	}

	// Check whether a storage proof is ready to be provided, and whether it
	// has been accepted. Check for death.
	if !so.ProofConfirmed && so.expiration()+resubmissionTimeout < h.blockHeight {
		// If the window has closed, the host has failed and the obligation can
		// be removed.
		if so.proofDeadline() < h.blockHeight {
			err := h.removeStorageObligation(so)
			if err != nil {
				h.log.Println(err)
			}
			return
		}

		// Get the segment index that should be used to build the storage
		// proof.
		segmentIndex, err := h.cs.StorageProofSegment(so.id())
		if err != nil {
			return
		}
		// Get the segment within the sector that should be used for the
		// storage proof.
		// {
		// Pull the full sector into memory to build a storage proof on that
		// sector.
		sectorIndex := segmentIndex / (sectorSize / crypto.SegmentSize)
		// Sanity check - sectorIndex should be less than the len of the sector
		// roots in the storage obligation.
		if sectorIndex >= uint64(len(so.SectorRoots)) {
			build.Critical("trying to prove storage on a sector that doesn't seem to exist")
			return
		}
		sectorRoot := so.SectorRoots[sectorIndex]
		sectorBytes, err := h.readSector(sectorRoot)
		if err != nil {
			return
		}
		// Build the storage proof for just the sector.
		sectorSegment := (sectorSize / crypto.SegmentSize) % segmentIndex
		sourceProof, err := crypto.BuildMerkleProof(sectorBytes, sectorSegment)
		if err != nil {
			return
		}
		// Using the sector, build a cached root.
		log2SectorSize := uint64(0)
		for 1<<log2SectorSize < sectorSize {
			log2SectorSize++
		}
		ct := merkletree.NewCachedTree(crypto.NewHash(), log2SectorSize)
		ct.SetIndex(segmentIndex)
		for _, root := range so.SectorRoots {
			ct.Push(root[:])
		}
		_, proofSet, _, _ := ct.Prove(sourceProof)
		// convert proofSet to base and hashSet
		base := proofSet[0]
		hashSet := make([]crypto.Hash, len(proofSet)-1)
		for i, proof := range proofSet[1:] {
			copy(hashSet[i][:], proof)
		}
		sp := types.StorageProof{
			ParentID: so.id(),
			HashSet:  hashSet,
		}
		copy(sp.Segment[:], base)

		// Create and build the transaction with the storage proof.
		builder := h.wallet.StartTransaction()
		feeRecommendation, _ := h.tpool.FeeEstimation()
		if so.AnticipatedRevenue.Cmp(feeRecommendation) < 0 {
			// There's no sense submitting the storage proof of the fee is more
			// than the anticipated revenue.
			return
		}
		txnSize := uint64(len(encoding.Marshal(sp)) + 300)
		requiredFee := feeRecommendation.Mul(types.NewCurrency64(txnSize))
		err = builder.FundSiacoins(requiredFee)
		if err != nil {
			return
		}
		builder.AddMinerFee(requiredFee)
		if err != nil {
			return
		}
		storageProofSet, err := builder.Sign(true)
		if err != nil {
			return
		}
		err = h.tpool.AcceptTransactionSet(storageProofSet)
		if err != nil {
			return
		}
		so.TransactionFeesAdded = so.TransactionFeesAdded.Add(requiredFee)

		// Queue another action item to check whether there the storage proof
		// got confirmed.
		err = h.queueActionItem(h.blockHeight+resubmissionTimeout, so.id())
		if err != nil {
			h.log.Println(err)
		}
		return
	}

	// Save the storage obligation to account for any fee changes.
	err := h.db.Update(func(tx *bolt.Tx) error {
		soBytes, err := json.Marshal(*so)
		if err != nil {
			return err
		}
		soid := so.id()
		return tx.Bucket(bucketStorageObligations).Put(soid[:], soBytes)
	})
	if err != nil {
		h.log.Println(err)
	}

	// Check if all items have succeded with the required confirmations. Report
	// success, delete the obligation.
	if so.ProofConfirmed && so.expiration()+types.BlockHeight(storageProofConfirmations) < h.blockHeight {
		h.removeStorageObligation(so)
		return
	}
}
