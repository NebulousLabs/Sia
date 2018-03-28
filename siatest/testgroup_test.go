package siatest

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestCreateTestGroup tests the behavior of NewGroup.
func TestNewGroup(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Specify the parameters for the group
	groupParams := GroupParams{
		Hosts:   5,
		Renters: 2,
		Miners:  2,
	}
	// Create the group
	tg, err := NewGroupFromTemplate(groupParams)
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Check if the correct number of nodes was created
	if len(tg.Hosts()) != groupParams.Hosts {
		t.Error("Wrong number of hosts")
	}
	if len(tg.Renters()) != groupParams.Renters {
		t.Error("Wrong number of renters")
	}
	if len(tg.Miners()) != groupParams.Miners {
		t.Error("Wrong number of miners")
	}
	if len(tg.Nodes()) != groupParams.Hosts+groupParams.Renters+groupParams.Miners {
		t.Error("Wrong number of nodes")
	}

	// Check if nodes are funded
	cg, err := tg.Nodes()[0].ConsensusGet()
	if err != nil {
		t.Fatal("Failed to get consensus: ", err)
	}
	for _, node := range tg.Nodes() {
		wtg, err := node.WalletTransactionsGet(0, cg.Height)
		if err != nil {
			t.Fatal(err)
		}
		if len(wtg.ConfirmedTransactions) == 0 {
			t.Errorf("Node has 0 confirmed funds")
		}
	}
}

// TestCreateTestGroup tests NewGroup without a miner
func TestNewGroupNoMiner(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	// Try to create a group without miners
	groupParams := GroupParams{
		Hosts:   5,
		Renters: 2,
		Miners:  0,
	}
	// Create the group
	_, err := NewGroupFromTemplate(groupParams)
	if err == nil {
		t.Fatal("Creating a group without miners should fail: ", err)
	}
}

// TestCreateTestGroup tests NewGroup with no renter or host
func TestNewGroupNoRenterHost(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	// Create a group with nothing but miners
	groupParams := GroupParams{
		Hosts:   0,
		Renters: 0,
		Miners:  5,
	}
	// Create the group
	tg, err := NewGroupFromTemplate(groupParams)
	if err != nil {
		t.Fatal("Failed to create group: ", err)
	}
	func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()
}
