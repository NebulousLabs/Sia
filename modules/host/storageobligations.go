package host

// TODO: Need to set up the multi-drive thing.

// TODO: File contracts actually cannot be mutable to add fees - makes the file
// contract id mutable too. BUT, revisions can be mutable.

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
	// ErrDuplicateStorageObligation is returned when the storage obligation
	// database already has a storage obligation with the provided file
	// contract. This error should only happen in the event of a developer
	// mistake.
	ErrDuplicateStorageObligation = errors.New("storage obligation has a file contract which conflicts with an existing storage obligation")

	// ErrInsaneFileContractOutputCounts is returned when a file contract has
	// the wrong number of outputs for either the valid or missed payouts.
	ErrInsaneFileContractOutputCounts = errors.New("file contract has incorrect number of outputs for the valid or missed payouts")

	// ErrInsaneFileContractRevisionOutputCounts is returned when a file
	// contract has the wrong number of outputs for either the valid or missed
	// payouts.
	ErrInsaneFileContractRevisionOutputCounts = errors.New("file contract revision has incorrect number of outputs for the valid or missed payouts")

	// ErrInsaneOriginSetFileContract is returned is the final transaction of
	// the origin transaction set of a storage obligation does not have a file
	// contract in the final transaction - there should be a file contract
	// associated with every storage obligation.
	ErrInsaneOriginSetFileContract = errors.New("origin transaction set of storage obligation should have one file contract in the final transaction")

	// ErrInsaneOriginSetSize is returned if the origin transaction set of a
	// storage obligation is empty - there should be a file contract associated
	// with every storage obligation.
	ErrInsaneOriginSetSize = errors.New("origin transaction set of storage obligation is size zero")

	// ErrInsaneRevisionSetRevisionCount is returned if the final transaction
	// in the revision transaction set of a storage obligation has more or less
	// than one file contract revision.
	ErrInsaneRevisionSetRevisionCount = errors.New("revision transaction set of storage obligation should have one file contract revision in the final transaction")
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

// isSane checks that required assumptions about the storage obligation are
// correct.
func (so *storageObligation) isSane() error {
	// There should be an origin transaction set.
	if len(so.OriginTransactionSet) == 0 {
		build.Critical("origin transaction set is empty")
		return ErrInsaneOriginSetSize
	}

	// The final transaction of the origin transaction set should have one file
	// contract.
	final := len(so.OriginTransactionSet) - 1
	fcCount := len(so.OriginTransactionSet[final].FileContracts)
	if fcCount != 1 {
		build.Critical("wrong number of file contracts associated with storage obligation:", fcCount)
		return ErrInsaneOriginSetFileContract
	}

	// The file contract in the final transaction of the origin transaction set
	// should have two valid proof outputs and two missed proof outputs.
	lenVPOs := len(so.OriginTransactionSet[final].FileContracts[0].ValidProofOutputs)
	lenMPOs := len(so.OriginTransactionSet[final].FileContracts[0].MissedProofOutputs)
	if lenVPOs != 2 || lenMPOs != 2 {
		build.Critical("file contract has wrong number of VPOs and MPOs, expecting 2 each:", lenVPOs, lenMPOs)
		return ErrInsaneFileContractOutputCounts
	}

	// If there is a revision transaction set, there should be one file
	// contract revision in the final transaction.
	if len(so.RevisionTransactionSet) > 0 {
		final = len(so.OriginTransactionSet) - 1
		fcrCount := len(so.OriginTransactionSet[final].FileContractRevisions)
		if fcrCount != 1 {
			build.Critical("wrong number of file contract revisions in final transaction of revision transaction set:", fcrCount)
			return ErrInsaneRevisionSetRevisionCount
		}

		// The file contract revision in the final transaction of the revision
		// transaction set should have two valid proof outputs and two missed
		// proof outputs.
		lenVPOs = len(so.RevisionTransactionSet[final].FileContractRevisions[0].NewValidProofOutputs)
		lenMPOs = len(so.RevisionTransactionSet[final].FileContractRevisions[0].NewMissedProofOutputs)
		if lenVPOs != 2 || lenMPOs != 2 {
			build.Critical("file contract has wrong number of VPOs and MPOs, expecting 2 each:", lenVPOs, lenMPOs)
			return ErrInsaneFileContractRevisionOutputCounts
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
		bai := tx.Bucket(BucketActionItems)
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

// addStorageObligation adds a storage obligation to the host. There is an
// assumption that the file contract transaction has not yet made it onto the
// blockchain.
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

	// Add the storage obligation information to the database.
	err := h.db.Update(func(tx *bolt.Tx) error {
		// Sanity check - a storage obligation using the same file contract id
		// should not already exist. This situation can happen if the
		// transaction pool ejects a file contract and then a new one is
		// created. Though the file contract will have the same terms, some
		// other conditions might cause problems. The check for duplicate file
		// contract ids should happen during the negotiation phase, and not
		// during the 'addStorageObligation' phase.
		bso := tx.Bucket(BucketStorageObligations)
		soBytes := bso.Get(soid[:])
		if soBytes != nil {
			h.log.Critical("host already has a save storage obligation for this file contract")
			return ErrDuplicateStorageObligation
		}

		// Add the storage obligation to the database.
		soBytes, err := json.Marshal(*so)
		if err != nil {
			return err
		}
		err = bso.Put(soid[:], soBytes)
		if err != nil {
			return err
		}

		// Expensive santiy check - all of the sectors in the obligation should
		// already be represented in the sector usage bucket.
		if build.DEBUG {
			bsu := tx.Bucket(BucketSectorUsage)
			for _, root := range so.SectorRoots {
				if bsu.Get(root[:]) == nil {
					h.log.Critical("sector root information has not been correctly updated")
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Update the host statistics to reflect the new storage obligation.
	h.anticipatedRevenue = h.anticipatedRevenue.Add(so.AnticipatedRevenue)

	// Set an action item that will have the host verify that the file contract
	// has been submitted to the blockchain.
	err = h.queueActionItem(h.blockHeight+resubmissionTimeout, soid)
	if err != nil {
		return err
	}
	return nil
}
