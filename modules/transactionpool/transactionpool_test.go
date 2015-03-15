package transactionpool

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
)

var (
	rpcPort int = 9500
)

// a TpoolTester contains a consensus tester and a transaction pool, and
// provides a set of helper functions for testing the transaction pool without
// modules that need to use the transaction pool.
type TpoolTester struct {
	*TransactionPool
	modules.Miner
}

// CreateTpoolTester initializes a TpoolTester.
func CreateTpoolTester(instance string, t *testing.T) (tpt *TpoolTester) {
	s := consensus.CreateGenesisState()
	g, err := gateway.New(":"+strconv.Itoa(rpcPort), ct.State, "")
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
