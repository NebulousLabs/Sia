package consensus

import (
	"testing"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"

	"github.com/coreos/bbolt"
)

// TestCommitDelayedSiacoinOutputDiffBadMaturity commits a delayed siacoin
// output that has a bad maturity height and triggers a panic.
func TestCommitDelayedSiacoinOutputDiffBadMaturity(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Trigger an inconsistency check.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expecting error after corrupting database")
		}
	}()

	// Commit a delayed siacoin output with maturity height = cs.height()+1
	maturityHeight := cst.cs.dbBlockHeight() - 1
	id := types.SiacoinOutputID{'1'}
	dsco := types.SiacoinOutput{Value: types.NewCurrency64(1)}
	dscod := modules.DelayedSiacoinOutputDiff{
		Direction:      modules.DiffApply,
		ID:             id,
		SiacoinOutput:  dsco,
		MaturityHeight: maturityHeight,
	}
	_ = cst.cs.db.Update(func(tx *bolt.Tx) error {
		commitDelayedSiacoinOutputDiff(tx, dscod, modules.DiffApply)
		return nil
	})
}

// TestCommitNodeDiffs probes the commitNodeDiffs method of the consensus set.
/*
func TestCommitNodeDiffs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	pb := cst.cs.dbCurrentProcessedBlock()
	_ = cst.cs.db.Update(func(tx *bolt.Tx) error {
		commitDiffSet(tx, pb, modules.DiffRevert) // pull the block node out of the consensus set.
		return nil
	})

	// For diffs that can be destroyed in the same block they are created,
	// create diffs that do just that. This has in the past caused issues upon
	// rewinding.
	scoid := types.SiacoinOutputID{'1'}
	scod0 := modules.SiacoinOutputDiff{
		Direction: modules.DiffApply,
		ID:        scoid,
	}
	scod1 := modules.SiacoinOutputDiff{
		Direction: modules.DiffRevert,
		ID:        scoid,
	}
	fcid := types.FileContractID{'2'}
	fcd0 := modules.FileContractDiff{
		Direction: modules.DiffApply,
		ID:        fcid,
	}
	fcd1 := modules.FileContractDiff{
		Direction: modules.DiffRevert,
		ID:        fcid,
	}
	sfoid := types.SiafundOutputID{'3'}
	sfod0 := modules.SiafundOutputDiff{
		Direction: modules.DiffApply,
		ID:        sfoid,
	}
	sfod1 := modules.SiafundOutputDiff{
		Direction: modules.DiffRevert,
		ID:        sfoid,
	}
	dscoid := types.SiacoinOutputID{'4'}
	dscod := modules.DelayedSiacoinOutputDiff{
		Direction:      modules.DiffApply,
		ID:             dscoid,
		MaturityHeight: cst.cs.dbBlockHeight() + types.MaturityDelay,
	}
	var siafundPool types.Currency
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		siafundPool = getSiafundPool(tx)
		return nil
	})
	if err != nil {
		panic(err)
	}
	sfpd := modules.SiafundPoolDiff{
		Direction: modules.DiffApply,
		Previous:  siafundPool,
		Adjusted:  siafundPool.Add(types.NewCurrency64(1)),
	}
	pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod0)
	pb.SiacoinOutputDiffs = append(pb.SiacoinOutputDiffs, scod1)
	pb.FileContractDiffs = append(pb.FileContractDiffs, fcd0)
	pb.FileContractDiffs = append(pb.FileContractDiffs, fcd1)
	pb.SiafundOutputDiffs = append(pb.SiafundOutputDiffs, sfod0)
	pb.SiafundOutputDiffs = append(pb.SiafundOutputDiffs, sfod1)
	pb.DelayedSiacoinOutputDiffs = append(pb.DelayedSiacoinOutputDiffs, dscod)
	pb.SiafundPoolDiffs = append(pb.SiafundPoolDiffs, sfpd)
	_ = cst.cs.db.Update(func(tx *bolt.Tx) error {
		createUpcomingDelayedOutputMaps(tx, pb, modules.DiffApply)
		return nil
	})
	_ = cst.cs.db.Update(func(tx *bolt.Tx) error {
		commitNodeDiffs(tx, pb, modules.DiffApply)
		return nil
	})
	exists := cst.cs.db.inSiacoinOutputs(scoid)
	if exists {
		t.Error("intradependent outputs not treated correctly")
	}
	exists = cst.cs.db.inFileContracts(fcid)
	if exists {
		t.Error("intradependent outputs not treated correctly")
	}
	exists = cst.cs.db.inSiafundOutputs(sfoid)
	if exists {
		t.Error("intradependent outputs not treated correctly")
	}
	_ = cst.cs.db.Update(func(tx *bolt.Tx) error {
		commitNodeDiffs(tx, pb, modules.DiffRevert)
		return nil
	})
	exists = cst.cs.db.inSiacoinOutputs(scoid)
	if exists {
		t.Error("intradependent outputs not treated correctly")
	}
	exists = cst.cs.db.inFileContracts(fcid)
	if exists {
		t.Error("intradependent outputs not treated correctly")
	}
	exists = cst.cs.db.inSiafundOutputs(sfoid)
	if exists {
		t.Error("intradependent outputs not treated correctly")
	}
}
*/

