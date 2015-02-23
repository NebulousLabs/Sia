package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/network"
)

// a TpoolTester contains a testing assistant and a transaction pool, and
// provides statefullness for doing testing.
type TpoolTester struct {
	*consensus.ConsensusTester
	*TransactionPool
}

// CreateTpoolTester initializes a TpoolTester.
func CreateTpoolTester(t *testing.T) (tpt *TpoolTester) {
	ct := consensus.NewTestingEnvironment(t)
	tcps, err := network.NewTCPServer(":9002")
	if err != nil {
		t.Fatal(err)
	}
	g, err := gateway.New(tcps, ct.State)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := New(ct.State, ct)
	if err != nil {
		t.Fatal(err)
	}

	tpt = new(TpoolTester)
	tpt.ConsensusTester = ct
	tpt.TransactionPool = tp
	return
}
