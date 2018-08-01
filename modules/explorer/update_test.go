package explorer

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

func (et *explorerTester) currentFacts() (facts modules.BlockFacts, exists bool) {
	var height types.BlockHeight
	err := et.explorer.db.View(dbGetInternal(internalBlockHeight, &height))
	if err != nil {
		exists = false
		return
	}
	return et.explorer.BlockFacts(height)
}

// TestIntegrationExplorerFileContractMetrics checks that the siacoin
// transfer volume metric is working correctly.
func TestIntegrationExplorerFileContractMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	et, err := createExplorerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	// Propel explorer tester past the hardfork height.
	for i := 0; i < 10; i++ {
		_, err = et.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	facts, ok := et.currentFacts()
	if !ok {
		t.Fatal("couldn't get current facts")
	}
	if !facts.ActiveContractCost.IsZero() {
		t.Error("fresh explorer has nonzero active contract cost")
	}
	if facts.ActiveContractCount != 0 {
		t.Error("active contract count should initialize to zero")
	}
	if !facts.ActiveContractSize.IsZero() {
		t.Error("active contract size should initialize to zero")
	}

	// Put a file contract into the chain, and check that the explorer
	// correctly does all of the counting.
	builder, err := et.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	builder.FundSiacoins(types.NewCurrency64(5e9))
	fcOutputs := []types.SiacoinOutput{{Value: types.NewCurrency64(4805e6)}}
	fc := types.FileContract{
		FileSize:           5e3,
		WindowStart:        et.cs.Height() + 2,
		WindowEnd:          et.cs.Height() + 3,
		Payout:             types.NewCurrency64(5e9),
		ValidProofOutputs:  fcOutputs,
		MissedProofOutputs: fcOutputs,
	}
	_ = builder.AddFileContract(fc)
	txns, err := builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = et.tpool.AcceptTransactionSet(txns)
	if err != nil {
		t.Fatal(err)
	}
	_, err = et.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the stats have updated to represent the file contract.
	facts, ok = et.currentFacts()
	if !ok {
		t.Fatal("couldn't get current facts")
	}
	if !facts.ActiveContractCost.Equals64(5e9) {
		t.Error("active resources providing wrong file contract cost")
	}
	if facts.ActiveContractCount != 1 {
		t.Error("active contract count does not read correctly")
	}
	if !facts.ActiveContractSize.Equals64(5e3) {
		t.Error("active contract size is not correctly reported")
	}
	if !facts.TotalContractCost.Equals64(5e9) {
		t.Error("total cost is not tallied correctly")
	}
	if facts.FileContractCount != 1 {
		t.Error("total contract count is not accurate")
	}
	if !facts.TotalContractSize.Equals64(5e3) {
		t.Error("total contract size is not accurate")
	}

	// Put a second file into the explorer to check that multiple files are
	// handled well.
	builder, err = et.wallet.StartTransaction()
	if err != nil {
		t.Fatal(err)
	}
	builder.FundSiacoins(types.NewCurrency64(1e9))
	fcOutputs = []types.SiacoinOutput{{Value: types.NewCurrency64(961e6)}}
	fc = types.FileContract{
		FileSize:           15e3,
		WindowStart:        et.cs.Height() + 2,
		WindowEnd:          et.cs.Height() + 3,
		Payout:             types.NewCurrency64(1e9),
		ValidProofOutputs:  fcOutputs,
		MissedProofOutputs: fcOutputs,
	}
	_ = builder.AddFileContract(fc)
	txns, err = builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = et.tpool.AcceptTransactionSet(txns)
	if err != nil {
		t.Fatal(err)
	}
	_, err = et.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the stats have updated to represent the file contracts.
	facts, ok = et.currentFacts()
	if !ok {
		t.Fatal("couldn't get current facts")
	}
	if !facts.ActiveContractCost.Equals64(6e9) {
		t.Error("active resources providing wrong file contract cost")
	}
	if facts.ActiveContractCount != 2 {
		t.Error("active contract count does not read correctly")
	}
	if !facts.ActiveContractSize.Equals64(20e3) {
		t.Error("active contract size is not correctly reported")
	}
	if !facts.TotalContractCost.Equals64(6e9) {
		t.Error("total cost is not tallied correctly")
	}
	if facts.FileContractCount != 2 {
		t.Error("total contract count is not accurate")
	}
	if !facts.TotalContractSize.Equals64(20e3) {
		t.Error("total contract size is not accurate")
	}

	// Expire the first file contract but not the second.
	_, err = et.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the stats have updated to reflect the expired file contract.
	facts, ok = et.currentFacts()
	if !ok {
		t.Fatal("couldn't get current facts")
	}
	if !facts.ActiveContractCost.Equals64(1e9) {
		t.Error("active resources providing wrong file contract cost", facts.ActiveContractCost)
	}
	if facts.ActiveContractCount != 1 {
		t.Error("active contract count does not read correctly")
	}
	if !facts.ActiveContractSize.Equals64(15e3) {
		t.Error("active contract size is not correctly reported")
	}
	if !facts.TotalContractCost.Equals64(6e9) {
		t.Error("total cost is not tallied correctly")
	}
	if facts.FileContractCount != 2 {
		t.Error("total contract count is not accurate")
	}
	if !facts.TotalContractSize.Equals64(20e3) {
		t.Error("total contract size is not accurate")
	}

	// Reorg the block explorer to a blank state, see that all of the file
	// contract statistics got removed.

	// TODO: broken by new block facts model

	// err = et.reorgToBlank()
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// facts, ok = et.currentFacts()
	// if !ok {
	// 	t.Fatal("couldn't get current facts")
	// }
	// if !facts.ActiveContractCost.IsZero() {
	// 	t.Error("post reorg active contract cost should be zero, got", facts.ActiveContractCost)
	// }
	// if facts.ActiveContractCount != 0 {
	// 	t.Error("post reorg active contract count should be zero, got", facts.ActiveContractCount)
	// }
	// if !facts.TotalContractCost.IsZero() {
	// 	t.Error("post reorg total contract cost should be zero, got", facts.TotalContractCost)
	// }
	// if facts.FileContractCount != 0 {
	// 	t.Error("post reorg file contract count should be zero, got", facts.FileContractCount)
	// }
}
