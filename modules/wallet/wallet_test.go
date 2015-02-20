package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
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
	wt.Wallet, err = New(wt.State, tpool, "")
	if err != nil {
		t.Fatal(err)
	}

	return
}
