package host

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

var (
	walletNum int = 0
	hostNum   int = 0
)

// A HostTester contains a consensus tester and a host, and provides a set of
// helper functions for testing the host without needing other modules such as
// the renter.
type HostTester struct {
	*consensus.ConsensusTester
	*Host
}

// CreateHostTester initializes a HostTester.
func CreateHostTester(directory string, t *testing.T) (ht *HostTester) {
	ct := consensus.NewTestingEnvironment(t)
	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := gateway.New(":0", ct.State, gDir)
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

	h, err := New(ct.State, tp, w, modules.HostDir)
	if err != nil {
		t.Fatal(err)
	}
	hostNum++

	ht = new(HostTester)
	ht.ConsensusTester = ct
	ht.Host = h
	return
}
