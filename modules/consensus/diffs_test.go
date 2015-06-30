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

// TestCommitDelayedSiacoinOutputDiff probes the commitDelayedSiacoinOutputDiff
// method of the consensus set.
func TestCommitDelayedSiacoinOutputDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitDelayedSiacoinOutputDiff")
	if err != nil {
		t.Fatal(err)
	}

	// Commit a delayed siacoin output with maturity height = cs.height()+1
	maturityHeight := cst.cs.height() + 1
	initialDscosLen := len(cst.cs.delayedSiacoinOutputs[maturityHeight])
	id := types.SiacoinOutputID{'1'}
	dsco := types.SiacoinOutput{Value: types.NewCurrency64(1)}
	dscod := modules.DelayedSiacoinOutputDiff{
		Direction:      modules.DiffApply,
		ID:             id,
		SiacoinOutput:  dsco,
		MaturityHeight: maturityHeight,
	}
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	if len(cst.cs.delayedSiacoinOutputs[maturityHeight]) != initialDscosLen+1 {
		t.Error("delayed output diff set did not increase in size")
	}
	if cst.cs.delayedSiacoinOutputs[maturityHeight][id].Value.Cmp(dsco.Value) != 0 {
		t.Error("wrong delayed siacoin output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffRevert)
	if len(cst.cs.delayedSiacoinOutputs[maturityHeight]) != initialDscosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	_, exists := cst.cs.delayedSiacoinOutputs[maturityHeight][id]
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	dscod.Direction = modules.DiffRevert
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	if len(cst.cs.delayedSiacoinOutputs[maturityHeight]) != initialDscosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	_, exists = cst.cs.delayedSiacoinOutputs[maturityHeight][id]
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffRevert)
	if len(cst.cs.delayedSiacoinOutputs[maturityHeight]) != initialDscosLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.delayedSiacoinOutputs[maturityHeight][id].Value.Cmp(dsco.Value) != 0 {
		t.Error("wrong siacoin output value after committing a diff")
	}

	// Trigger an inconsistency check.
	defer func() {
		r := recover()
		if r != errBadCommitDelayedSiacoinOutputDiff {
			t.Error("expecting errBadCommitDelayedSiacoinOutputDiff, got", r)
		}
	}()
	// Try applying an apply diff that was already applied. (add an object
	// that already exists)
	dscod.Direction = modules.DiffApply                             // set the direction to apply
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply) // apply an already existing delayed output.
}

// TestCommitDelayedSiacoinOutputDiffBadMaturity commits a delayed sicoin
// output that has a bad maturity height and triggers a panic.
func TestCommitDelayedSiacoinOutputDiffBadMaturity(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitDelayedSiacoinOutputDiff")
	if err != nil {
		t.Fatal(err)
	}

	// Trigger an inconsistency check.
	defer func() {
		r := recover()
		if r != errBadMaturityHeight {
			t.Error("expecting errBadMaturityHeight, got", r)
		}
	}()

	// Commit a delayed siacoin output with maturity height = cs.height()+1
	maturityHeight := cst.cs.height() - 1
	id := types.SiacoinOutputID{'1'}
	dsco := types.SiacoinOutput{Value: types.NewCurrency64(1)}
	dscod := modules.DelayedSiacoinOutputDiff{
		Direction:      modules.DiffApply,
		ID:             id,
		SiacoinOutput:  dsco,
		MaturityHeight: maturityHeight,
	}
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
}

// TestCommitSiafundPoolDiff probes the commitSiafundPoolDiff method of the
// consensus set.
func TestCommitSiafundPoolDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitDelayedSiacoinOutputDiff")
	if err != nil {
		t.Fatal(err)
	}

	// Apply two siafund pool diffs, and then a diff with 0 change. Then revert
	// them all.
	initial := cst.cs.siafundPool
	adjusted1 := initial.Add(types.NewCurrency64(200))
	adjusted2 := adjusted1.Add(types.NewCurrency64(500))
	adjusted3 := adjusted2.Add(types.NewCurrency64(0))
	sfpd1 := modules.SiafundPoolDiff{
		Previous: initial,
		Adjusted: adjusted1,
	}
	sfpd2 := modules.SiafundPoolDiff{
		Previous: adjusted1,
		Adjusted: adjusted2,
	}
	sfpd3 := modules.SiafundPoolDiff{
		Previous: adjusted2,
		Adjusted: adjusted3,
	}
	cst.cs.commitSiafundPoolDiff(sfpd1, modules.DiffApply)
	if cst.cs.siafundPool.Cmp(adjusted1) != 0 {
		t.Error("siafund pool was not adjusted correctly")
	}
	cst.cs.commitSiafundPoolDiff(sfpd2, modules.DiffApply)
	if cst.cs.siafundPool.Cmp(adjusted2) != 0 {
		t.Error("second siafund pool adjustment was flawed")
	}
	cst.cs.commitSiafundPoolDiff(sfpd3, modules.DiffApply)
	if cst.cs.siafundPool.Cmp(adjusted3) != 0 {
		t.Error("second siafund pool adjustment was flawed")
	}
	cst.cs.commitSiafundPoolDiff(sfpd3, modules.DiffRevert)
	if cst.cs.siafundPool.Cmp(adjusted2) != 0 {
		t.Error("reverting second adjustment was flawed")
	}
	cst.cs.commitSiafundPoolDiff(sfpd2, modules.DiffRevert)
	if cst.cs.siafundPool.Cmp(adjusted1) != 0 {
		t.Error("reverting second adjustment was flawed")
	}
	cst.cs.commitSiafundPoolDiff(sfpd1, modules.DiffRevert)
	if cst.cs.siafundPool.Cmp(initial) != 0 {
		t.Error("reverting first adjustment was flawed")
	}

	// Do a chaining set of panics. First apply a negative pool adjustment,
	// then revert the pool diffs in the wrong order, than apply the pool diffs
	// in the wrong order.
	defer func() {
		r := recover()
		if r != errApplySiafundPoolDiffMismatch {
			t.Error("expecting errApplySiafundPoolDiffMismatch, got", r)
		}
	}()
	defer func() {
		r := recover()
		if r != errRevertSiafundPoolDiffMismatch {
			t.Error("expecting errRevertSiafundPoolDiffMismatch, got", r)
		}
		cst.cs.commitSiafundPoolDiff(sfpd1, modules.DiffApply)
	}()
	defer func() {
		r := recover()
		if r != errNegativePoolAdjustment {
			t.Error("expecting errNegativePoolAdjustment, got", r)
		}
		cst.cs.commitSiafundPoolDiff(sfpd1, modules.DiffRevert)
	}()
	cst.cs.commitSiafundPoolDiff(sfpd1, modules.DiffApply)
	cst.cs.commitSiafundPoolDiff(sfpd2, modules.DiffApply)
	negativeAdjustment := adjusted2.Sub(types.NewCurrency64(100))
	negativeSfpd := modules.SiafundPoolDiff{
		Previous: adjusted3,
		Adjusted: negativeAdjustment,
	}
	cst.cs.commitSiafundPoolDiff(negativeSfpd, modules.DiffApply)
}