/*
// TestSiacoinOutputDiff applies and reverts a siacoin output diff, then
// triggers an inconsistency panic.
func TestCommitSiacoinOutputDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Commit a siacoin output diff.
	initialScosLen := cst.cs.db.lenSiacoinOutputs()
	id := types.SiacoinOutputID{'1'}
	sco := types.SiacoinOutput{Value: types.NewCurrency64(1)}
	scod := modules.SiacoinOutputDiff{
		Direction:     modules.DiffApply,
		ID:            id,
		SiacoinOutput: sco,
	}
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	if cst.cs.db.lenSiacoinOutputs() != initialScosLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.db.getSiacoinOutputs(id).Value.Cmp(sco.Value) != 0 {
		t.Error("wrong siacoin output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffRevert)
	if cst.cs.db.lenSiacoinOutputs() != initialScosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	exists := cst.cs.db.inSiacoinOutputs(id)
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	scod.Direction = modules.DiffRevert
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffApply)
	if cst.cs.db.lenSiacoinOutputs() != initialScosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	exists = cst.cs.db.inSiacoinOutputs(id)
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitSiacoinOutputDiff(scod, modules.DiffRevert)
	if cst.cs.db.lenSiacoinOutputs() != initialScosLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.db.getSiacoinOutputs(id).Value.Cmp(sco.Value) != 0 {
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
*/

/*
// TestCommitFileContracttDiff applies and reverts a file contract diff, then
// triggers an inconsistency panic.
func TestCommitFileContractDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Commit a file contract diff.
	initialFcsLen := cst.cs.db.lenFileContracts()
	id := types.FileContractID{'1'}
	fc := types.FileContract{Payout: types.NewCurrency64(1)}
	fcd := modules.FileContractDiff{
		Direction:    modules.DiffApply,
		ID:           id,
		FileContract: fc,
	}
	cst.cs.commitFileContractDiff(fcd, modules.DiffApply)
	if cst.cs.db.lenFileContracts() != initialFcsLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.db.getFileContracts(id).Payout.Cmp(fc.Payout) != 0 {
		t.Error("wrong siacoin output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitFileContractDiff(fcd, modules.DiffRevert)
	if cst.cs.db.lenFileContracts() != initialFcsLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	exists := cst.cs.db.inFileContracts(id)
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitFileContractDiff(fcd, modules.DiffApply)
	fcd.Direction = modules.DiffRevert
	cst.cs.commitFileContractDiff(fcd, modules.DiffApply)
	if cst.cs.db.lenFileContracts() != initialFcsLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	exists = cst.cs.db.inFileContracts(id)
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitFileContractDiff(fcd, modules.DiffRevert)
	if cst.cs.db.lenFileContracts() != initialFcsLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.db.getFileContracts(id).Payout.Cmp(fc.Payout) != 0 {
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
*/

