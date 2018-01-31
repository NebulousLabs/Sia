package siatest

import (
	"testing"
)

// TestCreateTestGroup tests the behavior of NewGroup.
func TestNewGroup(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Specify the parameters for the group
	groupParams := GroupParams{
		hosts:   5,
		renters: 2,
		miners:  2,
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
	if len(tg.Hosts()) != groupParams.hosts {
		t.Error("Wrong number of hosts")
	}
	if len(tg.Renters()) != groupParams.renters {
		t.Error("Wrong number of renters")
	}
	if len(tg.Miners()) != groupParams.miners {
		t.Error("Wrong number of miners")
	}
	if len(tg.Nodes()) != groupParams.hosts+groupParams.renters+groupParams.miners {
		t.Error("Wrong number of nodes")
	}

	// Check if nodes are funded
	cg, err := tg.Nodes()[0].ConsensusGet()
	for _, node := range tg.Nodes() {
		wtg, err := node.WalletTransactionsGet(0, cg.Height)
		if err != nil {
			t.Fatal(err)
		}
		if len(wtg.ConfirmedTransactions) == 0 {
			t.Errorf("Node has 0 confirmed funds")
		}
	}

	// TODO check if hosts are announced and in each other's database
}

// TestCreateTestGroup tests NewGroup without a miner
func TestNewGroupNoMiner(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Try to create a group without miners
	groupParams := GroupParams{
		hosts:   5,
		renters: 2,
		miners:  0,
	}
	// Create the group
	_, err := NewGroupFromTemplate(groupParams)
	if err == nil {
		t.Fatal("Creating a group without miners should fail: ", err)
	}
}

// TestCreateTestGroup tests NewGroup with no renter or host
func TestNewGroupNoRenterHost(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a group with nothing but a single miner
	groupParams := GroupParams{
		hosts:   0,
		renters: 0,
		miners:  5,
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
}
