package consensus

import (
	"testing"
)

// TestApplyTransaction provides testing coverage for State.applyTransaction()
func TestApplyTransaction(t *testing.T) {
	// Create a state to which transactions can be applied.
	s := CreateGenesisState()
	genesisSubsidyID := s.currentBlockNode().block.SubsidyID()

	// Check that the genesis subsidy exists.
	_, exists := s.unspentOutputs[genesisSubsidyID]
	if !exists {
		t.Fatal("genesis subsidy not found after calling CreateGenesisState")
	}

	// Create a transaction with one input and one output.
	transaction := Transaction{
		Inputs:  []Input{Input{OutputID: genesisSubsidyID}},
		Outputs: []Output{Output{Value: 1}},
	}
	s.applyTransaction(transaction)

	// Check that the genesis subsidy got deleted.
	_, exists = s.unspentOutputs[genesisSubsidyID]
	if exists {
		t.Error("apply transaction did not remove the output from the unspent outputs list.")
	}

	// Check that the new output got added.
	_, exists = s.unspentOutputs[transaction.OutputID(0)]
	if !exists {
		t.Error("new output not added during applyTransaction")
	}
}
