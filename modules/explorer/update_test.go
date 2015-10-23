package explorer

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationExplorerActiveContractMetrics checks that the siacoin
// transfer volume metric is working correctly.
func TestIntegrationExplorerActiveContractMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	et, err := createExplorerTester("TestIntegrationExporerActiveContractMetrics")
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

	// Check that the active stats have updated to represent the file contract.
	if et.explorer.activeContractCost.Cmp(types.NewCurrency64(5e9)) != 0 {
		t.Error("active resources providing wrong file contract cost")
	}
	if et.explorer.activeContractCount != 1 {
		t.Error("active contract count does not read correctly")
	}
	if et.explorer.activeContractSize.Cmp(types.NewCurrency64(5e3)) != 0 {
		t.Error("active contract size is not correctly reported")
	}

	// Perform some sort of reorg and check that the reorg is handled
	// correctly.
}
