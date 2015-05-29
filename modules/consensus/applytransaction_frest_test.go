package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestApplySiacoinInput probes the applySiacoinInput method of the consensus
// set.
func TestApplySiacoinInput(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a consensus set and get it to 3 siacoin outputs. The consensus
	// set starts with 2 siacoin outputs, mining a block will add another.
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = cst.miner.FindBlock()
	if err != nil {
		t.Fatal(err)
	}
	cst.csUpdateWait()

	// Grab the inital hash of the consensus set.
	initialSum := cst.cs.consensusSetHash()

	// Create a block node to use with application.
	bn := new(blockNode)

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiacoinOutputID
	for id, _ := range cst.cs.siacoinOutputs {
		ids = append(ids, id)
	}

	// Apply a transaction with a single siacoin input.
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			types.SiacoinInput{
				ParentID: ids[0],
			},
		},
	}
	cst.cs.applySiacoinInputs(bn, txn)
	_, exists := cst.cs.siacoinOutputs[ids[0]]
	if exists {
		t.Error("Failed to conusme a siacoin output")
	}
	if len(cst.cs.siacoinOutputs) != 2 {
		t.Error("siacoin outputs not correctly updated")
	}
	if len(bn.siacoinOutputDiffs) != 1 {
		t.Error("block node was not updated for single transaction")
	}
	if bn.siacoinOutputDiffs[0].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when consuming a siacoin output")
	}
	if bn.siacoinOutputDiffs[0].ID != ids[0] {
		t.Error("wrong id used when consuming a siacoin output")
	}

	// Apply a transaction with two siacoin inputs.
	txn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			types.SiacoinInput{
				ParentID: ids[1],
			},
			types.SiacoinInput{
				ParentID: ids[2],
			},
		},
	}
	cst.cs.applySiacoinInputs(bn, txn)
	if len(cst.cs.siacoinOutputs) != 0 {
		t.Error("failed to consume all siacoin outputs in the consensus set")
	}
	if len(bn.siacoinOutputDiffs) != 3 {
		t.Error("block node was not updated for single transaction")
	}

	// Get the resulting consensus set hash.
	resultingSum := cst.cs.consensusSetHash()
	if initialSum == resultingSum {
		t.Error("consensus set hash is consistent")
	}

	// Revert and reapply the diffs and check that consistency is maintained.
	for _, diff := range bn.siacoinOutputDiffs {
		cst.cs.commitSiacoinOutputDiff(diff, modules.DiffRevert)
	}
	if initialSum != cst.cs.consensusSetHash() {
		t.Error("inconsistency after rewinding a diff set")
	}
	for _, diff := range bn.siacoinOutputDiffs {
		cst.cs.commitSiacoinOutputDiff(diff, modules.DiffApply)
	}
	if resultingSum != cst.cs.consensusSetHash() {
		t.Error("inconsistency after rewinding a diff set")
	}
}

// TestMisuseApplySiacoinInput misuses applySiacoinInput and checks that a
// panic was triggered.
func TestMisuseApplysiacoinInput(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a consensus set and get it to 3 siacoin outputs. The consensus
	// set starts with 2 siacoin outputs, mining a block will add another.
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	bn := new(blockNode)

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiacoinOutputID
	for id, _ := range cst.cs.siacoinOutputs {
		ids = append(ids, id)
	}

	// Apply a transaction with a single siacoin input.
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			types.SiacoinInput{
				ParentID: ids[0],
			},
		},
	}
	cst.cs.applySiacoinInputs(bn, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplySiacoinInput {
			t.Error("no panic occured when misusing applySiacoinInput")
			t.Error(r)
		}
	}()
	cst.cs.applySiacoinInputs(bn, txn)
}
