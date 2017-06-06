package explorer

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
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
func TestFileContractPayouts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	et, err := createExplorerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create and fund valid file contracts.
	builder := et.wallet.StartTransaction()
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

	index := builder.AddFileContract(fc)
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

	// Mine until contract payouts is in consensus
	for i := et.cs.Height(); i < windowEnd+types.MaturityDelay; i++ {
		b, _ := et.miner.FindBlock()
		err = et.cs.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}

	fcid := tSet[1].FileContractID(index)
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
