package host

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

var (
	rpcPort   int = 10500
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
	g, err := gateway.New(":"+strconv.Itoa(rpcPort), ct.State)
	if err != nil {
		t.Fatal(err)
	}
	rpcPort++
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
