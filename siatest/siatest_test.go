package siatest

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestNewTestNode checks that the NewTestNode method is properly build and can
// use the actual 'New' calls by the modules to create test objects.
func TestNewTestNode(t *testing.T) {
	testDir := build.TempDir("TempDir", "TestNewTestNode")
	tn, err := NewTestNode(NewTestNodeParams{Dir: testDir})
	if err != nil {
		t.Fatal(err)
	}
	if tn.Gateway == nil {
		t.Error("gateway not set correctly")
	}
	if tn.ConsensusSet == nil {
		t.Error("consensus set not set correctly")
	}
	if tn.TransactionPool == nil {
		t.Error("transaction pool not set correctly")
	}
	if tn.Wallet == nil {
		t.Error("wallet not set correctly")
	}
	if tn.Host == nil {
		t.Error("host not set correctly")
	}
	if tn.Renter == nil {
		t.Error("renter not set correctly")
	}
	if tn.Miner == nil {
		t.Error("miner not set correctly")
	}
}
