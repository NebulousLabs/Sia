package wallet

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
)

// global variables used to prevent conflicts
var (
	rpcPort   int = 10000
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
	g, err := gateway.New(":"+strconv.Itoa(rpcPort), wt.State)
	if err != nil {
		t.Fatal(err)
	}
	rpcPort++
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
