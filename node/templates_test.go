package node

import (
	"testing"

	"github.com/NebulousLabs/Sia/siatest"
)

// TestNewNode is a basic smoke test for NewNode that uses all of the templates
// to verify a working NewNode function.
func TestNewNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Test AllModulesTemplate.
	dir, err := siatest.TestDir("node", t.Name()+"-AllModulesTemplate")
	if err != nil {
		t.Fatal(err)
	}
	n, err := NewNode(AllModules(dir))
	if err != nil {
		t.Fatal(err)
	}
	if n.Gateway == nil {
		t.Error("gateway not set correctly")
	}
	if n.ConsensusSet == nil {
		t.Error("consensus set not set correctly")
	}
	if n.Explorer == nil {
		// TODO: Add the explorer to the node package.
		t.Log("Need to add the explorer to the SiaNode framework.")
	}
	if n.TransactionPool == nil {
		t.Error("transaction pool not set correctly")
	}
	if n.Wallet == nil {
		t.Error("wallet not set correctly")
	}
	if n.Host == nil {
		t.Error("host not set correctly")
	}
	if n.Renter == nil {
		t.Error("renter not set correctly")
	}
	if n.Miner == nil {
		t.Error("miner not set correctly")
	}
	err = n.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Test WalletTemplate.
	dir, err = siatest.TestDir("node", t.Name()+"-WalletTemplate")
	if err != nil {
		t.Fatal(err)
	}
	n, err = NewNode(Wallet(dir))
	if err != nil {
		t.Fatal(err)
	}
	if n.Gateway == nil {
		t.Error("gateway not set correctly")
	}
	if n.ConsensusSet == nil {
		t.Error("consensus set not set correctly")
	}
	if n.Explorer != nil {
		t.Error("explorer should not be created when using the wallet template")
	}
	if n.TransactionPool == nil {
		t.Error("transaction pool not set correctly")
	}
	if n.Wallet == nil {
		t.Error("wallet not set correctly")
	}
	if n.Host != nil {
		t.Error("host should not be created when using the wallet template")
	}
	if n.Renter != nil {
		t.Error("renter should not be created when using the wallet template")
	}
	if n.Miner != nil {
		t.Error("miner should not be created when using the wallet template")
	}
	err = n.Close()
	if err != nil {
		t.Fatal(err)
	}
}
