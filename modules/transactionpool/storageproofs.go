package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/hash"
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
		_, err = tp.state.Contract(sp.ContractID)
		if err != nil {
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
		tp.storageProofs[greatestHeight] = make(map[hash.Hash]consensus.Transaction)
		heightMap = tp.storageProofs[greatestHeight]
	}
	_, exists = heightMap[hash.HashObject(t)]
	if exists {
		err = errors.New("transaction already known")
		return
	}
	heightMap[hash.HashObject(t)] = t
	return
}

// When doing a pool dump, need to find a way to grab all of the storageProofs
// that you can without grabbing proofs that conflict with each other or repeat
// on the same contract.
