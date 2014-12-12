package consensus

import (
	"testing"
)

// TestApplyTransaction provides testing coverage for State.applyTransaction()
func TestApplyTransaction(t *testing.T) {
	// Create a state to which transactions can be applied.
	s := CreateGenesisState()

	// Create a transaction with one input and one output.
	transaction := Transaction{
		Inputs:  []Input{Input{OutputID: s.CurrentBlock().SubsidyID()}},
		Outputs: []Output{Output{Value: 1}},
	}
	s.applyTransaction(transaction)

	// Check that the genesis subsidy got deleted.
	_, exists := s.unspentOutputs[s.CurrentBlock().SubsidyID()]
	if exists {
		t.Error("apply transaction did not remove the output from the unspent outputs list.")
	}

	// Check that the new output got added.
	_, exists = s.unspentOutputs[transaction.OutputID(0)]
	if !exists {
		t.Error("new output not added during applyTransaction")
	}
}
