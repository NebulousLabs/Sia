package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
)

type walletTester struct {
	*Wallet
	*consensus.ConsensusTester
}

func newWalletTester(t *testing.T) (wt *walletTester) {
	wt = new(walletTester)
	wt.ConsensusTester = consensus.NewTestingEnvironment(t)
	tpool, err := transactionpool.New(wt.State)
	if err != nil {
		t.Fatal(err)
	}
	gateway := gateway.New(nil, wt.assistant.State, tpool)
	wt.wallet, err = New(wt.assistant.State, tpool, gateway, "")
	if err != nil {
		t.Fatal(err)
	}

	return
}
