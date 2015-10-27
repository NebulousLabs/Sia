package explorer

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationExplorerFileContractMetrics checks that the siacoin
// transfer volume metric is working correctly.
func TestIntegrationExplorerFileContractMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	et, err := createExplorerTester("TestIntegrationExporerFileContractMetrics")
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
	if !et.explorer.activeContractCost.IsZero() {
		t.Error("fresh explorer has nonzero active contract cost")
	}
	if et.explorer.activeContractCount != 0 {
		t.Error("active contract count should initialize to zero")
	}
	if !et.explorer.activeContractSize.IsZero() {
		t.Error("active contract size should initialize to zero")
	}

	// Put a file contract into the chain, and check that the explorer
	// correctly does all of the counting.
	builder := et.wallet.StartTransaction()
	builder.FundSiacoins(types.NewCurrency64(5e9))
	fcOutputs := []types.SiacoinOutput{{Value: types.NewCurrency64(4805e6)}}
	fc := types.FileContract{
		FileSize:           5e3,
		WindowStart:        et.explorer.blockchainHeight + 2,
		WindowEnd:          et.explorer.blockchainHeight + 3,
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
	if et.explorer.activeContractCost.Cmp(types.NewCurrency64(5e9)) != 0 {
		t.Error("active resources providing wrong file contract cost")
	}
	if et.explorer.activeContractCount != 1 {
		t.Error("active contract count does not read correctly")
	}
	if et.explorer.activeContractSize.Cmp(types.NewCurrency64(5e3)) != 0 {
		t.Error("active contract size is not correctly reported")
	}
	if et.explorer.totalContractCost.Cmp(types.NewCurrency64(5e9)) != 0 {
		t.Error("total cost is not tallied correctly")
	}
	if et.explorer.fileContractCount != 1 {
		t.Error("total contract count is not accurate")
	}
	if et.explorer.totalContractSize.Cmp(types.NewCurrency64(5e3)) != 0 {
		t.Error("total contract size is not accurate")
	}

	// Put a second file into the explorer to check that multiple files are
	// handled well.
	builder = et.wallet.StartTransaction()
	builder.FundSiacoins(types.NewCurrency64(1e9))
	fcOutputs = []types.SiacoinOutput{{Value: types.NewCurrency64(961e6)}}
	fc = types.FileContract{
		FileSize:           15e3,
		WindowStart:        et.explorer.blockchainHeight + 2,
		WindowEnd:          et.explorer.blockchainHeight + 3,
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
	if et.explorer.activeContractCost.Cmp(types.NewCurrency64(6e9)) != 0 {
		t.Error("active resources providing wrong file contract cost")
	}
	if et.explorer.activeContractCount != 2 {
		t.Error("active contract count does not read correctly")
	}
	if et.explorer.activeContractSize.Cmp(types.NewCurrency64(20e3)) != 0 {
		t.Error("active contract size is not correctly reported")
	}
	if et.explorer.totalContractCost.Cmp(types.NewCurrency64(6e9)) != 0 {
		t.Error("total cost is not tallied correctly")
	}
	if et.explorer.fileContractCount != 2 {
		t.Error("total contract count is not accurate")
	}
	if et.explorer.totalContractSize.Cmp(types.NewCurrency64(20e3)) != 0 {
		t.Error("total contract size is not accurate")
	}

	// Expire the first file contract but not the second.
	_, err = et.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the stats have updated to reflect the expired file contract.
	if et.explorer.activeContractCost.Cmp(types.NewCurrency64(1e9)) != 0 {
		t.Error("active resources providing wrong file contract cost", et.explorer.activeContractCost)
	}
	if et.explorer.activeContractCount != 1 {
		t.Error("active contract count does not read correctly")
	}
	if et.explorer.activeContractSize.Cmp(types.NewCurrency64(15e3)) != 0 {
		t.Error("active contract size is not correctly reported")
	}
	if et.explorer.totalContractCost.Cmp(types.NewCurrency64(6e9)) != 0 {
		t.Error("total cost is not tallied correctly")
	}
	if et.explorer.fileContractCount != 2 {
		t.Error("total contract count is not accurate")
	}
	if et.explorer.totalContractSize.Cmp(types.NewCurrency64(20e3)) != 0 {
		t.Error("total contract size is not accurate")
	}

	// TODO: Perform some sort of reorg and check that the reorg is handled
	// correctly.
}
