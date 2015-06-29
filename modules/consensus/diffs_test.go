package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestSiacoinOutputDiff applies and reverts a siacoin output diff, then
// triggers an inconsistency panic.
func TestCommitSiacoinOutputDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitSiacoinOutputDiff")
	if err != nil {
		t.Fatal(err)
	}

	// Commit a siacoin output diff.
	initialScosLen := len(cst.cs.siacoinOutputs)
	id := types.SiacoinOutputID{'1'}
	sco := types.SiacoinOutput{Value: types.NewCurrency64(1)}
	scod := modules.SiacoinOutputDiff{
		Direction:     modules.DiffApply,
		ID:            id,
		SiacoinOutput: sco,
	}
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	if len(cst.cs.siacoinOutputs) != initialScosLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.siacoinOutputs[id].Value.Cmp(sco.Value) != 0 {
		t.Error("wrong siacoin output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffRevert)
	if len(cst.cs.siacoinOutputs) != initialScosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	_, exists := cst.cs.siacoinOutputs[id]
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	scod.Direction = modules.DiffRevert
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	if len(cst.cs.siacoinOutputs) != initialScosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	_, exists = cst.cs.siacoinOutputs[id]
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffRevert)
	if len(cst.cs.siacoinOutputs) != initialScosLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.siacoinOutputs[id].Value.Cmp(sco.Value) != 0 {
		t.Error("wrong siacoin output value after committing a diff")
	}

	// Trigger an inconsistency check.
	defer func() {
		r := recover()
		if r != errBadCommitSiacoinOutputDiff {
			t.Error("expecting errBadCommitSiacoinOutputDiff, got", r)
		}
	}()
	// Try reverting a revert diff that was already reverted. (add an object
	// that already exists)
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffRevert)
}

// TestCommitFileContracttDiff applies and reverts a file contract diff, then
// triggers an inconsistency panic.
func TestCommitFileContractDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitFileContractDiff")
	if err != nil {
		t.Fatal(err)
	}

	// Commit a file contract diff.
	initialFcsLen := len(cst.cs.fileContracts)
	id := types.FileContractID{'1'}
	fc := types.FileContract{Payout: types.NewCurrency64(1)}
	fcd := modules.FileContractDiff{
		Direction:    modules.DiffApply,
		ID:           id,
		FileContract: fc,
	}
	cst.cs.commitFileContractDiff(fcd, modules.DiffApply)
	if len(cst.cs.fileContracts) != initialFcsLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.fileContracts[id].Payout.Cmp(fc.Payout) != 0 {
		t.Error("wrong siacoin output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitFileContractDiff(fcd, modules.DiffRevert)
	if len(cst.cs.fileContracts) != initialFcsLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	_, exists := cst.cs.fileContracts[id]
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitFileContractDiff(fcd, modules.DiffApply)
	fcd.Direction = modules.DiffRevert
	cst.cs.commitFileContractDiff(fcd, modules.DiffApply)
	if len(cst.cs.fileContracts) != initialFcsLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	_, exists = cst.cs.fileContracts[id]
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitFileContractDiff(fcd, modules.DiffRevert)
	if len(cst.cs.fileContracts) != initialFcsLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.fileContracts[id].Payout.Cmp(fc.Payout) != 0 {
		t.Error("wrong siacoin output value after committing a diff")
	}

	// Trigger an inconsistency check.
	defer func() {
		r := recover()
		if r != errBadCommitFileContractDiff {
			t.Error("expecting errBadCommitFileContractDiff, got", r)
		}
	}()
	// Try reverting an apply diff that was already reverted. (remove an object
	// that was already removed)
	fcd.Direction = modules.DiffApply                      // Object currently exists, but make the direction 'apply'.
	cst.cs.commitFileContractDiff(fcd, modules.DiffRevert) // revert the application.
	cst.cs.commitFileContractDiff(fcd, modules.DiffRevert) // revert the application again, in error.
}

// TestSiafundOutputDiff applies and reverts a siafund output diff, then
// triggers an inconsistency panic.
func TestCommitSiafundOutputDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitSiafundOutputDiff")
	if err != nil {
		t.Fatal(err)
	}

	// Commit a siafund output diff.
	initialScosLen := len(cst.cs.siafundOutputs)
	id := types.SiafundOutputID{'1'}
	sfo := types.SiafundOutput{Value: types.NewCurrency64(1)}
	sfod := modules.SiafundOutputDiff{
		Direction:     modules.DiffApply,
		ID:            id,
		SiafundOutput: sfo,
	}
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	if len(cst.cs.siafundOutputs) != initialScosLen+1 {
		t.Error("siafund output diff set did not increase in size")
	}
	if cst.cs.siafundOutputs[id].Value.Cmp(sfo.Value) != 0 {
		t.Error("wrong siafund output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffRevert)
	if len(cst.cs.siafundOutputs) != initialScosLen {
		t.Error("siafund output diff set did not increase in size")
	}
	_, exists := cst.cs.siafundOutputs[id]
	if exists {
		t.Error("siafund output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	sfod.Direction = modules.DiffRevert
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	if len(cst.cs.siafundOutputs) != initialScosLen {
		t.Error("siafund output diff set did not increase in size")
	}
	_, exists = cst.cs.siafundOutputs[id]
	if exists {
		t.Error("siafund output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffRevert)
	if len(cst.cs.siafundOutputs) != initialScosLen+1 {
		t.Error("siafund output diff set did not increase in size")
	}
	if cst.cs.siafundOutputs[id].Value.Cmp(sfo.Value) != 0 {
		t.Error("wrong siafund output value after committing a diff")
	}

	// Trigger an inconsistency check.
	defer func() {
		r := recover()
		if r != errBadCommitSiafundOutputDiff {
			t.Error("expecting errBadCommitSiafundOutputDiff, got", r)
		}
	}()
	// Try applying a revert diff that was already applied. (remove an object
	// that was already removed)
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply) // Remove the object.
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply) // Remove the object again.
}

// TODO: Repeat the above test for delayed diffs
// TODO: Repeat the above test for pool diffs
// NOTE: Try all variations of the 'exists == (scod == dir)' clause throughout the testing.