// TestSiafundOutputDiff applies and reverts a siafund output diff, then
// triggers an inconsistency panic.
/*
func TestCommitSiafundOutputDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Commit a siafund output diff.
	initialScosLen := cst.cs.db.lenSiafundOutputs()
	id := types.SiafundOutputID{'1'}
	sfo := types.SiafundOutput{Value: types.NewCurrency64(1)}
	sfod := modules.SiafundOutputDiff{
		Direction:     modules.DiffApply,
		ID:            id,
		SiafundOutput: sfo,
	}
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	if cst.cs.db.lenSiafundOutputs() != initialScosLen+1 {
		t.Error("siafund output diff set did not increase in size")
	}
	sfo1 := cst.cs.db.getSiafundOutputs(id)
	if sfo1.Value.Cmp(sfo.Value) != 0 {
		t.Error("wrong siafund output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffRevert)
	if cst.cs.db.lenSiafundOutputs() != initialScosLen {
		t.Error("siafund output diff set did not increase in size")
	}
	exists := cst.cs.db.inSiafundOutputs(id)
	if exists {
		t.Error("siafund output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	sfod.Direction = modules.DiffRevert
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
	if cst.cs.db.lenSiafundOutputs() != initialScosLen {
		t.Error("siafund output diff set did not increase in size")
	}
	exists = cst.cs.db.inSiafundOutputs(id)
	if exists {
		t.Error("siafund output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitSiafundOutputDiff(sfod, modules.DiffRevert)
	if cst.cs.db.lenSiafundOutputs() != initialScosLen+1 {
		t.Error("siafund output diff set did not increase in size")
	}
	sfo2 := cst.cs.db.getSiafundOutputs(id)
	if sfo2.Value.Cmp(sfo.Value) != 0 {
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
*/

// TestCommitDelayedSiacoinOutputDiff probes the commitDelayedSiacoinOutputDiff
// method of the consensus set.
/*
func TestCommitDelayedSiacoinOutputDiff(t *testing.T) {
	t.Skip("test isn't working, but checks the wrong code anyway")
	if testing.Short() {
		t.Skip()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Commit a delayed siacoin output with maturity height = cs.height()+1
	maturityHeight := cst.cs.height() + 1
	initialDscosLen := cst.cs.db.lenDelayedSiacoinOutputsHeight(maturityHeight)
	id := types.SiacoinOutputID{'1'}
	dsco := types.SiacoinOutput{Value: types.NewCurrency64(1)}
	dscod := modules.DelayedSiacoinOutputDiff{
		Direction:      modules.DiffApply,
		ID:             id,
		SiacoinOutput:  dsco,
		MaturityHeight: maturityHeight,
	}
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	if cst.cs.db.lenDelayedSiacoinOutputsHeight(maturityHeight) != initialDscosLen+1 {
		t.Fatal("delayed output diff set did not increase in size")
	}
	if cst.cs.db.getDelayedSiacoinOutputs(maturityHeight, id).Value.Cmp(dsco.Value) != 0 {
		t.Error("wrong delayed siacoin output value after committing a diff")
	}

	// Rewind the diff.
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffRevert)
	if cst.cs.db.lenDelayedSiacoinOutputsHeight(maturityHeight) != initialDscosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	exists := cst.cs.db.inDelayedSiacoinOutputsHeight(maturityHeight, id)
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Restore the diff and then apply the inverse diff.
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	dscod.Direction = modules.DiffRevert
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffApply)
	if cst.cs.db.lenDelayedSiacoinOutputsHeight(maturityHeight) != initialDscosLen {
		t.Error("siacoin output diff set did not increase in size")
	}
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(maturityHeight, id)
	if exists {
		t.Error("siacoin output was not reverted")
	}

	// Revert the inverse diff.
	cst.cs.commitDelayedSiacoinOutputDiff(dscod, modules.DiffRevert)
	if cst.cs.db.lenDelayedSiacoinOutputsHeight(maturityHeight) != initialDscosLen+1 {
		t.Error("siacoin output diff set did not increase in size")
	}
	if cst.cs.db.getDelayedSiacoinOutputs(maturityHeight, id).Value.Cmp(dsco.Value) != 0 {
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
*/

