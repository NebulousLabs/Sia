package host

// storageobligations.go is responsible for managing the storage obligations
// within the host - making sure that any file contracts, transaction
// dependencies, file contract revisions, and storage proofs are making it into
// the blockchain in a reasonable time.

import (
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
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
	AnticipatedRevenue     types.Currency
	RiskedCollateral       types.Currency
	OriginTransactionSet   []types.Transaction
	RevisionTransactionSet []types.Transaction

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
		// Sanity check - the expiration of the revision and the expiration of
		// the origin should be the same. While they do not inherently need to
		// be, not doing so complicates the code significantly and opens up
		// several attack vectors. Support for changing the expiration may be
		// added in the future.
		if expiration != originExpiration {
			build.Critical("origin file contract and most recent file contract revision do not expire at the same time")
		}
		return expiration
	}
	return originExpriation
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

// id returns the id of the storage obligation, which is definied by the file
// contract id of the file contract that governs the storage contract.
func (so *storageObligation) id() types.FileContractID {
	err := so.isSane()
	if err != nil {
		build.Critical("insane storage obligation when returning id")
		return types.FileContractID{}
	}
	return so.OriginTransactionSet[len(so.OriginTransactionSet)-1].FileContractID(0)
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
		err := bai.Put(heightBytes, existingItems)
		if err != nil {
			return err
		}

		// Expensive sanity check - there should be no duplicate file contract
		// ids in 'existingItems'.
		if build.DEBUG {
			// Sanity check takes a shortcut by knowing that all file contract
			// ids are 32 bytes, and that there is no padding or prefixes for
			// any of the data.
			var ids [][32]byte
			for i := 0; i < len(existingItems); i += 32 {
				var newID [32]byte
				copy(newID[:], existingItems[i:i+32])
				for _, id := range ids {
					if newID == id {
						h.log.Critical("host has multiple action items for a single storage obligation at one height")
					}
				}
				ids = append(ids, newID)
			}
		}
		return nil
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
	// Expensive Sanity check - obligation being added should have valid
	// tranasctions.
	if build.DEBUG {
		for _, txn := range so.OriginTransactionSet {
			err := txn.StandaloneValid(h.blockHeight)
			if err != nil {
				h.log.Critical("invalid transaction is being added in a storage obligation")
			}
		}
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
	// has been submitted to the blockchain.
	err = h.queueActionItem(h.blockHeight+resubmissionTimeout, soid)
	if err != nil {
		return err
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
		h.log.Critical("modifying a revision with garbase sector data", len(sectorsGained), len(gainedSectorData))
		return errInsaneStorageObligationModification
	}
	// Sanity check - all of the sector data should be sectorSize
	for _, data := range gainedSectorData {
		if len(data) != sectorSize {
			h.log.Critical("modifying a revision with garbase sector sizes", len(data))
			return errInsaneStorageObligationModificationData
		}
	}

	// Call removeSector for all of the sectors that have been removed.
	for _, sr := range sectorsRemoved {
		err = h.removeSector(sr, so.expiration())
		if err != nil {
			return err
		}
	}
	// Call addSector for all of the sectors that are getting added.
	for i := range sectorsGained {
		err = h.addSector(sectorsGained[i], gainedSectorData[i])
		if err != nil {
			return err
		}
	}

	// Update the database to contain the new storage obligation.
	return h.db.Update(func(tx *bolt.Tx) error {
		soBytes, err := json.Marshal(*so)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketStorageObligations).Put(soid[:], soBytes)
	})
}

// TODO: removeStorageObligation

// TODO: handleActionItem - maybe action items should be explicit, instead of
// being figured out contextually.

// Potential action items:
//  1. Submit a file contract
//  2. Submit a file contract revision
//  3. Submit a storage proof
//  4. Recognize that an obligation is finished due to a failed fc submit
//  5. Recognize that an obligation is finished due to a failed fcr submit
//  6. Recognize that an obligation is finished due to a failed sp submit
//  7. Recognize that an obligation is finished due to a successful and deeply-confirmed sp submit.
