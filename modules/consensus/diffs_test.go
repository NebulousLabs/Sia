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
		// t.SkipNow()
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

// TODO: Repeat the above test for file contract diffs
// TODO: Repeat the above test for siafund diffs
// TODO: Repeat the above test for delayed diffs
// TODO: Repeat the above test for pool diffs
// NOTE: Try all variations of the 'exists == (scod == dir)' clause throughout the testing.
