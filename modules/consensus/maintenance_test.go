package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestApplyMinerPayouts probes the applyMinerPayouts method of the consensus
// set.
func TestApplyMinerPayouts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyMinerPayouts")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node with a single miner payout.
	bn := new(blockNode)
	bn.height = cst.cs.height()
	bn.block.Timestamp = 2 // MinerPayout id is determined by block id + index; add uniqueness to the block id.
	bn.block.MinerPayouts = append(bn.block.MinerPayouts, types.SiacoinOutput{Value: types.NewCurrency64(12)})
	mpid0 := bn.block.MinerPayoutID(0)

	// Apply the single miner payout.
	cst.cs.applyMinerPayouts(bn)
	dsco, exists := cst.cs.delayedSiacoinOutputs[cst.cs.height()+types.MaturityDelay][mpid0]
	if !exists {
		t.Error("miner payout was not created in the delayed outputs set")
	}
	if dsco.Value.Cmp(types.NewCurrency64(12)) != 0 {
		t.Error("miner payout created with wrong currency value")
	}
	_, exists = cst.cs.siacoinOutputs[mpid0]
	if exists {
		t.Error("miner payout was added to the siacoin output set")
	}
	if len(cst.cs.delayedSiacoinOutputs[cst.cs.height()+types.MaturityDelay]) != 2 { // 1 for consensus set creation, 1 for the output that just got added.
		t.Error("wrong number of delayed siacoin outputs in consensus set")
	}
	if len(bn.delayedSiacoinOutputDiffs) != 1 {
		t.Fatal("block node did not get the delayed siacoin output diff")
	}
	if bn.delayedSiacoinOutputDiffs[0].Direction != modules.DiffApply {
		t.Error("delayed siacoin output diff has the wrong direction")
	}
	if bn.delayedSiacoinOutputDiffs[0].ID != mpid0 {
		t.Error("delayed siacoin output diff has wrong id")
	}

	// Apply a block node with two miner payouts.
	bn2 := new(blockNode)
	bn2.height = cst.cs.height()
	bn2.block.Timestamp = 5 // MinerPayout id is determined by block id + index; add uniqueness to the block id.
	bn2.block.MinerPayouts = []types.SiacoinOutput{
		{Value: types.NewCurrency64(5)},
		{Value: types.NewCurrency64(10)},
	}
	mpid1 := bn2.block.MinerPayoutID(0)
	mpid2 := bn2.block.MinerPayoutID(1)
	cst.cs.applyMinerPayouts(bn2)
	_, exists = cst.cs.delayedSiacoinOutputs[cst.cs.height()+types.MaturityDelay][mpid1]
	if !exists {
		t.Error("delayed siacoin output was not created")
	}
	_, exists = cst.cs.delayedSiacoinOutputs[cst.cs.height()+types.MaturityDelay][mpid2]
	if !exists {
		t.Error("delayed siacoin output was not created")
	}
	if len(bn2.delayedSiacoinOutputDiffs) != 2 {
		t.Error("block node should have 2 delayed outputs")
	}
}

// TestApplyMissedStorageProof probes the applyMissedStorageProof method of the
// consensus set.
func TestApplyMissedStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyMissedStorageProof")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node.
	bn := new(blockNode)
	bn.height = cst.cs.height()

	// Create a file contract that's expiring and has 1 missed proof output.
	expiringFC := types.FileContract{
		Payout:             types.NewCurrency64(300e3),
		WindowEnd:          bn.height,
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(290e3)}},
	}
	cst.cs.fileContracts[types.FileContractID{}] = expiringFC // assign the contract a 0-id.
	cst.cs.applyMissedStorageProof(bn, types.FileContractID{})
	_, exists := cst.cs.fileContracts[types.FileContractID{}]
	if exists {
		t.Error("file contract was not consumed in missed storage proof")
	}
	spoid := types.FileContractID{}.StorageProofOutputID(types.ProofMissed, 0)
	_, exists = cst.cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][spoid]
	if !exists {
		t.Error("missed proof output was never created")
	}
	_, exists = cst.cs.siacoinOutputs[spoid]
	if exists {
		t.Error("storage proof output made it into the siacoin output set")
	}
	if cst.cs.siafundPool.Cmp(types.NewCurrency64(10e3)) != 0 {
		t.Error("siafund pool not updated!")
	}
	_, exists = cst.cs.fileContracts[types.FileContractID{}]
	if exists {
		t.Error("file contract remains after expiration")
	}
}

// TestApplyFileContractMaintenance probes the applyFileContractMaintenance
// method of the consensus set.
func TestApplyFileContractMaintenance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestApplyMissedStorageProof")
	if err != nil {
		t.Fatal(err)
	}

	// Create a block node.
	bn := new(blockNode)
	bn.height = cst.cs.height()

	// Create a file contract that's expiring and has 1 missed proof output.
	expiringFC := types.FileContract{
		Payout:             types.NewCurrency64(300e3),
		WindowEnd:          bn.height,
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(290e3)}},
	}
	cst.cs.fileContracts[types.FileContractID{}] = expiringFC // assign the contract a 0-id.
	cst.cs.applyFileContractMaintenance(bn)
	_, exists := cst.cs.fileContracts[types.FileContractID{}]
	if exists {
		t.Error("file contract was not consumed in missed storage proof")
	}
	spoid := types.FileContractID{}.StorageProofOutputID(types.ProofMissed, 0)
	_, exists = cst.cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay][spoid]
	if !exists {
		t.Error("missed proof output was never created")
	}
	_, exists = cst.cs.siacoinOutputs[spoid]
	if exists {
		t.Error("storage proof output made it into the siacoin output set")
	}
	if cst.cs.siafundPool.Cmp(types.NewCurrency64(10e3)) != 0 {
		t.Error("siafund pool not updated!")
	}
	_, exists = cst.cs.fileContracts[types.FileContractID{}]
	if exists {
		t.Error("file contract remains after expiration")
	}
}
