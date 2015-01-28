package transactionpool

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
)

// TransactionSet will return a list of transactions that can be put in a block
// in order, and will not result in the block being too large. TransactionSet
// prioritizes transactions that have already been in a block (on another
// fork), and then treats remaining transactions in a first come first serve
// manner.
func (tp *TransactionPool) TransactionSet() (transactions []consensus.Transaction, err error) {
	// Add transactions from the head of the linked list until there are no
	// more transactions or until the size limit has been reached.
	remainingSize := consensus.BlockSizeLimit - 1024 // Leave 1kb for block header and metadata, which should actually only be about 120 bytes.

	// Add storage proofs.
	transactions, sizeUsed := tp.storageProofTransactionSet(remainingSize)
	remainingSize -= sizeUsed

	currentTxn := tp.head
	for currentTxn != nil {
		// Make sure that any contracts created in the transaction are still
		// valid - if a transaction doesn't make it into the state in time, the
		// contract inside can become invalid.
		for _, contract := range currentTxn.transaction.FileContracts {
			err = tp.state.ValidContract(contract)
			if err != nil {
				// Break out of the inner loop but pass the error down.
				break
			}
		}
		if err != nil {
			badTxn := currentTxn
			currentTxn = currentTxn.next
			tp.removeTransactionFromList(badTxn)
			continue
		}

		// Allocate space for the transaction, breaking if there is not enough
		// space.
		encodedTxn := encoding.Marshal(currentTxn.transaction)
		remainingSize -= len(encodedTxn)
		if remainingSize < 0 {
			break
		}

		// Add the transaction to the list, without updating the linked list.
		// (linked list updating only happens when processing an update from
		// the state or getting a new transaction)
		transactions = append(transactions, currentTxn.transaction)
		currentTxn = currentTxn.next
	}

	return
}
