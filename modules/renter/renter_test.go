package renter

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/network"
)

var (
	tcpsPort int = 10500
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

	tcps, err := network.NewTCPServer(":" + strconv.Itoa(tcpsPort))
	tcpsPort++
	if err != nil {
		t.Fatal(err)
	}
	g, err := gateway.New(tcps, ct.State)
	if err != nil {
		t.Fatal(err)
	}
	hdb, err := hostdb.New(ct.State)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(ct.State, tp, "")
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(ct.State, hdb, w)
	if err != nil {
		t.Fatal(err)
	}

	rt = new(RenterTester)
	rt.ConsensusTester = ct
	rt.Renter = r
	return
}

// TestCreateHostTester is a temporary function to call CreateHostTester during
// testing.
func TestCreateRenterTester(t *testing.T) {
	_ = CreateRenterTester(t)
}