// TestCommitDiffSetSanity triggers all of the panics in the
// commitDiffSetSanity method of the consensus set.
func TestCommitDiffSetSanity(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitDelayedSiacoinOutputDiff")
	if err != nil {
		t.Fatal(err)
	}
	bn := cst.cs.currentBlockNode()

	defer func() {
		r := recover()
		if r != errDiffsNotGenerated {
			t.Error("expected errDiffsNotGenerated, got", r)
		}
	}()
	defer func() {
		r := recover()
		if r != errWrongAppliedDiffSet {
			t.Error("expected errWrongAppliedDiffSet, got", r)
		}

		// Trigger a panic about diffs not being generated.
		bn.diffsGenerated = false
		cst.cs.commitDiffSetSanity(bn, modules.DiffRevert)
	}()
	defer func() {
		r := recover()
		if r != errWrongRevertDiffSet {
			t.Error("expected errWrongRevertDiffSet, got", r)
		}

		// trigger a panic about applying the wrong block.
		bn.block.ParentID[0]++
		cst.cs.commitDiffSetSanity(bn, modules.DiffApply)
	}()

	// Trigger a panic about incorrectly reverting a diff set.
	bn.block.MinerPayouts = append(bn.block.MinerPayouts, types.SiacoinOutput{}) // change the block id by adding a miner payout
	cst.cs.commitDiffSetSanity(bn, modules.DiffRevert)
}

// TestCreateUpcomingDelayedOutputMaps probes the createUpcomingDelayedMaps
// method of the consensus set.
func TestCreateUpcomingDelayedOutputMaps(t *testing.T) {
	if testing.Short() {
		// t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestCommitDelayedSiacoinOutputDiff")
	if err != nil {
		t.Fatal(err)
	}
	bn := cst.cs.currentBlockNode()

	// Check that a map gets created upon revert.
	_, exists := cst.cs.delayedSiacoinOutputs[bn.height]
	if exists {
		t.Fatal("unexpected delayed output map at bn.height")
	}
	cst.cs.commitDiffSet(bn, modules.DiffRevert) // revert the current block node
	_, exists = cst.cs.delayedSiacoinOutputs[bn.height]
	if !exists {
		t.Error("delayed output map was not created when reverting diffs")
	}

	// Check that a map gets created on apply.
	_, exists = cst.cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay]
	if exists {
		t.Fatal("delayed output map exists when it shouldn't")
	}
	cst.cs.createUpcomingDelayedOutputMaps(bn, modules.DiffApply)
	_, exists = cst.cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay]
	if !exists {
		t.Error("delayed output map was not created")
	}

	// Check that a map is not created on revert when the height is
	// sufficiently low.
	cst.cs.commitDiffSet(bn.parent, modules.DiffRevert)
	cst.cs.commitDiffSet(bn.parent.parent, modules.DiffRevert)
	_, exists = cst.cs.delayedSiacoinOutputs[bn.parent.parent.height]
	if exists {
		t.Error("delayed output map was created when bringing the height too low")
	}

	defer func() {
		r := recover()
		if r != errCreatingExistingUpcomingMap {
			t.Error("expected errCreatingExistingUpcomingMap, got", r)
		}
	}()
	defer func() {
		r := recover()
		if r != errCreatingExistingUpcomingMap {
			t.Error("expected errCreatingExistingUpcomingMap, got", r)
		}

		// Trigger a panic by creating a map that's already there during a revert.
		cst.cs.createUpcomingDelayedOutputMaps(bn, modules.DiffRevert)
	}()

	// Trigger a panic by creating a map that's already there during an apply.
	cst.cs.createUpcomingDelayedOutputMaps(bn, modules.DiffApply)
}
