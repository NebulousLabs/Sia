// +build testing

package types

import (
	"errors"
)

// TransactionGraph will return a set of valid transactions that all spend
// outputs according to the input graph. Each [source, dest] pair defines an
// edge of the graph. The graph must be fully connected and the granparent of
// the graph must be the sourceOutput. '0' refers to an edge from the source
// output. Each edge also specifies a value for the output, and an amount of
// fees. If the fees are zero, no fees will be added for that edge. 'sources'
// must be sorted.
//
// Example of acceptable input:
//
// sourceOutput: // a valid siacoin output spending to UnlockConditions{}.UnlockHash()
//
// Sources: [0, 0, 1, 2, 3, 3, 3, 4]
// Dests:   [1, 2, 3, 3, 4, 4, 5, 6]
//
// Resulting Graph:
//
//    o
//   / \
//  o   o
//   \ /
//    o
//   /|\
//   \| \
//    o  x // 'x' transactions are symbolic, not actually created
//    |
//    x
//
func TransactionGraph(sourceOutput SiacoinOutputID, sources []int, dests []int, outputValues []Currency, fees []Currency) ([]Transaction, error) {
	// Basic input validation.
	if len(sources) < 1 {
		return nil, errors.New("no graph specificed")
	}
	if len(dests) != len(sources) || len(outputValues) != len(sources) || len(fees) != len(sources) {
		return nil, errors.New("invalid [sources, dests, outputValues, fees] tuples")
	}

	// Check that the first value of 'sources' is zero, and that the rest of the
	// array is sorted.
	if sources[0] != 0 {
		return nil, errors.New("first edge must speficy node 0 as the parent")
	}
	if dests[0] != 1 {
		return nil, errors.New("first edge must speficy node 1 as the child")
	}
	latest := sources[0]
	for _, parent := range sources {
		if parent < latest {
			return nil, errors.New("'sources' input is not sorted")
		}
		latest = parent
	}

	// Create the set of output ids, and fill out the input ids for the source
	// transaction.
	biggest := 0
	for _, dest := range dests {
		if dest > biggest {
			biggest = dest
		}
	}
	txnInputs := make([][]SiacoinOutputID, biggest+1)
	txnInputs[0] = []SiacoinOutputID{sourceOutput}

	// Go through the nodes bit by bit and create outputs.
	// Fill out the outputs for the source.
	j := 0
	ts := make([]Transaction, sources[len(sources)-1]+1)
	for i := 0; i < len(sources); i++ {
		var t Transaction

		// Grab the inputs for this transaction.
		for _, outputID := range txnInputs[j] {
			t.SiacoinInputs = append(t.SiacoinInputs, SiacoinInput{
				ParentID: outputID,
			})
		}

		// Grab the outputs for this transaction.
		startingPoint := i
		current := sources[i]
		for i < len(sources) && sources[i] == current {
			t.SiacoinOutputs = append(t.SiacoinOutputs, SiacoinOutput{
				Value: outputValues[i],
				UnlockHash: UnlockConditions{}.UnlockHash(),
			})
			if !fees[i].IsZero() {
				t.MinerFees = append(t.MinerFees, fees[i])
			}
			i++
		}

		// Record the inputs for the next transactions.
		for k := startingPoint; k < i; k++ {
			txnInputs[dests[k]] = append(txnInputs[dests[k]], t.SiacoinOutputID(uint64(k-startingPoint)))
		}
		ts[j] = t
		j++
	}

	return ts, nil
}
