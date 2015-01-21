package consensus

// A block is composed of many transactions. Blocks that have transactions that
// depend on other transactions, but the transactions must all be applied in a
// deterministic order. Transactions cannot have inter-dependencies, meaning
// that an output cannot be created and then spent in the same transaction. As
// far as diffs are concenred, this means that the elements of a transaction
// diff should be able to be applied in any order and the outcome should be the
// same. The elements of a block diff however must be applied in a specific
// order, as transactions may depend on each other.

// An OutputDiff indicates an output that has either been added to or removed
// from the unspent outputs set. New=true means that the output was added when
// the block was applied, and new=false means that the output was deleted when
// the block was applied.
type OutputDiff struct {
	New    bool
	ID     OutputID
	Output Output
}

type ContractDiff struct {
	New      bool
	ID       ContractID
	Contract FileContract
}

// applyOutputDiff takes an output diff and applies it to the state. Forward
// indicates the direction of the blockchain.
func (s *State) applyOutputDiff(od OutputDiff, forward bool) {
	add := od.New
	if !forward {
		add = !add
	}

	if add {
		// Sanity check - output should not already exist.
		if DEBUG {
			_, exists := s.unspentOutputs[od.ID]
			if exists {
				panic("rogue new output in applyOutputDiff")
			}
		}

		s.unspentOutputs[od.ID] = od.Output
	} else {
		// Sanity check - output should exist.
		if DEBUG {
			_, exists := s.unspentOutputs[od.ID]
			if !exists {
				panic("rogue non-new output in applyOutputDiff")
			}
		}

		delete(s.unspentOutputs, od.ID)
	}
}

// applyContractDiff takes a contract diff and applies it to the state. Forward
// indicates the direction of the blockchain.
func (s *State) applyContractDiff(cd ContractDiff, forward bool) {
	add := cd.New
	if !forward {
		add = !add
	}

	if add {
		// Sanity check - contract should not already exist.
		if DEBUG {
			_, exists := s.openContracts[cd.ID]
			if exists {
				panic("rogue new contract in applyContractDiff")
			}
		}

		s.openContracts[cd.ID] = cd.Contract
	} else {
		// Sanity check - contract should exist.
		if DEBUG {
			_, exists := s.openContracts[cd.ID]
			if !exists {
				panic("rogue non-new contract in applyContractDiff")
			}
		}

		delete(s.openContracts, cd.ID)
	}
}
