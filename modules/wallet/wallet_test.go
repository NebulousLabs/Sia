package wallet

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/network"
)

// global variables used to prevent conflicts
var (
	tcpsPort  int = 10000
	walletNum int = 0
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
	tcps, err := network.NewTCPServer(":" + strconv.Itoa(tcpsPort))
	tcpsPort++
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
	wt.Wallet, err = New(wt.State, tpool, "../../wallet_test"+strconv.Itoa(walletNum)+".wallet")
	if err != nil {
		t.Fatal(err)
	}
	walletNum++

	return
}
