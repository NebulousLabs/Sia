package consensus

/*
import (
	"testing"

	"github.com/coreos/bbolt"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestApplyMinerPayouts probes the applyMinerPayouts method of the consensus
// set.
func TestApplyMinerPayouts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node with a single miner payout.
	b := new(database.Block)
	b.Height = cst.cs.dbBlockHeight()
	b.Block.Timestamp = 2 // MinerPayout id is determined by block id + index; add uniqueness to the block id.
	b.Block.MinerPayouts = append(b.Block.MinerPayouts, types.SiacoinOutput{Value: types.NewCurrency64(12)})
	mpid0 := b.Block.MinerPayoutID(0)

	// Apply the single miner payout.
	_ = cst.cs.db.Update(func(tx database.Tx) error {
		applyMinerPayouts(tx, b)
		return nil
	})
	exists := cst.cs.db.inDelayedSiacoinOutputsHeight(cst.cs.dbBlockHeight()+types.MaturityDelay, mpid0)
	if !exists {
		t.Error("miner payout was not created in the delayed outputs set")
	}
	dsco, err := cst.cs.dbGetDSCO(cst.cs.dbBlockHeight()+types.MaturityDelay, mpid0)
	if err != nil {
		t.Fatal(err)
	}
	if dsco.Value.Cmp64(12) != 0 {
		t.Error("miner payout created with wrong currency value")
	}
	exists = cst.cs.db.inSiacoinOutputs(mpid0)
	if exists {
		t.Error("miner payout was added to the siacoin output set")
	}
	if cst.cs.db.lenDelayedSiacoinOutputsHeight(cst.cs.dbBlockHeight()+types.MaturityDelay) != 2 { // 1 for consensus set creation, 1 for the output that just got added.
		t.Error("wrong number of delayed siacoin outputs in consensus set")
	}
	if len(b.DelayedSiacoinOutputDiffs) != 1 {
		t.Fatal("block node did not get the delayed siacoin output diff")
	}
	if b.DelayedSiacoinOutputDiffs[0].Direction != modules.DiffApply {
		t.Error("delayed siacoin output diff has the wrong direction")
	}
	if b.DelayedSiacoinOutputDiffs[0].ID != mpid0 {
		t.Error("delayed siacoin output diff has wrong id")
	}

	// Apply a processed block with two miner payouts.
	b2 := new(database.Block)
	b2.Height = cst.cs.dbBlockHeight()
	b2.Block.Timestamp = 5 // MinerPayout id is determined by block id + index; add uniqueness to the block id.
	b2.Block.MinerPayouts = []types.SiacoinOutput{
		{Value: types.NewCurrency64(5)},
		{Value: types.NewCurrency64(10)},
	}
	mpid1 := b2.Block.MinerPayoutID(0)
	mpid2 := b2.Block.MinerPayoutID(1)
	_ = cst.cs.db.Update(func(tx database.Tx) error {
		applyMinerPayouts(tx, b2)
		return nil
	})
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(cst.cs.dbBlockHeight()+types.MaturityDelay, mpid1)
	if !exists {
		t.Error("delayed siacoin output was not created")
	}
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(cst.cs.dbBlockHeight()+types.MaturityDelay, mpid2)
	if !exists {
		t.Error("delayed siacoin output was not created")
	}
	if len(b2.DelayedSiacoinOutputDiffs) != 2 {
		t.Error("block node should have 2 delayed outputs")
	}

	// Trigger a panic where the miner payouts have already been applied.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expecting error after corrupting database")
		}
	}()
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expecting error after corrupting database")
		}
		cst.cs.db.rmDelayedSiacoinOutputsHeight(b.Height+types.MaturityDelay, mpid0)
		cst.cs.db.addSiacoinOutputs(mpid0, types.SiacoinOutput{})
		_ = cst.cs.db.Update(func(tx database.Tx) error {
			applyMinerPayouts(tx, b)
			return nil
		})
	}()
	_ = cst.cs.db.Update(func(tx database.Tx) error {
		applyMinerPayouts(tx, b)
		return nil
	})
}

// TestApplyMaturedSiacoinOutputs probes the applyMaturedSiacoinOutputs method
// of the consensus set.
func TestApplyMaturedSiacoinOutputs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	b := cst.cs.dbCurrentProcessedBlock()

	// Trigger the sanity check concerning already-matured outputs.
	defer func() {
		r := recover()
		if r != errOutputAlreadyMature {
			t.Error(r)
		}
	}()
	cst.cs.db.addSiacoinOutputs(types.SiacoinOutputID{}, types.SiacoinOutput{})
	_ = cst.cs.db.Update(func(tx database.Tx) error {
		createDSCOBucket(tx, b.Height)
		return nil
	})
	cst.cs.db.addDelayedSiacoinOutputsHeight(b.Height, types.SiacoinOutputID{}, types.SiacoinOutput{})
	_ = cst.cs.db.Update(func(tx database.Tx) error {
		applyMaturedSiacoinOutputs(tx, b)
		return nil
	})
}

// TestApplyMissedStorageProof probes the applyMissedStorageProof method of the
// consensus set.
func TestApplyMissedStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node.
	b := new(database.Block)
	b.Height = cst.cs.height()

	// Create a file contract that's expiring and has 1 missed proof output.
	expiringFC := types.FileContract{
		Payout:             types.NewCurrency64(300e3),
		WindowEnd:          b.Height,
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(290e3)}},
	}
	// Assign the contract a 0-id.
	cst.cs.db.addFileContracts(types.FileContractID{}, expiringFC)
	cst.cs.db.addFCExpirations(b.Height)
	cst.cs.db.addFCExpirationsHeight(b.Height, types.FileContractID{})
	cst.cs.applyMissedStorageProof(b, types.FileContractID{})
	exists := cst.cs.db.inFileContracts(types.FileContractID{})
	if exists {
		t.Error("file contract was not consumed in missed storage proof")
	}
	spoid := types.FileContractID{}.StorageProofOutputID(types.ProofMissed, 0)
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(b.Height+types.MaturityDelay, spoid)
	if !exists {
		t.Error("missed proof output was never created")
	}
	exists = cst.cs.db.inSiacoinOutputs(spoid)
	if exists {
		t.Error("storage proof output made it into the siacoin output set")
	}
	exists = cst.cs.db.inFileContracts(types.FileContractID{})
	if exists {
		t.Error("file contract remains after expiration")
	}

	// Trigger the debug panics.
	// not exist.
	defer func() {
		r := recover()
		if r != errNilItem {
			t.Error(r)
		}
	}()
	defer func() {
		r := recover()
		if r != errNilItem {
			t.Error(r)
		}
		// Trigger errMissingFileContract
		cst.cs.applyMissedStorageProof(b, types.FileContractID(spoid))
	}()
	defer func() {
		r := recover()
		if r != errNilItem {
			t.Error(r)
		}

		// Trigger errStorageProofTiming
		expiringFC.WindowEnd = 0
		cst.cs.applyMissedStorageProof(b, types.FileContractID{})
	}()
	defer func() {
		r := recover()
		if r != errNilItem {
			t.Error(r)
		}

		// Trigger errPayoutsAlreadyPaid from siacoin outputs.
		cst.cs.db.rmDelayedSiacoinOutputsHeight(b.Height+types.MaturityDelay, spoid)
		cst.cs.db.addSiacoinOutputs(spoid, types.SiacoinOutput{})
		cst.cs.applyMissedStorageProof(b, types.FileContractID{})
	}()
	// Trigger errPayoutsAlreadyPaid from delayed outputs.
	cst.cs.db.rmFileContracts(types.FileContractID{})
	cst.cs.db.addFileContracts(types.FileContractID{}, expiringFC)
	cst.cs.db.addDelayedSiacoinOutputsHeight(b.Height+types.MaturityDelay, spoid, types.SiacoinOutput{})
	cst.cs.applyMissedStorageProof(b, types.FileContractID{})
}
*/

