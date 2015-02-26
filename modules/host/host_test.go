package host

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/network"
)

var (
	tcpsPort  int = 10500
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
func CreateHostTester(t *testing.T) (ht *HostTester) {
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
	tp, err := transactionpool.New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(ct.State, tp, "../../host_test"+strconv.Itoa(walletNum)+".wallet")
	if err != nil {
		t.Fatal(err)
	}
	walletNum++
	h, err := New(ct.State, tp, w, "../../hostdir"+strconv.Itoa(hostNum))
	if err != nil {
		t.Fatal(err)
	}
	hostNum++

	ht = new(HostTester)
	ht.ConsensusTester = ct
	ht.Host = h
	return
}

// TestSaveLoad tests that saving and loading a Host restores its data.
// TODO: expand this once Host testing is fleshed out.
func TestSaveLoad(t *testing.T) {
	ht := CreateHostTester(t)
	err := ht.save("../../hostdata_test")
	if err != nil {
		ht.Fatal(err)
	}
	err = ht.load("../../hostdata_test")
	if err != nil {
		ht.Fatal(err)
	}
}
