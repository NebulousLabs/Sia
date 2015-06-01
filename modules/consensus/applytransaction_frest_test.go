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
	for _, diff := range bn.fileContractDiffs {
		cst.cs.commitFileContractDiff(diff, modules.DiffRevert)
	}
	for _, diff := range bn.siafundOutputDiffs {
		cst.cs.commitSiafundOutputDiff(diff, modules.DiffRevert)
	}
	if initialSum != cst.cs.consensusSetHash() {
		return errors.New("inconsistency after rewinding a diff set")
	}
	for _, diff := range bn.siacoinOutputDiffs {
		cst.cs.commitSiacoinOutputDiff(diff, modules.DiffApply)
	}
	for _, diff := range bn.fileContractDiffs {
		cst.cs.commitFileContractDiff(diff, modules.DiffApply)
	}
	for _, diff := range bn.siafundOutputDiffs {
		cst.cs.commitSiafundOutputDiff(diff, modules.DiffApply)
	}
	if resultingSum != cst.cs.consensusSetHash() {
		return errors.New("inconsistency after reapplying a diff set")
	}
	return nil
}

// TestApplySiacoinInputs probes the applySiacoinInputs method of the consensus
// set.
func TestApplySiacoinInputs(t *testing.T) {
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

// TestMisuseApplySiacoinInputs misuses applySiacoinInput and checks that a
// panic was triggered.
func TestMisuseApplySiacoinInputs(t *testing.T) {
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

// TestApplySiacoinOutputs probes the applySiacoinOutput method of the
// consensus set.
func TestApplySiacoinOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
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
		t.Error("wrong diff direction applied when creating a siacoin output")
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
		t.Error("block node was not updated correctly")
	}

	err = cst.checkRevertApplyNode(initialSum, bn)
	if err != nil {
		t.Error(err)
	}
}

// TestMisuseApplySiacoinOutputs misuses applySiacoinOutputs and checks that a
// panic was triggered.
func TestMisuseApplySiacoinOutputs(t *testing.T) {
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

// TestApplyFileContracts probes the appliyFileContracts method of the
// consensus set.
func TestApplyFileContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyFileContracts")
	if err != nil {
		t.Fatal(err)
	}

	// Grab the inital hash of the consensus set.
	initialSum := cst.cs.consensusSetHash()

	// Create a block node to use with application.
	bn := new(blockNode)

	// Apply a transaction with a single file contract.
	txn := types.Transaction{
		FileContracts: []types.FileContract{{}},
	}
	cst.cs.applyFileContracts(bn, txn)
	fcid := txn.FileContractID(0)
	_, exists := cst.cs.fileContracts[fcid]
	if !exists {
		t.Error("Failed to create file contract")
	}
	if len(cst.cs.fileContracts) != 1 {
		t.Error("file contracts not correctly updated")
	}
	if len(bn.fileContractDiffs) != 1 {
		t.Error("block node was not updated for single element transaction")
	}
	if bn.fileContractDiffs[0].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when creating a file contract")
	}
	if bn.fileContractDiffs[0].ID != fcid {
		t.Error("wrong id used when creating a file contract")
	}

	// Apply a transaction with 2 file contracts.
	txn = types.Transaction{
		FileContracts: []types.FileContract{
			{Payout: types.NewCurrency64(1)},
			{Payout: types.NewCurrency64(2)},
		},
	}
	cst.cs.applyFileContracts(bn, txn)
	fcid0 := txn.FileContractID(0)
	fcid1 := txn.FileContractID(1)
	_, exists = cst.cs.fileContracts[fcid0]
	if !exists {
		t.Error("Failed to create file contract")
	}
	_, exists = cst.cs.fileContracts[fcid1]
	if !exists {
		t.Error("Failed to create file contract")
	}
	if len(cst.cs.fileContracts) != 3 {
		t.Error("file contracts not correctly updated")
	}
	if len(bn.fileContractDiffs) != 3 {
		t.Error("block node was not updated correctly")
	}

	err = cst.checkRevertApplyNode(initialSum, bn)
	if err != nil {
		t.Error(err)
	}
}

// TestMisuseApplyFileContracts misuses applyFileContracts and checks that a
// panic was triggered.
func TestMisuseApplyFileContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	bn := new(blockNode)

	// Apply a transaction with a single file contract.
	txn := types.Transaction{
		FileContracts: []types.FileContract{{}},
	}
	cst.cs.applyFileContracts(bn, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplyFileContracts {
			t.Error("no panic occured when misusing applySiacoinInput")
		}
	}()
	cst.cs.applyFileContracts(bn, txn)
}

