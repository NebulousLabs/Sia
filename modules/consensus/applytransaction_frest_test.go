package consensus

import (
	"errors"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// checkRewindApplyNode is a helper function that reverts and reapplies a block
// node, checking for consistency with the original and resulting consensus
// hashes.
func (cst *consensusSetTester) checkRevertApplyNode(initialSum crypto.Hash, bn *blockNode) error {
	resultingSum := cst.cs.consensusSetHash()

	// Revert and reapply the diffs and check that consistency is maintained.
	for _, diff := range bn.siacoinOutputDiffs {
		cst.cs.commitSiacoinOutputDiff(diff, modules.DiffRevert)
	}
	if initialSum != cst.cs.consensusSetHash() {
		return errors.New("inconsistency after rewinding a diff set")
	}
	for _, diff := range bn.siacoinOutputDiffs {
		cst.cs.commitSiacoinOutputDiff(diff, modules.DiffApply)
	}
	if resultingSum != cst.cs.consensusSetHash() {
		return errors.New("inconsistency after rewinding a diff set")
	}
	return nil
}

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
			{ParentID: ids[0]},
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
			{ParentID: ids[1]},
			{ParentID: ids[2]},
		},
	}
	cst.cs.applySiacoinInputs(bn, txn)
	if len(cst.cs.siacoinOutputs) != 0 {
		t.Error("failed to consume all siacoin outputs in the consensus set")
	}
	if len(bn.siacoinOutputDiffs) != 3 {
		t.Error("block node was not updated for single transaction")
	}

	err = cst.checkRevertApplyNode(initialSum, bn)
	if err != nil {
		t.Error(err)
	}
}

// TestMisuseApplySiacoinInput misuses applySiacoinInput and checks that a
// panic was triggered.
func TestMisuseApplySiacoinInput(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
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
			{ParentID: ids[0]},
		},
	}
	cst.cs.applySiacoinInputs(bn, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplySiacoinInput {
			t.Error("no panic occured when misusing applySiacoinInput")
		}
	}()
	cst.cs.applySiacoinInputs(bn, txn)
}

// TestApplySiacoinOutput probes the applySiacoinOutput method of the consensus
// set.
func TestApplySiacoinOutput(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a consensus set and get it to 3 siacoin outputs. The consensus
	// set starts with 2 siacoin outputs, mining a block will add another.
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}

	// Grab the inital hash of the consensus set.
	initialSum := cst.cs.consensusSetHash()

	// Create a block node to use with application.
	bn := new(blockNode)

	// Apply a transaction with a single siacoin output.
	txn := types.Transaction{
		SiacoinOutputs: []types.SiacoinOutput{{}},
	}
	cst.cs.applySiacoinOutputs(bn, txn)
	scoid := txn.SiacoinOutputID(0)
	_, exists := cst.cs.siacoinOutputs[scoid]
	if !exists {
		t.Error("Failed to create siacoin output")
	}
	if len(cst.cs.siacoinOutputs) != 3 { // 3 because createConsensusSetTester has 2 initially.
		t.Error("siacoin outputs not correctly updated")
	}
	if len(bn.siacoinOutputDiffs) != 1 {
		t.Error("block node was not updated for single element transaction")
	}
	if bn.siacoinOutputDiffs[0].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when consuming a siacoin output")
	}
	if bn.siacoinOutputDiffs[0].ID != scoid {
		t.Error("wrong id used when creating a siacoin output")
	}

	// Apply a transaction with 2 siacoin outputs.
	txn = types.Transaction{
		SiacoinOutputs: []types.SiacoinOutput{
			{Value: types.NewCurrency64(1)},
			{Value: types.NewCurrency64(2)},
		},
	}
	cst.cs.applySiacoinOutputs(bn, txn)
	scoid0 := txn.SiacoinOutputID(0)
	scoid1 := txn.SiacoinOutputID(1)
	_, exists = cst.cs.siacoinOutputs[scoid0]
	if !exists {
		t.Error("Failed to create siacoin output")
	}
	_, exists = cst.cs.siacoinOutputs[scoid1]
	if !exists {
		t.Error("Failed to create siacoin output")
	}
	if len(cst.cs.siacoinOutputs) != 5 { // 5 because createConsensusSetTester has 2 initially.
		t.Error("siacoin outputs not correctly updated")
	}
	if len(bn.siacoinOutputDiffs) != 3 {
		t.Error("block node was not updated for single element transaction")
	}

	err = cst.checkRevertApplyNode(initialSum, bn)
	if err != nil {
		t.Error(err)
	}
}

// TestMisuseApplySiacoinOutput misuses applySiacoinOutput and checks that a
// panic was triggered.
func TestMisuseApplySiacoinOutput(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	bn := new(blockNode)

	// Apply a transaction with a single siacoin output.
	txn := types.Transaction{
		SiacoinOutputs: []types.SiacoinOutput{{}},
	}
	cst.cs.applySiacoinOutputs(bn, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplySiacoinOutput {
			t.Error("no panic occured when misusing applySiacoinInput")
		}
	}()
	cst.cs.applySiacoinOutputs(bn, txn)
}
