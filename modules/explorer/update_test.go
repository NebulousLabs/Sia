package explorer

import (
	"testing"
)

// TestIntegrationExplorerActiveContractMetrics checks that the siacoin
// transfer volume metric is working correctly.
func TestIntegrationExplorerActiveContractMetrics(t *testing.T) {
	et, err := createExplorerTester("TestIntegrationExporerActiveContractMetrics")
	if err != nil {
		t.Fatal(err)
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

	// Perform some sort of reorg and check that the reorg is handled
	// correctly.
}
