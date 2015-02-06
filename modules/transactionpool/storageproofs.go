package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

func (tp *TransactionPool) acceptStorageProofTransaction(t consensus.Transaction) (err error) {
	// Sanity Check - transaction should contain at least 1 storage proof.
	if consensus.DEBUG {
		if len(t.StorageProofs) < 1 {
			panic("misuse of acceptStorageProofTransaction")
		}
	}

	// Check that each storage proof acts on an existing contract in the
	// blockchain.
	var greatestHeight consensus.BlockHeight
	for _, sp := range t.StorageProofs {
		var contract consensus.FileContract
		_, exists := tp.state.Contract(sp.FileContractID)
		if !exists {
			err = errors.New("storage proof is for a nonexistant contract")
			return
		}

		// Track the highest start height of the contracts that the proofs
		// fulfill.
		if contract.Start > greatestHeight {
			greatestHeight = contract.Start
		}
	}

	// Put the transaction in the proof map.
	heightMap, exists := tp.storageProofs[greatestHeight]
	if !exists {
		tp.storageProofs[greatestHeight] = make(map[crypto.Hash]consensus.Transaction)
		heightMap = tp.storageProofs[greatestHeight]
	}
	_, exists = heightMap[crypto.HashObject(t)]
	if exists {
		err = errors.New("transaction already known")
		return
	}
	heightMap[crypto.HashObject(t)] = t
	return
}

func (tp *TransactionPool) storageProofTransactionSet(remainingSize int) (transactions []consensus.Transaction, sizeUsed int) {
	contractsSatisfied := make(map[consensus.FileContractID]struct{})

	// Get storage proofs for all heights from 12 earlier to the current
	// height.
	for height := tp.state.Height() - 12; height != tp.state.Height(); height++ {
	TxnLoop:
		for _, txn := range tp.storageProofs[height] {
			// Check that the transaction is valid, and that none of the
			// storage proofs have already been used in another transaction.
			err := tp.state.ValidTransaction(txn)
			if err != nil {
				continue // don't remove the transaction because it might be valid on another fork. (this action is only taken for storage proofs)
			}

			for _, proof := range txn.StorageProofs {
				_, exists := contractsSatisfied[proof.FileContractID]
				if exists {
					continue TxnLoop
				}
			}
			for _, proof := range txn.StorageProofs {
				contractsSatisfied[proof.FileContractID] = struct{}{}
			}

			// Check for size requirements.
			encodedTxn := encoding.Marshal(txn)
			remainingSize -= len(encodedTxn)
			if remainingSize < 0 {
				return
			}
			sizeUsed += len(encodedTxn)
			transactions = append(transactions, txn)
		}
	}

	return
}
