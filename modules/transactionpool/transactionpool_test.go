package transactionpool

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/network"
)

var (
	tcpsPort int = 9500
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
	tcps, err := network.NewTCPServer(":" + strconv.Itoa(tcpsPort))
	tcpsPort++
	if err != nil {
		t.Fatal(err)
	}
	g, err := gateway.New(tcps, ct.State)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := New(ct.State, g)
	if err != nil {
		t.Fatal(err)
	}

	tpt = new(TpoolTester)
	tpt.ConsensusTester = ct
	tpt.TransactionPool = tp
	return
}
