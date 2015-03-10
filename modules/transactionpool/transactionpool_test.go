package transactionpool

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

var (
	rpcPort int = 9500
)

// a TpoolTester contains a consensus tester and a transaction pool, and
// provides a set of helper functions for testing the transaction pool without
// modules that need to use the transaction pool.
type TpoolTester struct {
	*consensus.ConsensusTester
	*TransactionPool
}

// CreateTpoolTester initializes a TpoolTester.
func CreateTpoolTester(t *testing.T) (tpt *TpoolTester) {
	ct := consensus.NewTestingEnvironment(t)
	g, err := gateway.New(":"+strconv.Itoa(rpcPort), ct.State)
	if err != nil {
		t.Fatal(err)
	}
	rpcPort++
	tp, err := New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}

	tpt = new(TpoolTester)
	tpt.ConsensusTester = ct
	tpt.TransactionPool = tp
	return
}
