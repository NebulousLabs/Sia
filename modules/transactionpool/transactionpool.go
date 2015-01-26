package transactionpool

import (
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
)

type unconfirmedTransaction struct {
	transaction  consensus.Transaction
	requirements []*unconfirmedTransaction
	dependents   []*unconfirmedTransaction
}

type TransactionPool struct {
	usedOutputs   map[consensus.OutputID]*unconfirmedTransaction
	newOutputs    map[consensus.OutputID]*unconfirmedTransaction
	storageProofs map[consensus.ContractID]*unconfirmedTransaction

	transactionList map[consensus.OutputID]*unconfirmedTransaction

	mu sync.RWMutex
}

func (tp *TransactionPool) addTransaction(t consensus.Transaction) {
}

func (tp *TransactionPool) AcceptTransaction(t consensus.Transaction) (err error) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Check for conflicts with existing unconfirmed transactions.
	if tp.conflict(t) {
		err = ConflictingTransactionErr
		return
	}

	// Check that the transaction is legal.
	err = tp.validTransaction(t)
	if err != nil {
		return
	}

	// Add the transaction.
	err = s.addTransaction(t)
	if consensus.DEBUG {
		if err != nil {
			panic(err)
		}
	}
}
