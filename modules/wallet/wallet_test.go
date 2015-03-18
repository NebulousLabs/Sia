package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
)

var (
	walletNum int = 0
)

// A Wallet tester contains a ConsensusTester and has a bunch of helpful
// functions for facilitating wallet integration testing.
type WalletTester struct {
	*Wallet
	*consensus.ConsensusTester
}

// NewWalletTester takes a testing.T and creates a WalletTester.
func NewWalletTester(directory string, t *testing.T) (wt *WalletTester) {
	wt = new(WalletTester)
	wt.ConsensusTester = consensus.NewTestingEnvironment(t)
	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := gateway.New(":0", wt.State, gDir)
	if err != nil {
		t.Fatal(err)
	}
	tpool, err := transactionpool.New(wt.State, g)
	if err != nil {
		t.Fatal(err)
	}
	wDir := tester.TempDir(directory, modules.WalletDir)
	wt.Wallet, err = New(wt.State, tpool, wDir)
	if err != nil {
		t.Fatal(err)
	}
	walletNum++

	return
}
