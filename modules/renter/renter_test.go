package renter

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

// A RenterTester contains a consensus tester and a renter, and provides a set
// of helper functions for testing the renter without needing other modules
// such as the host.
type RenterTester struct {
	*consensus.ConsensusTester
	*Renter
}

// CreateHostTester initializes a HostTester.
func CreateRenterTester(name string, t *testing.T) (rt *RenterTester) {
	testdir := tester.TempDir("renter", name)
	cs, err := consensus.New(filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	ct := consensus.NewConsensusTester(t, cs)
	g, err := gateway.New(":0", ct.State, filepath.Join(testdir, modules.GatewayDir))
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
	w, err := wallet.New(ct.State, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(ct.State, g, hdb, w, filepath.Join(testdir, modules.RenterDir))
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
	rt := CreateRenterTester("TestSaveLoad", t)
	err := rt.save()
	if err != nil {
		rt.Fatal(err)
	}
	err = rt.load()
	if err != nil {
		rt.Fatal(err)
	}
}
