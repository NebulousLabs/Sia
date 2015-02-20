package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
)

// TODO: Test that all of the linked list functions work correctly.

// a tpoolTester contains a testing assistant and a transaction pool, and
// provides statefullness for doing testing.
type tpoolTester struct {
	assistant       *consensus.ConsensusTester
	transactionPool *TransactionPool
}

func CreateTpoolTester(t *testing.T) (tpt *tpoolTester) {
	a := consensus.NewTestingEnvironment(t)
	tp, err := New(a.State)
	if err != nil {
		t.Fatal(err)
	}

	tpt = &tpoolTester{
		assistant:       a,
		transactionPool: tp,
	}
	return
}
