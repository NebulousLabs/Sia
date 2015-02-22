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
func (tp *TransactionPool) TransactionSet() (transactionSet []consensus.Transaction, err error) {
	tp.update()

	// Add transactions from the head of the linked list until there are no
	// more transactions or until the size limit has been reached.
	var remainingSize int = consensus.BlockSizeLimit - 5e3 // Leave 5kb for the block header and the miner transaction.

	// Iterate through the transactions and add them in first-come-first-serve
	// order.
	currentTxn := tp.head
	for currentTxn != nil {
		// Allocate space for the transaction, exiting the loop if there is not
		// enough space.
		encodedTxn := encoding.Marshal(currentTxn.transaction)
		remainingSize -= len(encodedTxn)
		if remainingSize < 0 {
			break
		}

		// Add the transaction to the transaction set and move onto the next
		// transaction.
		transactionSet = append(transactionSet, currentTxn.transaction)
		currentTxn = currentTxn.next
	}

	return
}

// UnconfirmedSiacoinOutputDiffs returns the set of siacoin output diffs that
// would be created immediately if all of the unconfirmed transactions were
// added to the blockchain.
func (tp *TransactionPool) UnconfirmedSiacoinOutputDiffs() (scods []consensus.SiacoinOutputDiff) {
	tp.update()

	// For each transaction in the linked list, grab the siacoin output diffs
	// that would be created by the transaction.
	currentTxn := tp.head
	for currentTxn != nil {
		// Produce diffs for the siacoin outputs consumed by this transaction.
		txn := currentTxn.transaction
		for _, input := range txn.SiacoinInputs {
			scod := consensus.SiacoinOutputDiff{
				Direction: consensus.DiffRevert,
				ID:        input.ParentID,
			}

			// Get the output from tpool if it's a new output, and from the
			// state if it already existed.
			output, exists := tp.siacoinOutputs[input.ParentID]
			if !exists {
				output, exists = tp.state.SiacoinOutput(input.ParentID)

				// Sanity check - the output should exist in the state because
				// the transaction is in the transaction pool.
				if consensus.DEBUG {
					if !exists {
						panic("output in tpool txn that's neither in the state or in the tpool")
					}
				}
			}
			scod.SiacoinOutput = output

			scods = append(scods, scod)
		}

		// Produce diffs for the siacoin outputs created by this transaction.
		for i, output := range txn.SiacoinOutputs {
			scod := consensus.SiacoinOutputDiff{
				Direction:     consensus.DiffApply,
				ID:            txn.SiacoinOutputID(i),
				SiacoinOutput: output,
			}
			scods = append(scods, scod)
		}

		currentTxn = currentTxn.next
	}

	return
}
