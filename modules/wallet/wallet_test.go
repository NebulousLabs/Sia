package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
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
	tpool, err := transactionpool.New(wt.State)
	if err != nil {
		t.Fatal(err)
	}
	gateway := gateway.New(nil, wt.State, tpool)
	wt.Wallet, err = New(wt.State, tpool, gateway, "")
	if err != nil {
		t.Fatal(err)
	}

	return
}
