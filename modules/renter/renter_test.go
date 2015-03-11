package renter

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

var (
	rpcPort   int = 11000
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
func CreateRenterTester(t *testing.T) (rt *RenterTester) {
	ct := consensus.NewTestingEnvironment(t)

	g, err := gateway.New(":"+strconv.Itoa(rpcPort), ct.State, "")
	if err != nil {
		t.Fatal(err)
	}
	rpcPort++
	hdb, err := hostdb.New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(ct.State, tp, "../../renter_test"+strconv.Itoa(walletNum)+".wallet")
	if err != nil {
		t.Fatal(err)
	}
	walletNum++
	r, err := New(ct.State, g, hdb, w)
	if err != nil {
		t.Fatal(err)
	}

	rt = new(RenterTester)
	rt.ConsensusTester = ct
	rt.Renter = r
	return
}

// TestSaveLoad tests that saving and loading a Renter restores its data.
// TODO: expand this once Renter testing is fleshed out.
func TestSaveLoad(t *testing.T) {
	rt := CreateRenterTester(t)
	rt.persistentFile = "../../renterdata_test"
	err := rt.save()
	if err != nil {
		rt.Fatal(err)
	}
	err = rt.load("../../renterdata_test")
	if err != nil {
		rt.Fatal(err)
	}
}