// TestApplyFileContractMaintenance probes the applyFileContractMaintenance
// method of the consensus set.
/*
func TestApplyFileContractMaintenance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block node.
	b := new(database.Block)
	b.Height = cst.cs.height()

	// Create a file contract that's expiring and has 1 missed proof output.
	expiringFC := types.FileContract{
		Payout:             types.NewCurrency64(300e3),
		WindowEnd:          b.Height,
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(290e3)}},
	}
	// Assign the contract a 0-id.
	cst.cs.db.addFileContracts(types.FileContractID{}, expiringFC)
	cst.cs.db.addFCExpirations(b.Height)
	cst.cs.db.addFCExpirationsHeight(b.Height, types.FileContractID{})
	err = cst.cs.db.Update(func(tx database.Tx) error {
		applyFileContractMaintenance(tx, b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	exists := cst.cs.db.inFileContracts(types.FileContractID{})
	if exists {
		t.Error("file contract was not consumed in missed storage proof")
	}
	spoid := types.FileContractID{}.StorageProofOutputID(types.ProofMissed, 0)
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(b.Height+types.MaturityDelay, spoid)
	if !exists {
		t.Error("missed proof output was never created")
	}
	exists = cst.cs.db.inSiacoinOutputs(spoid)
	if exists {
		t.Error("storage proof output made it into the siacoin output set")
	}
	exists = cst.cs.db.inFileContracts(types.FileContractID{})
	if exists {
		t.Error("file contract remains after expiration")
	}
}
*/
