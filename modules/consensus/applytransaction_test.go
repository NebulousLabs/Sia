package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestApplySiacoinInputs probes the applySiacoinInputs method of the consensus
// set.
func TestApplySiacoinInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a consensus set and get it to 3 siacoin outputs. The consensus
	// set starts with 2 siacoin outputs, mining a block will add another.
	cst, err := createConsensusSetTester("TestApplySiacoinInputs")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	cst.csUpdateWait()

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
}

// TestMisuseApplySiacoinInputs misuses applySiacoinInput and checks that a
// panic was triggered.
func TestMisuseApplySiacoinInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMisuseApplySiacoinInputs")
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
	cst, err := createConsensusSetTester("TestApplySiacoinOutputs")
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
}

// TestMisuseApplySiacoinOutputs misuses applySiacoinOutputs and checks that a
// panic was triggered.
func TestMisuseApplySiacoinOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMisuseApplySiacoinOutputs")
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

// TestApplyFileContracts probes the applyFileContracts method of the
// consensus set.
func TestApplyFileContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyFileContracts")
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
}

// TestMisuseApplyFileContracts misuses applyFileContracts and checks that a
// panic was triggered.
func TestMisuseApplyFileContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMisuseApplyFileContracts")
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

// TestApplyFileContractRevisions probes the applyFileContractRevisions method
// of the consensus set.
func TestApplyFileContractRevisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyFileContractRevisions")
	if err != nil {
		t.Fatal(err)
	}

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
}

// TestMisuseApplyFileContractRevisions misuses applyFileContractRevisions and
// checks that a panic was triggered.
func TestMisuseApplyFileContractRevisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMisuseApplyFileContractRevisions")
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

// TestApplyStorageProofs probes the applyStorageProofs method of the consensus
// set.
func TestApplyStorageProofs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyStorageProofs")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	bn := new(blockNode)
	bn.height = cst.cs.height()

	// Apply a transaction with two file contracts - there is a reason to
	// create a storage proof.
	txn := types.Transaction{
		FileContracts: []types.FileContract{
			{
				Payout: types.NewCurrency64(300e3),
				ValidProofOutputs: []types.SiacoinOutput{
					{Value: types.NewCurrency64(290e3)},
				},
			},
			{},
			{
				Payout: types.NewCurrency64(600e3),
				ValidProofOutputs: []types.SiacoinOutput{
					{Value: types.NewCurrency64(280e3)},
					{Value: types.NewCurrency64(300e3)},
				},
			},
		},
	}
	cst.cs.applyFileContracts(bn, txn)
	fcid0 := txn.FileContractID(0)
	fcid1 := txn.FileContractID(1)
	fcid2 := txn.FileContractID(2)

	// Apply a single storage proof.
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{{ParentID: fcid0}},
	}
	cst.cs.applyStorageProofs(bn, txn)
	_, exists := cst.cs.fileContracts[fcid0]
	if exists {
		t.Error("Storage proof did not disable a file contract.")
	}
	if len(cst.cs.fileContracts) != 2 {
		t.Error("file contracts not correctly updated")
	}
	if len(bn.fileContractDiffs) != 4 { // 3 creating the initial contracts, 1 for the storage proof.
		t.Error("block node was not updated for single element transaction")
	}
	if bn.fileContractDiffs[3].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when revising a file contract")
	}
	if bn.fileContractDiffs[3].ID != fcid0 {
		t.Error("wrong id used when revising a file contract")
	}
	spoid0 := fcid0.StorageProofOutputID(types.ProofValid, 0)
	sco, exists := cst.cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][spoid0]
	if !exists {
		t.Error("storage proof output not created after applying a storage proof")
	}
	if sco.Value.Cmp(types.NewCurrency64(290e3)) != 0 {
		t.Error("storage proof output was created with the wrong value")
	}
	if cst.cs.siafundPool.Cmp(types.NewCurrency64(10e3)) != 0 {
		t.Error("siafund pool was not correctly updated when applying a storage proof")
	}

	// Apply a transaction with 2 storage proofs.
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{
			{ParentID: fcid1},
			{ParentID: fcid2},
		},
	}
	cst.cs.applyStorageProofs(bn, txn)
	_, exists = cst.cs.fileContracts[fcid1]
	if exists {
		t.Error("Storage proof failed to consume file contract.")
	}
	_, exists = cst.cs.fileContracts[fcid2]
	if exists {
		t.Error("storage proof did not consume file contract")
	}
	if len(cst.cs.fileContracts) != 0 {
		t.Error("file contracts not correctly updated")
	}
	if len(bn.fileContractDiffs) != 6 {
		t.Error("block node was not updated correctly")
	}
	spoid1 := fcid1.StorageProofOutputID(types.ProofValid, 0)
	_, exists = cst.cs.siacoinOutputs[spoid1]
	if exists {
		t.Error("output created when file contract had no corresponding output")
	}
	spoid2 := fcid2.StorageProofOutputID(types.ProofValid, 0)
	sco, exists = cst.cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][spoid2]
	if !exists {
		t.Error("no output created by first output of file contract")
	}
	if sco.Value.Cmp(types.NewCurrency64(280e3)) != 0 {
		t.Error("first siacoin output created has wrong value")
	}
	spoid3 := fcid2.StorageProofOutputID(types.ProofValid, 1)
	sco, exists = cst.cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][spoid3]
	if !exists {
		t.Error("second output not created for storage proof")
	}
	if sco.Value.Cmp(types.NewCurrency64(300e3)) != 0 {
		t.Error("second siacoin output has wrong value")
	}
	if cst.cs.siafundPool.Cmp(types.NewCurrency64(30e3)) != 0 {
		t.Error("siafund pool not being added up correctly")
	}
}

