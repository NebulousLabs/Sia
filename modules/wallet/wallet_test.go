package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
)

type walletTester struct {
	assistant *consensus.Assistant
	wallet    *Wallet
}

func newWalletTester(t *testing.T) (wt *walletTester) {
	wt = new(walletTester)
	wt.assistant = consensus.NewTestingEnvironment(t)
	tpool, err := transactionpool.New(wt.assistant.State)
	if err != nil {
		t.Fatal(err)
	}
	wt.wallet, err = New(wt.assistant.State, tpool, "")
	if err != nil {
		t.Fatal(err)
	}

	return
}
