package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/network"
)

// A Wallet tester contains a ConsensusTester and has a bunch of helpful
// functions for facilitating wallet integration testing.
type WalletTester struct {
	*Wallet
	*consensus.ConsensusTester
}

// NewWalletTester takes a testing.T and creates a WalletTester.
func NewWalletTester(t *testing.T) (wt *WalletTester) {
	wt = new(WalletTester)
	wt.ConsensusTester = consensus.NewTestingEnvironment(t)
	tcps, err := network.NewTCPServer(":9003")
	if err != nil {
		t.Fatal(err)
	}
	g, err := gateway.New(tcps, wt.State)
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(wt.State, g)
	if err != nil {
		t.Fatal(err)
	}
	wt.Wallet, err = New(wt.State, tpool, "")
	if err != nil {
		t.Fatal(err)
	}

	return
}
