package consensus

/*
import (
	"testing"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"
)

// TestApplySiacoinInputs probes the applySiacoinInputs method of the consensus
// set.
func TestApplySiacoinInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a consensus set and get it to 3 siacoin outputs. The consensus
	// set starts with 2 siacoin outputs, mining a block will add another.
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	b, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiacoinOutputID
	cst.cs.db.forEachSiacoinOutputs(func(id types.SiacoinOutputID, sco types.SiacoinOutput) {
		ids = append(ids, id)
	})

	// Apply a transaction with a single siacoin input.
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			{ParentID: ids[0]},
		},
	}
	cst.cs.applySiacoinInputs(pb, txn)
	exists := cst.cs.db.inSiacoinOutputs(ids[0])
	if exists {
		t.Error("Failed to conusme a siacoin output")
	}
	if cst.cs.db.lenSiacoinOutputs() != 2 {
		t.Error("siacoin outputs not correctly updated")
	}
	if len(pb.SiacoinOutputDiffs) != 1 {
		t.Error("block node was not updated for single transaction")
	}
	if pb.SiacoinOutputDiffs[0].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when consuming a siacoin output")
	}
	if pb.SiacoinOutputDiffs[0].ID != ids[0] {
		t.Error("wrong id used when consuming a siacoin output")
	}

	// Apply a transaction with two siacoin inputs.
	txn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			{ParentID: ids[1]},
			{ParentID: ids[2]},
		},
	}
	cst.cs.applySiacoinInputs(pb, txn)
	if cst.cs.db.lenSiacoinOutputs() != 0 {
		t.Error("failed to consume all siacoin outputs in the consensus set")
	}
	if len(pb.SiacoinOutputDiffs) != 3 {
		t.Error("processed block was not updated for single transaction")
	}
}

// TestMisuseApplySiacoinInputs misuses applySiacoinInput and checks that a
// panic was triggered.
func TestMisuseApplySiacoinInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiacoinOutputID
	cst.cs.db.forEachSiacoinOutputs(func(id types.SiacoinOutputID, sco types.SiacoinOutput) {
		ids = append(ids, id)
	})

	// Apply a transaction with a single siacoin input.
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			{ParentID: ids[0]},
		},
	}
	cst.cs.applySiacoinInputs(pb, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expecting error after corrupting database")
		}
	}()
	cst.cs.applySiacoinInputs(pb, txn)
}

// TestApplySiacoinOutputs probes the applySiacoinOutput method of the
// consensus set.
func TestApplySiacoinOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Apply a transaction with a single siacoin output.
	txn := types.Transaction{
		SiacoinOutputs: []types.SiacoinOutput{{}},
	}
	cst.cs.applySiacoinOutputs(pb, txn)
	scoid := txn.SiacoinOutputID(0)
	exists := cst.cs.db.inSiacoinOutputs(scoid)
	if !exists {
		t.Error("Failed to create siacoin output")
	}
	if cst.cs.db.lenSiacoinOutputs() != 3 { // 3 because createConsensusSetTester has 2 initially.
		t.Error("siacoin outputs not correctly updated")
	}
	if len(pb.SiacoinOutputDiffs) != 1 {
		t.Error("block node was not updated for single element transaction")
	}
	if pb.SiacoinOutputDiffs[0].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when creating a siacoin output")
	}
	if pb.SiacoinOutputDiffs[0].ID != scoid {
		t.Error("wrong id used when creating a siacoin output")
	}

	// Apply a transaction with 2 siacoin outputs.
	txn = types.Transaction{
		SiacoinOutputs: []types.SiacoinOutput{
			{Value: types.NewCurrency64(1)},
			{Value: types.NewCurrency64(2)},
		},
	}
	cst.cs.applySiacoinOutputs(pb, txn)
	scoid0 := txn.SiacoinOutputID(0)
	scoid1 := txn.SiacoinOutputID(1)
	exists = cst.cs.db.inSiacoinOutputs(scoid0)
	if !exists {
		t.Error("Failed to create siacoin output")
	}
	exists = cst.cs.db.inSiacoinOutputs(scoid1)
	if !exists {
		t.Error("Failed to create siacoin output")
	}
	if cst.cs.db.lenSiacoinOutputs() != 5 { // 5 because createConsensusSetTester has 2 initially.
		t.Error("siacoin outputs not correctly updated")
	}
	if len(pb.SiacoinOutputDiffs) != 3 {
		t.Error("block node was not updated correctly")
	}
}

// TestMisuseApplySiacoinOutputs misuses applySiacoinOutputs and checks that a
// panic was triggered.
func TestMisuseApplySiacoinOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Apply a transaction with a single siacoin output.
	txn := types.Transaction{
		SiacoinOutputs: []types.SiacoinOutput{{}},
	}
	cst.cs.applySiacoinOutputs(pb, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occurred when misusing applySiacoinInput")
		}
	}()
	cst.cs.applySiacoinOutputs(pb, txn)
}

// TestApplyFileContracts probes the applyFileContracts method of the
// consensus set.
func TestApplyFileContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Apply a transaction with a single file contract.
	txn := types.Transaction{
		FileContracts: []types.FileContract{{}},
	}
	cst.cs.applyFileContracts(pb, txn)
	fcid := txn.FileContractID(0)
	exists := cst.cs.db.inFileContracts(fcid)
	if !exists {
		t.Error("Failed to create file contract")
	}
	if cst.cs.db.lenFileContracts() != 1 {
		t.Error("file contracts not correctly updated")
	}
	if len(pb.FileContractDiffs) != 1 {
		t.Error("block node was not updated for single element transaction")
	}
	if pb.FileContractDiffs[0].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when creating a file contract")
	}
	if pb.FileContractDiffs[0].ID != fcid {
		t.Error("wrong id used when creating a file contract")
	}

	// Apply a transaction with 2 file contracts.
	txn = types.Transaction{
		FileContracts: []types.FileContract{
			{Payout: types.NewCurrency64(1)},
			{Payout: types.NewCurrency64(300e3)},
		},
	}
	cst.cs.applyFileContracts(pb, txn)
	fcid0 := txn.FileContractID(0)
	fcid1 := txn.FileContractID(1)
	exists = cst.cs.db.inFileContracts(fcid0)
	if !exists {
		t.Error("Failed to create file contract")
	}
	exists = cst.cs.db.inFileContracts(fcid1)
	if !exists {
		t.Error("Failed to create file contract")
	}
	if cst.cs.db.lenFileContracts() != 3 {
		t.Error("file contracts not correctly updated")
	}
	if len(pb.FileContractDiffs) != 3 {
		t.Error("block node was not updated correctly")
	}
	if cst.cs.siafundPool.Cmp64(10e3) != 0 {
		t.Error("siafund pool did not update correctly upon creation of a file contract")
	}
}

// TestMisuseApplyFileContracts misuses applyFileContracts and checks that a
// panic was triggered.
func TestMisuseApplyFileContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Apply a transaction with a single file contract.
	txn := types.Transaction{
		FileContracts: []types.FileContract{{}},
	}
	cst.cs.applyFileContracts(pb, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("no panic occurred when misusing applySiacoinInput")
		}
	}()
	cst.cs.applyFileContracts(pb, txn)
}

// TestApplyFileContractRevisions probes the applyFileContractRevisions method
// of the consensus set.
func TestApplyFileContractRevisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Apply a transaction with two file contracts - that way there is
	// something to revise.
	txn := types.Transaction{
		FileContracts: []types.FileContract{
			{},
			{Payout: types.NewCurrency64(1)},
		},
	}
	cst.cs.applyFileContracts(pb, txn)
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
	cst.cs.applyFileContractRevisions(pb, txn)
	exists := cst.cs.db.inFileContracts(fcid0)
	if !exists {
		t.Error("Revision killed a file contract")
	}
	fc := cst.cs.db.getFileContracts(fcid0)
	if fc.FileSize != 1 {
		t.Error("file contract filesize not properly updated")
	}
	if cst.cs.db.lenFileContracts() != 2 {
		t.Error("file contracts not correctly updated")
	}
	if len(pb.FileContractDiffs) != 4 { // 2 creating the initial contracts, 1 to remove the old, 1 to add the revision.
		t.Error("block node was not updated for single element transaction")
	}
	if pb.FileContractDiffs[2].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when revising a file contract")
	}
	if pb.FileContractDiffs[3].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when revising a file contract")
	}
	if pb.FileContractDiffs[2].ID != fcid0 {
		t.Error("wrong id used when revising a file contract")
	}
	if pb.FileContractDiffs[3].ID != fcid0 {
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
	cst.cs.applyFileContractRevisions(pb, txn)
	exists = cst.cs.db.inFileContracts(fcid0)
	if !exists {
		t.Error("Revision ate file contract")
	}
	fc0 := cst.cs.db.getFileContracts(fcid0)
	exists = cst.cs.db.inFileContracts(fcid1)
	if !exists {
		t.Error("Revision ate file contract")
	}
	fc1 := cst.cs.db.getFileContracts(fcid1)
	if fc0.FileSize != 2 {
		t.Error("Revision not correctly applied")
	}
	if fc1.FileSize != 3 {
		t.Error("Revision not correctly applied")
	}
	if cst.cs.db.lenFileContracts() != 2 {
		t.Error("file contracts not correctly updated")
	}
	if len(pb.FileContractDiffs) != 8 {
		t.Error("block node was not updated correctly")
	}
}

// TestMisuseApplyFileContractRevisions misuses applyFileContractRevisions and
// checks that a panic was triggered.
func TestMisuseApplyFileContractRevisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Trigger a panic from revising a nonexistent file contract.
	defer func() {
		r := recover()
		if r != errNilItem {
			t.Error("no panic occurred when misusing applySiacoinInput")
		}
	}()
	txn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{}},
	}
	cst.cs.applyFileContractRevisions(pb, txn)
}

// TestApplyStorageProofs probes the applyStorageProofs method of the consensus
// set.
func TestApplyStorageProofs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)
	pb.Height = cst.cs.height()

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
	cst.cs.applyFileContracts(pb, txn)
	fcid0 := txn.FileContractID(0)
	fcid1 := txn.FileContractID(1)
	fcid2 := txn.FileContractID(2)

	// Apply a single storage proof.
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{{ParentID: fcid0}},
	}
	cst.cs.applyStorageProofs(pb, txn)
	exists := cst.cs.db.inFileContracts(fcid0)
	if exists {
		t.Error("Storage proof did not disable a file contract.")
	}
	if cst.cs.db.lenFileContracts() != 2 {
		t.Error("file contracts not correctly updated")
	}
	if len(pb.FileContractDiffs) != 4 { // 3 creating the initial contracts, 1 for the storage proof.
		t.Error("block node was not updated for single element transaction")
	}
	if pb.FileContractDiffs[3].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when revising a file contract")
	}
	if pb.FileContractDiffs[3].ID != fcid0 {
		t.Error("wrong id used when revising a file contract")
	}
	spoid0 := fcid0.StorageProofOutputID(types.ProofValid, 0)
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(pb.Height+types.MaturityDelay, spoid0)
	if !exists {
		t.Error("storage proof output not created after applying a storage proof")
	}
	sco := cst.cs.db.getDelayedSiacoinOutputs(pb.Height+types.MaturityDelay, spoid0)
	if sco.Value.Cmp64(290e3) != 0 {
		t.Error("storage proof output was created with the wrong value")
	}

	// Apply a transaction with 2 storage proofs.
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{
			{ParentID: fcid1},
			{ParentID: fcid2},
		},
	}
	cst.cs.applyStorageProofs(pb, txn)
	exists = cst.cs.db.inFileContracts(fcid1)
	if exists {
		t.Error("Storage proof failed to consume file contract.")
	}
	exists = cst.cs.db.inFileContracts(fcid2)
	if exists {
		t.Error("storage proof did not consume file contract")
	}
	if cst.cs.db.lenFileContracts() != 0 {
		t.Error("file contracts not correctly updated")
	}
	if len(pb.FileContractDiffs) != 6 {
		t.Error("block node was not updated correctly")
	}
	spoid1 := fcid1.StorageProofOutputID(types.ProofValid, 0)
	exists = cst.cs.db.inSiacoinOutputs(spoid1)
	if exists {
		t.Error("output created when file contract had no corresponding output")
	}
	spoid2 := fcid2.StorageProofOutputID(types.ProofValid, 0)
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(pb.Height+types.MaturityDelay, spoid2)
	if !exists {
		t.Error("no output created by first output of file contract")
	}
	sco = cst.cs.db.getDelayedSiacoinOutputs(pb.Height+types.MaturityDelay, spoid2)
	if sco.Value.Cmp64(280e3) != 0 {
		t.Error("first siacoin output created has wrong value")
	}
	spoid3 := fcid2.StorageProofOutputID(types.ProofValid, 1)
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(pb.Height+types.MaturityDelay, spoid3)
	if !exists {
		t.Error("second output not created for storage proof")
	}
	sco = cst.cs.db.getDelayedSiacoinOutputs(pb.Height+types.MaturityDelay, spoid3)
	if sco.Value.Cmp64(300e3) != 0 {
		t.Error("second siacoin output has wrong value")
	}
	if cst.cs.siafundPool.Cmp64(30e3) != 0 {
		t.Error("siafund pool not being added up correctly")
	}
}

// TestNonexistentStorageProof applies a storage proof which points to a
// nonextentent parent.
func TestNonexistentStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Trigger a panic by applying a storage proof for a nonexistent file
	// contract.
	defer func() {
		r := recover()
		if r != errNilItem {
			t.Error("no panic occurred when misusing applySiacoinInput")
		}
	}()
	txn := types.Transaction{
		StorageProofs: []types.StorageProof{{}},
	}
	cst.cs.applyStorageProofs(pb, txn)
}

// TestDuplicateStorageProof applies a storage proof which has already been
// applied.
func TestDuplicateStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node.
	pb := new(processedBlock)
	pb.Height = cst.cs.height()

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
	cst.cs.applyFileContracts(pb, txn0)
	fcid := txn0.FileContractID(0)

	// Apply a single storage proof.
	txn1 := types.Transaction{
		StorageProofs: []types.StorageProof{{ParentID: fcid}},
	}
	cst.cs.applyStorageProofs(pb, txn1)

	// Trigger a panic by applying the storage proof again.
	defer func() {
		r := recover()
		if r != ErrDuplicateValidProofOutput {
			t.Error("failed to trigger ErrDuplicateValidProofOutput:", r)
		}
	}()
	cst.cs.applyFileContracts(pb, txn0) // File contract was consumed by the first proof.
	cst.cs.applyStorageProofs(pb, txn1)
}

// TestApplySiafundInputs probes the applySiafundInputs method of the consensus
// set.
func TestApplySiafundInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)
	pb.Height = cst.cs.height()

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiafundOutputID
	cst.cs.db.forEachSiafundOutputs(func(sfoid types.SiafundOutputID, sfo types.SiafundOutput) {
		ids = append(ids, sfoid)
	})

	// Apply a transaction with a single siafund input.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{
			{ParentID: ids[0]},
		},
	}
	cst.cs.applySiafundInputs(pb, txn)
	exists := cst.cs.db.inSiafundOutputs(ids[0])
	if exists {
		t.Error("Failed to conusme a siafund output")
	}
	if cst.cs.db.lenSiafundOutputs() != 2 {
		t.Error("siafund outputs not correctly updated", cst.cs.db.lenSiafundOutputs())
	}
	if len(pb.SiafundOutputDiffs) != 1 {
		t.Error("block node was not updated for single transaction")
	}
	if pb.SiafundOutputDiffs[0].Direction != modules.DiffRevert {
		t.Error("wrong diff direction applied when consuming a siafund output")
	}
	if pb.SiafundOutputDiffs[0].ID != ids[0] {
		t.Error("wrong id used when consuming a siafund output")
	}
	if cst.cs.db.lenDelayedSiacoinOutputsHeight(cst.cs.height()+types.MaturityDelay) != 2 { // 1 for a block subsidy, 1 for the siafund claim.
		t.Error("siafund claim was not created")
	}
}

// TestMisuseApplySiafundInputs misuses applySiafundInputs and checks that a
// panic was triggered.
func TestMisuseApplySiafundInputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)
	pb.Height = cst.cs.height()

	// Fetch the output id's of each siacoin output in the consensus set.
	var ids []types.SiafundOutputID
	cst.cs.db.forEachSiafundOutputs(func(sfoid types.SiafundOutputID, sfo types.SiafundOutput) {
		ids = append(ids, sfoid)
	})

	// Apply a transaction with a single siafund input.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{
			{ParentID: ids[0]},
		},
	}
	cst.cs.applySiafundInputs(pb, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplySiafundInput {
			t.Error("no panic occurred when misusing applySiacoinInput")
			t.Error(r)
		}
	}()
	cst.cs.applySiafundInputs(pb, txn)
}

// TestApplySiafundOutputs probes the applySiafundOutputs method of the
// consensus set.
func TestApplySiafundOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	cst.cs.siafundPool = types.NewCurrency64(101)

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Apply a transaction with a single siafund output.
	txn := types.Transaction{
		SiafundOutputs: []types.SiafundOutput{{}},
	}
	cst.cs.applySiafundOutputs(pb, txn)
	sfoid := txn.SiafundOutputID(0)
	exists := cst.cs.db.inSiafundOutputs(sfoid)
	if !exists {
		t.Error("Failed to create siafund output")
	}
	if cst.cs.db.lenSiafundOutputs() != 4 {
		t.Error("siafund outputs not correctly updated")
	}
	if len(pb.SiafundOutputDiffs) != 1 {
		t.Error("block node was not updated for single element transaction")
	}
	if pb.SiafundOutputDiffs[0].Direction != modules.DiffApply {
		t.Error("wrong diff direction applied when creating a siafund output")
	}
	if pb.SiafundOutputDiffs[0].ID != sfoid {
		t.Error("wrong id used when creating a siafund output")
	}
	if pb.SiafundOutputDiffs[0].SiafundOutput.ClaimStart.Cmp64(101) != 0 {
		t.Error("claim start set incorrectly when creating a siafund output")
	}

	// Apply a transaction with 2 siacoin outputs.
	txn = types.Transaction{
		SiafundOutputs: []types.SiafundOutput{
			{Value: types.NewCurrency64(1)},
			{Value: types.NewCurrency64(2)},
		},
	}
	cst.cs.applySiafundOutputs(pb, txn)
	sfoid0 := txn.SiafundOutputID(0)
	sfoid1 := txn.SiafundOutputID(1)
	exists = cst.cs.db.inSiafundOutputs(sfoid0)
	if !exists {
		t.Error("Failed to create siafund output")
	}
	exists = cst.cs.db.inSiafundOutputs(sfoid1)
	if !exists {
		t.Error("Failed to create siafund output")
	}
	if cst.cs.db.lenSiafundOutputs() != 6 {
		t.Error("siafund outputs not correctly updated")
	}
	if len(pb.SiafundOutputDiffs) != 3 {
		t.Error("block node was not updated for single element transaction")
	}
}

// TestMisuseApplySiafundOutputs misuses applySiafundOutputs and checks that a
// panic was triggered.
func TestMisuseApplySiafundOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node to use with application.
	pb := new(processedBlock)

	// Apply a transaction with a single siacoin output.
	txn := types.Transaction{
		SiafundOutputs: []types.SiafundOutput{{}},
	}
	cst.cs.applySiafundOutputs(pb, txn)

	// Trigger the panic that occurs when an output is applied incorrectly, and
	// perform a catch to read the error that is created.
	defer func() {
		r := recover()
		if r != ErrMisuseApplySiafundOutput {
			t.Error("no panic occurred when misusing applySiafundInput")
		}
	}()
	cst.cs.applySiafundOutputs(pb, txn)
}
*/
