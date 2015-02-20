package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
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
	tp, err := New(ct.State)
	if err != nil {
		t.Fatal(err)
	}

	tpt = new(TpoolTester)
	tpt.ConsensusTester = ct
	tpt.TransactionPool = tp
	return
}
