package transactionpool

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// accept.go is responsible for applying a transaction to the transaction pool.
// Validation is handled by valid.go. The componenets of the transcation are
// added to the unconfirmed consensus set piecemeal, and then the transaction
// itself is appended to the linked list of transactions, such that any
// dependecies will appear earlier in the list.

// applySiacoinInputs incorporates all of the siacoin inputs of a transaction
// into the unconfirmed set.
func (tp *TransactionPool) applySiacoinInputs(t types.Transaction) {
	// For each siacoin input, remove the output from the unconfirmed set and
	// add the output to the reference set.
	for _, sci := range t.SiacoinInputs {
		// Sanity check - input should be in the unconfirmed set and absent
		// from the reference set.
		if build.DEBUG {
			_, exists := tp.referenceSiacoinOutputs[sci.ParentID]
			if exists {
				panic("applying a siacoin output that's already in the reference set")
			}
			_, exists = tp.siacoinOutputs[sci.ParentID]
			if !exists {
				panic("applying a siacoin input that spends an unrecognized siacoin output")
			}
		}

		tp.referenceSiacoinOutputs[sci.ParentID] = tp.siacoinOutputs[sci.ParentID]
		delete(tp.siacoinOutputs, sci.ParentID)
	}
}

// applySiacoinOutputs incorporates all of the siacoin outputs of a transaction
// into the unconfirmed set.
func (tp *TransactionPool) applySiacoinOutputs(t types.Transaction) {
	// For each siacoin output, add the output to the unconfirmed set.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - output should not exist in the unconfirmed set.
		scoid := t.SiacoinOutputID(i)
		if build.DEBUG {
			_, exists := tp.siacoinOutputs[scoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siacoinOutputs[scoid] = sco
	}
}

// applyFileContracts incorporates all of the file contracts of a transaction
// into the unconfirmed set.
func (tp *TransactionPool) applyFileContracts(t types.Transaction) {
	// For each file contract, add the contract to the unconfirmed set.
	for i, fc := range t.FileContracts {
		// Sanity check - file contract should be in the unconfirmed set.
		fcid := t.FileContractID(i)
		if build.DEBUG {
			_, exists := tp.fileContracts[fcid]
			if exists {
				panic("trying to add a file contract that's already in the unconfirmed set")
			}
		}

		tp.fileContracts[fcid] = fc
	}
}

// applyFileContractRevisions incorporates all of the file contract revisions
// of a transaction into the unconfirmed set.
func (tp *TransactionPool) applyFileContractRevisions(t types.Transaction) {
	for _, fcr := range t.FileContractRevisions {
		// Sanity check - file contract should be in the unconfirmed set and
		// absent from the reference set.
		revisionID := crypto.HashAll(fcr.ParentID, fcr.NewRevisionNumber)
		fc, exists := tp.fileContracts[fcr.ParentID]
		if build.DEBUG {
			if !exists {
				panic("could not find file contract")
			}
			_, exists = tp.referenceFileContractRevisions[revisionID]
			if exists {
				panic("reference contract already exists")
			}
		}

		// Delete the old file contract from the unconfirmed set and add it to
		// the reference set.
		delete(tp.fileContracts, fcr.ParentID)
		tp.referenceFileContractRevisions[revisionID] = fc

		// Add the new file contract to the reference set.
		nfc := types.FileContract{
			FileSize:           fcr.NewFileSize,
			FileMerkleRoot:     fcr.NewFileMerkleRoot,
			WindowStart:        fcr.NewWindowStart,
			WindowEnd:          fcr.NewWindowEnd,
			Payout:             fc.Payout,
			ValidProofOutputs:  fcr.NewValidProofOutputs,
			MissedProofOutputs: fcr.NewMissedProofOutputs,
			UnlockHash:         fcr.NewUnlockHash,
			RevisionNumber:     fcr.NewRevisionNumber,
		}
		tp.fileContracts[fcr.ParentID] = nfc
	}
}

// applyStorageProofs incorporates all of the storage proofs of a transaction
// into the unconfirmed set.
func (tp *TransactionPool) applyStorageProofs(t types.Transaction) {
	// For each storage proof, delete the corresponding file contract from the
	// unconfirmed set and add it to the reference set
	for _, sp := range t.StorageProofs {
		// Sanity check - file contract should be in the unconfirmed set and
		// not in the reference set.
		fc, exists := tp.fileContracts[sp.ParentID]
		if build.DEBUG {
			if !exists {
				panic("could not find file contract in unconfirmed set")
			}
			_, exists = tp.referenceFileContracts[sp.ParentID]
			if exists {
				panic("file contract is in both unconfirmed set and reference set")
			}
		}

		tp.referenceFileContracts[sp.ParentID] = fc
		delete(tp.fileContracts, sp.ParentID)
	}
}

// applySiafundInputs incorporates all of the siafund inputs of a transaction
// into the unconfirmed set.
func (tp *TransactionPool) applySiafundInputs(t types.Transaction) {
	// For each siafund input, delete the corresponding siafund output from the
	// unconfirmed set and add it to the reference set.
	for _, sfi := range t.SiafundInputs {
		// Sanity check - the corresponding siafund output should be in the
		// unconfirmed set and absent from the reference set.
		if build.DEBUG {
			_, exists := tp.referenceSiafundOutputs[sfi.ParentID]
			if exists {
				panic("applying a siafund output that's already in the reference set")
			}
			_, exists = tp.siafundOutputs[sfi.ParentID]
			if !exists {
				panic("applying a siafund input that spends an unrecognized siafund output")
			}
		}

		tp.referenceSiafundOutputs[sfi.ParentID] = tp.siafundOutputs[sfi.ParentID]
		delete(tp.siafundOutputs, sfi.ParentID)
	}
}

// applySiafundOutputs incorporates all of the siafund outputs of a transaction
// into the unconfirmed set.
func (tp *TransactionPool) applySiafundOutputs(t types.Transaction) {
	// For each siafund output, add the output to the unconfirmed set.
	for i, sfo := range t.SiafundOutputs {
		// Sanity check - output should not already be in the unconfirmed set.
		sfoid := t.SiafundOutputID(i)
		if build.DEBUG {
			_, exists := tp.siafundOutputs[sfoid]
			if exists {
				panic("trying to add an output that already exists?")
			}
		}

		tp.siafundOutputs[sfoid] = sfo
	}
}

// addTransactionToPool puts a transaction into the transaction pool, changing
// the unconfirmed set and the transaction linked list to reflect the new
// transaction.
func (tp *TransactionPool) addTransactionToPool(t types.Transaction) {
	// Apply each individual part of the transaction to the transaction pool.
	tp.applySiacoinInputs(t)
	tp.applySiacoinOutputs(t)
	tp.applyFileContracts(t)
	tp.applyFileContractRevisions(t)
	tp.applyStorageProofs(t)
	tp.applySiafundInputs(t)
	tp.applySiafundOutputs(t)

	// Add the transaction to the list of transactions.
	tp.transactions[crypto.HashObject(t)] = struct{}{}
	tp.transactionList = append(tp.transactionList, t)
}

// AcceptTransaction adds a transaction to the unconfirmed set of
// transactions. If the transaction is accepted, it will be relayed to
// connected peers.
func (tp *TransactionPool) AcceptTransaction(t types.Transaction) (err error) {
	id := tp.mu.Lock()
	defer tp.mu.Unlock(id)

	// Check that the transaction is not currently in the unconfirmed set.
	txnHash := crypto.HashObject(t)
	_, exists := tp.transactions[txnHash]
	if exists {
		return modules.ErrTransactionPoolDuplicate
	}

	// Check that the transaction is legal given the unconfirmed consensus set
	// and the settings of the transaction pool.
	err = tp.validUnconfirmedTransaction(t)
	if err != nil {
		return
	}

	// Add the transaction to the pool, notify all subscribers, and broadcast
	// the transaction.
	tp.addTransactionToPool(t)
	tp.updateSubscribers(modules.ConsensusChange{}, tp.transactionList, tp.unconfirmedSiacoinOutputDiffs())
	go tp.gateway.Broadcast("RelayTransaction", t)
	return
}

// RelayTransaction is an RPC that accepts a transaction from a peer. If the
// accept is successful, the transaction will be relayed to the Gateway's
// other peers.
func (tp *TransactionPool) RelayTransaction(conn modules.PeerConn) error {
	var t types.Transaction
	err := encoding.ReadObject(conn, &t, types.BlockSizeLimit)
	if err != nil {
		return err
	}
	err = tp.AcceptTransaction(t)
	// ErrTransactionPoolDuplicate is benign
	if err == modules.ErrTransactionPoolDuplicate {
		err = nil
	}
	return err
}