// TestApplyFileContractRevisions probes the appliyFileContractRevisions method
// of the consensus set.
func TestApplyFileContractRevisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyFileContracts")
	if err != nil {
		t.Fatal(err)
	}

	// Grab the inital hash of the consensus set.
	initialSum := cst.cs.consensusSetHash()

	// Create a block node to use with application.
	bn := new(blockNode)

	// Apply a transaction with two file contracts - that way there is
	// something to revise.
	txn := types.Transaction{
		FileContracts: []types.FileContract{
			{},
			{Payout: types.NewCurrency64(1)},
		},
	}
	cst.cs.applyFileContracts(bn, txn)
	fcid0 := txn.FileContractID(0)
	fcid1 := txn.FileContractID(1)

	// Apply a single file contract revision.
	txn = types.Transaction{
		FileContractRevisions: []types.FileContractRevision{
			{
				ParentID:    fcid0,
				NewFileSize: 1,
			},
		},
	}
	cst.cs.applyFileContractRevisions(bn, txn)
	fc, exists := cst.cs.fileContracts[fcid0]
	if !exists {
		t.Error("Revision killed a file contract")
	}
	if fc.FileSize != 1 {
		t.Error("file contract filesize not properly updated")
	}
	if len(cst.cs.fileContracts) != 2 {
		t.Error("file contracts not correctly updated")
	}
	if len(bn.fileContractDiffs) != 4 { // 2 creating the initial contracts, 1 to remove the old, 1 to add the revision.
		t.Error("block node was not updated for single element transaction")
	}
	if bn.fileContractDiffs[2].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when revising a file contract")
	}
	if bn.fileContractDiffs[3].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when revising a file contract")
	}
	if bn.fileContractDiffs[2].ID != fcid0 {
		t.Error("wrong id used when revising a file contract")
	}
	if bn.fileContractDiffs[3].ID != fcid0 {
		t.Error("wrong id used when revising a file contract")
	}

	// Apply a transaction with 2 file contract revisions.
	txn = types.Transaction{
		FileContractRevisions: []types.FileContractRevision{
			{
				ParentID:    fcid0,
				NewFileSize: 2,
			},
			{
				ParentID:    fcid1,
				NewFileSize: 3,
			},
		},
	}
	cst.cs.applyFileContractRevisions(bn, txn)
	fc0, exists := cst.cs.fileContracts[fcid0]
	if !exists {
		t.Error("Revision ate file contract")
	}
	fc1, exists := cst.cs.fileContracts[fcid1]
	if !exists {
		t.Error("Revision ate file contract")
	}
	if fc0.FileSize != 2 {
		t.Error("Revision not correctly applied")
	}
	if fc1.FileSize != 3 {
		t.Error("Revision not correctly applied")
	}
	if len(cst.cs.fileContracts) != 2 {
		t.Error("file contracts not correctly updated")
	}
	if len(bn.fileContractDiffs) != 8 {
		t.Error("block node was not updated correctly")
	}

	err = cst.checkRevertApplyNode(initialSum, bn)
	if err != nil {
		t.Error(err)
	}
}

// TestMisuseApplyFileContractRevisions misuses applyFileContractRevisions and
// checks that a panic was triggered.
func TestMisuseApplyFileContractRevisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	bn := new(blockNode)

	// Trigger a panic from revising a nonexistent file contract.
	defer func() {
		r := recover()
		if r != ErrMisuseApplyFileContractRevisions {
			t.Error("no panic occured when misusing applySiacoinInput")
		}
	}()
	txn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{}},
	}
	cst.cs.applyFileContractRevisions(bn, txn)
}

/*
// TestApplySiafundInputs probes the applySiafundInputs method of the consensus
// set.
func TestApplySiafundInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
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

// TestMisuseApplySiacoinInputs misuses applySiacoinInput and checks that a
// panic was triggered.
func TestMisuseApplySiacoinInputs(t *testing.T) {
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
*/

// TestApplySiafundOutputs probes the applySiafundOutputs method of the
// consensus set.
func TestApplySiafundOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}
	cst.cs.siafundPool = types.NewCurrency64(101)

	// Grab the inital hash of the consensus set.
	initialSum := cst.cs.consensusSetHash()

	// Create a block node to use with application.
	bn := new(blockNode)

	// Apply a transaction with a single siafund output.
	txn := types.Transaction{
		SiafundOutputs: []types.SiafundOutput{{}},
	}
	cst.cs.applySiafundOutputs(bn, txn)
	sfoid := txn.SiafundOutputID(0)
	_, exists := cst.cs.siafundOutputs[sfoid]
	if !exists {
		t.Error("Failed to create siafund output")
	}
	if len(cst.cs.siafundOutputs) != 2 { // TODO: This value needs to be updated when siafunds are added.
		t.Error("siafund outputs not correctly updated")
	}
	if len(bn.siafundOutputDiffs) != 1 {
		t.Error("block node was not updated for single element transaction")
	}
	if bn.siafundOutputDiffs[0].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when creating a siafund output")
	}
	if bn.siafundOutputDiffs[0].ID != sfoid {
		t.Error("wrong id used when creating a siafund output")
	}
	if bn.siafundOutputDiffs[0].SiafundOutput.ClaimStart.Cmp(types.NewCurrency64(101)) != 0 {
		t.Error("claim start set incorrectly when creating a siafund output")
	}

	// Apply a transaction with 2 siacoin outputs.
	txn = types.Transaction{
		SiafundOutputs: []types.SiafundOutput{
			{Value: types.NewCurrency64(1)},
			{Value: types.NewCurrency64(2)},
		},
	}
	cst.cs.applySiafundOutputs(bn, txn)
	sfoid0 := txn.SiafundOutputID(0)
	sfoid1 := txn.SiafundOutputID(1)
	_, exists = cst.cs.siafundOutputs[sfoid0]
	if !exists {
		t.Error("Failed to create siafund output")
	}
	_, exists = cst.cs.siafundOutputs[sfoid1]
	if !exists {
		t.Error("Failed to create siafund output")
	}
	if len(cst.cs.siafundOutputs) != 4 { // TODO: This value needs to be added when genesis siafunds are added.
		t.Error("siafund outputs not correctly updated")
	}
	if len(bn.siafundOutputDiffs) != 3 {
		t.Error("block node was not updated for single element transaction")
	}

	err = cst.checkRevertApplyNode(initialSum, bn)
	if err != nil {
		t.Error(err)
	}
}

// TestMisuseApplySiafundOutputs misuses applySiafundOutputs and checks that a
// panic was triggered.
func TestMisuseApplySiafundOutputs(t *testing.T) {
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
		SiafundOutputs: []types.SiafundOutput{{}},
	}
	cst.cs.applySiafundOutputs(bn, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplySiafundOutput {
			t.Error("no panic occured when misusing applySiafundInput")
		}
	}()
	cst.cs.applySiafundOutputs(bn, txn)
}
