package explorer

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// TestImmediateBlockFacts grabs the block facts object from the block explorer
// at the current height and verifies that the data has been filled out.
func TestImmediateBlockFacts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	et, err := createExplorerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	facts := et.explorer.LatestBlockFacts()
	var explorerHeight types.BlockHeight
	err = et.explorer.db.View(dbGetInternal(internalBlockHeight, &explorerHeight))
	if err != nil {
		t.Fatal(err)
	}
	if facts.Height != explorerHeight || explorerHeight == 0 {
		t.Error("wrong height reported in facts object")
	}
	if !facts.TotalCoins.Equals(types.CalculateNumSiacoins(et.cs.Height())) {
		t.Error("wrong number of total coins:", facts.TotalCoins, et.cs.Height())
	}
}

// TestBlock probes the Block function of the explorer.
func TestBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	et, err := createExplorerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	gb := types.GenesisBlock
	gbFetch, height, exists := et.explorer.Block(gb.ID())
	if !exists || height != 0 || gbFetch.ID() != gb.ID() {
		t.Error("call to 'Block' inside explorer failed")
	}
}

// TestBlockFacts checks that the correct block facts are returned for a query.
func TestBlockFacts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	et, err := createExplorerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	gb := types.GenesisBlock
	bf, exists := et.explorer.BlockFacts(0)
	if !exists || bf.BlockID != gb.ID() || bf.Height != 0 {
		t.Error("call to 'BlockFacts' inside explorer failed")
		t.Error("Expecting true ->", exists)
		t.Error("Expecting", gb.ID(), "->", bf.BlockID)
		t.Error("Expecting 0 ->", bf.Height)
	}

	bf, exists = et.explorer.BlockFacts(1)
	if !exists || bf.Height != 1 {
		t.Error("call to 'BlockFacts' has failed")
	}
}

// TestFileContractPayouts checks that file contract outputs are tracked by the explorer
func TestFileContractPayoutsMissingProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	et, err := createExplorerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create and fund valid file contracts.
	builder, err := et.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	payout := types.NewCurrency64(1e9)
	err = builder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}

	windowStart := et.cs.Height() + 2
	windowEnd := et.cs.Height() + 5

	fc := types.FileContract{
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		Payout:             payout,
		ValidProofOutputs:  []types.SiacoinOutput{{Value: types.PostTax(et.cs.Height(), payout)}},
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.PostTax(et.cs.Height(), payout)}},
		UnlockHash:         types.UnlockConditions{}.UnlockHash(),
	}

	fcIndex := builder.AddFileContract(fc)
	tSet, err := builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	if err != nil {
		t.Fatal(err)
	}

	err = et.tpool.AcceptTransactionSet(tSet)
	if err != nil {
		t.Fatal(err)
	}

	// Mine until contract payout is in consensus
	for i := et.cs.Height(); i < windowEnd+types.MaturityDelay; i++ {
		_, err := et.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	ti := len(tSet) - 1
	fcid := tSet[ti].FileContractID(fcIndex)
	txns := et.explorer.FileContractID(fcid)
	if len(txns) == 0 {
		t.Error("Filecontract ID does not appear in blockchain")
	}

	outputs, err := et.explorer.FileContractPayouts(fcid)
	if err != nil {
		t.Fatal(err)
	}

	// Check if MissedProofOutputs were added to spendable outputs
	if len(outputs) != len(fc.MissedProofOutputs) {
		t.Error("Incorrect number of outputs returned")
		t.Error("Expecting -> ", len(fc.MissedProofOutputs))
		t.Error("But was -> ", len(outputs))
	}
}

func TestFileContractsPayoutValidProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	et, err := createExplorerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// COMPATv0.4.0 - Step the block height up past the hardfork amount. This
	// code stops nondeterministic failures when producing storage proofs that
	// is related to buggy old code.
	for et.cs.Height() <= 10 {
		_, err := et.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create a file (as a bytes.Buffer) that will be used for the file
	// contract.
	filesize := uint64(4e3)
	file := fastrand.Bytes(int(filesize))
	merkleRoot := crypto.MerkleRoot(file)

	// Create a funded file contract
	payout := types.NewCurrency64(400e6)
	fc := types.FileContract{
		FileSize:           filesize,
		FileMerkleRoot:     merkleRoot,
		WindowStart:        et.cs.Height() + 1,
		WindowEnd:          et.cs.Height() + 2,
		Payout:             payout,
		ValidProofOutputs:  []types.SiacoinOutput{{Value: types.PostTax(et.cs.Height(), payout)}},
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.PostTax(et.cs.Height(), payout)}},
	}

	// Submit a transaction with the file contract.
	//oldSiafundPool := cst.cs.dbGetSiafundPool()
	builder, err := et.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	err = builder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}

	fcIndex := builder.AddFileContract(fc)
	tSet, err := builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}

	err = et.tpool.AcceptTransactionSet(tSet)
	if err != nil {
		t.Fatal(err)
	}
	_, err = et.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	ti := len(tSet) - 1
	fcid := tSet[ti].FileContractID(fcIndex)

	// Create and submit a storage proof for the file contract.
	segmentIndex, err := et.cs.StorageProofSegment(fcid)
	if err != nil {
		t.Fatal(err)
	}
	segment, hashSet := crypto.MerkleProof(file, segmentIndex)
	sp := types.StorageProof{
		ParentID: fcid,
		HashSet:  hashSet,
	}
	copy(sp.Segment[:], segment)
	builder, err = et.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	builder.AddStorageProof(sp)
	tSet, err = builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = et.tpool.AcceptTransactionSet(tSet)
	if err != nil {
		t.Fatal(err)
	}

	// Mine until contract payout is in consensus
	for i := types.BlockHeight(0); i < types.MaturityDelay+1; i++ {
		_, err := et.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	txns := et.explorer.FileContractID(fcid)
	if len(txns) == 0 {
		t.Error("Filecontract ID does not appear in blockchain")
	}

	// Check that the storageproof was added to the explorer after
	// the filecontract was removed from the consensus set
	outputs, err := et.explorer.FileContractPayouts(fcid)
	if err != nil {
		t.Fatal(err)
	}

	if len(outputs) != len(fc.ValidProofOutputs) {
		t.Errorf("expected %v, got %v ", fc.MissedProofOutputs, outputs)
	}
}
