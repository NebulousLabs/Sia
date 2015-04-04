package renter

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

var (
	walletNum int = 0
)

// A RenterTester contains a consensus tester and a renter, and provides a set
// of helper functions for testing the renter without needing other modules
// such as the host.
type RenterTester struct {
	*consensus.ConsensusTester
	*Renter
}

// CreateHostTester initializes a HostTester.
func CreateRenterTester(directory string, t *testing.T) (rt *RenterTester) {
	ct := consensus.NewTestingEnvironment(t)

	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := gateway.New(":0", ct.State, gDir)
	if err != nil {
		t.Fatal(err)
	}
	hdb, err := hostdb.New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}
	wDir := tester.TempDir(directory, modules.WalletDir)
	w, err := wallet.New(ct.State, tp, wDir)
	if err != nil {
		t.Fatal(err)
	}
	walletNum++
	rDir := tester.TempDir(directory, modules.RenterDir)
	r, err := New(ct.State, g, hdb, w, rDir)
	if err != nil {
		t.Fatal(err)
	}

	rt = new(RenterTester)
	rt.ConsensusTester = ct
	rt.Renter = r
	return
}

// TestSaveLoad tests that saving and loading a Renter restores its data.
func TestSaveLoad(t *testing.T) {
	rt := CreateRenterTester("Renter - TestSaveLoad", t)
	err := rt.save()
	if err != nil {
		rt.Fatal(err)
	}
	err = rt.load()
	if err != nil {
		rt.Fatal(err)
	}
}