/*
// TestCommitSiafundPoolDiff probes the commitSiafundPoolDiff method of the
// consensus set.
func TestCommitSiafundPoolDiff(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
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
		Direction: modules.DiffApply,
		Previous:  initial,
		Adjusted:  adjusted1,
	}
	sfpd2 := modules.SiafundPoolDiff{
		Direction: modules.DiffApply,
		Previous:  adjusted1,
		Adjusted:  adjusted2,
	}
	sfpd3 := modules.SiafundPoolDiff{
		Direction: modules.DiffApply,
		Previous:  adjusted2,
		Adjusted:  adjusted3,
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
		if r != errNonApplySiafundPoolDiff {
			t.Error(r)
		}
		cst.cs.commitSiafundPoolDiff(sfpd1, modules.DiffRevert)
	}()
	defer func() {
		r := recover()
		if r != errNegativePoolAdjustment {
			t.Error("expecting errNegativePoolAdjustment, got", r)
		}
		sfpd2.Direction = modules.DiffRevert
		cst.cs.commitSiafundPoolDiff(sfpd2, modules.DiffApply)
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
*/

/*
// TestDeleteObsoleteDelayedOutputMapsSanity probes the sanity checks of the
// deleteObsoleteDelayedOutputMaps method of the consensus set.
func TestDeleteObsoleteDelayedOutputMapsSanity(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	pb := cst.cs.currentProcessedBlock()
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		return commitDiffSet(tx, pb, modules.DiffRevert)
	})
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expecting an error after corrupting the database")
		}
	}()
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expecting an error after corrupting the database")
		}

		// Trigger a panic by deleting a map with outputs in it during revert.
		err = cst.cs.db.Update(func(tx *bolt.Tx) error {
			return createUpcomingDelayedOutputMaps(tx, pb, modules.DiffApply)
		})
		if err != nil {
			t.Fatal(err)
		}
		err = cst.cs.db.Update(func(tx *bolt.Tx) error {
			return commitNodeDiffs(tx, pb, modules.DiffApply)
		})
		if err != nil {
			t.Fatal(err)
		}
		err = cst.cs.db.Update(func(tx *bolt.Tx) error {
			return deleteObsoleteDelayedOutputMaps(tx, pb, modules.DiffRevert)
		})
		if err != nil {
			t.Fatal(err)
		}
	}()

	// Trigger a panic by deleting a map with outputs in it during apply.
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		return deleteObsoleteDelayedOutputMaps(tx, pb, modules.DiffApply)
	})
	if err != nil {
		t.Fatal(err)
	}
}
*/

/*
// TestGenerateAndApplyDiffSanity triggers the sanity checks in the
// generateAndApplyDiff method of the consensus set.
func TestGenerateAndApplyDiffSanity(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	pb := cst.cs.currentProcessedBlock()
	cst.cs.commitDiffSet(pb, modules.DiffRevert)

	defer func() {
		r := recover()
		if r != errRegenerateDiffs {
			t.Error("expected errRegenerateDiffs, got", r)
		}
	}()
	defer func() {
		r := recover()
		if r != errInvalidSuccessor {
			t.Error("expected errInvalidSuccessor, got", r)
		}

		// Trigger errRegenerteDiffs
		_ = cst.cs.generateAndApplyDiff(pb)
	}()

	// Trigger errInvalidSuccessor
	parent := cst.cs.db.getBlockMap(pb.Parent)
	parent.DiffsGenerated = false
	_ = cst.cs.generateAndApplyDiff(parent)
}
*/