// TestNonexistentStorageProof applies a storage proof which points to a
// nonextentent parent.
func TestNonexistentStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestNonexistentStorageProof")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	bn := new(blockNode)

	// Trigger a panic by applying a storage proof for a nonexistent file
	// contract.
	defer func() {
		r := recover()
		if r != ErrNonexistentStorageProof {
			t.Error("no panic occured when misusing applySiacoinInput")
		}
	}()
	txn := types.Transaction{
		StorageProofs: []types.StorageProof{{}},
	}
	cst.cs.applyStorageProofs(bn, txn)
}

// TestDuplicateStorageProof applies a storage proof which has already been
// applied.
func TestDuplicateStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestDuplicateStorageProof")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node.
	bn := new(blockNode)
	bn.height = cst.cs.height()

	// Create a file contract for the storage proof to prove.
	txn0 := types.Transaction{
		FileContracts: []types.FileContract{
			{
				Payout: types.NewCurrency64(300e3),
				ValidProofOutputs: []types.SiacoinOutput{
					{Value: types.NewCurrency64(290e3)},
				},
			},
		},
	}
	cst.cs.applyFileContracts(bn, txn0)
	fcid := txn0.FileContractID(0)

	// Apply a single storage proof.
	txn1 := types.Transaction{
		StorageProofs: []types.StorageProof{{ParentID: fcid}},
	}
	cst.cs.applyStorageProofs(bn, txn1)

	// Trigger a panic by applying the storage proof again.
	defer func() {
		r := recover()
		if r != ErrDuplicateValidProofOutput {
			t.Error("failed to trigger ErrDuplicateValidProofOutput:", r)
		}
	}()
	cst.cs.applyFileContracts(bn, txn0) // File contract was consumed by the first proof.
	cst.cs.applyStorageProofs(bn, txn1)
}

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

	// Create a block node to use with application.
	bn := new(blockNode)
	bn.height = cst.cs.height()

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiafundOutputID
	for id, _ := range cst.cs.siafundOutputs {
		ids = append(ids, id)
	}

	// Apply a transaction with a single siafund input.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{
			{ParentID: ids[0]},
		},
	}
	cst.cs.applySiafundInputs(bn, txn)
	_, exists := cst.cs.siafundOutputs[ids[0]]
	if exists {
		t.Error("Failed to conusme a siafund output")
	}
	if len(cst.cs.siafundOutputs) != 1 {
		t.Error("siafund outputs not correctly updated", len(cst.cs.siafundOutputs))
	}
	if len(bn.siafundOutputDiffs) != 1 {
		t.Error("block node was not updated for single transaction")
	}
	if bn.siafundOutputDiffs[0].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when consuming a siafund output")
	}
	if bn.siafundOutputDiffs[0].ID != ids[0] {
		t.Error("wrong id used when consuming a siafund output")
	}
	if len(cst.cs.delayedSiacoinOutputs[cst.cs.height()+types.MaturityDelay]) != 2 { // 1 for a block subsidy, 1 for the siafund claim.
		t.Error("siafund claim was not created")
	}
}

// TestMisuseApplySiafundInputs misuses applySiafundInputs and checks that a
// panic was triggered.
func TestMisuseApplySiafundInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplySiacoinInput")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	bn := new(blockNode)
	bn.height = cst.cs.height()

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiafundOutputID
	for id, _ := range cst.cs.siafundOutputs {
		ids = append(ids, id)
	}

	// Apply a transaction with a single siafund input.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{
			{ParentID: ids[0]},
		},
	}
	cst.cs.applySiafundInputs(bn, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplySiafundInput {
			t.Error("no panic occured when misusing applySiacoinInput")
			t.Error(r)
		}
	}()
	cst.cs.applySiafundInputs(bn, txn)
}

// TestApplySiafundOutputs probes the applySiafundOutputs method of the
// consensus set.
func TestApplySiafundOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplySiafundOutputs")
	if err != nil {
		t.Fatal(err)
	}
	cst.cs.siafundPool = types.NewCurrency64(101)

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
	if len(cst.cs.siafundOutputs) != 3 {
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
	if len(cst.cs.siafundOutputs) != 5 {
		t.Error("siafund outputs not correctly updated")
	}
	if len(bn.siafundOutputDiffs) != 3 {
		t.Error("block node was not updated for single element transaction")
	}
}

// TestMisuseApplySiafundOutputs misuses applySiafundOutputs and checks that a
// panic was triggered.
func TestMisuseApplySiafundOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMisuseApplySiafundOutputs")
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
