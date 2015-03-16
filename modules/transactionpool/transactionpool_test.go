package transactionpool

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

// A TpoolTester contains a consensus tester and a transaction pool, and
// provides a set of helper functions for testing the transaction pool without
// modules that need to use the transaction pool.
//
// updateChan is a channel that will block until the transaction pool posts an
// update. This is useful for synchronizing with updates from the state.
type TpoolTester struct {
	cs     *consensus.State
	tpool  *TransactionPool
	miner  modules.Miner
	wallet modules.Wallet

	updateChan chan struct{}

	t *testing.T
}

// emptyUpdateChan will empty the update channel of the TpoolTester. Because
// the channel is only buffered 1 deep, a single pull from the channel is
// sufficient.
func (tpt *TpoolTester) emptyUpdateChan() {
	select {
	case <-tpt.updateChan:
	default:
	}
}

// CreateTpoolTester initializes a TpoolTester.
func CreateTpoolTester(directory string, t *testing.T) (tpt *TpoolTester) {
	// Create the consensus set.
	cs := consensus.CreateGenesisState()

	// Create the gateway.
	gPort := ":" + strconv.Itoa(tester.NewPort())
	gDir := filepath.Join(tester.TempDir(directory), modules.GatewayDir)
	g, err := gateway.New(gPort, cs, gDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create the transaction pool.
	tp, err := New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the wallet.
	wDir := filepath.Join(tester.TempDir(directory), modules.WalletDir)
	w, err := wallet.New(cs, tp, wDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, g, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe to the updates of the transaction pool.
	updateChan := make(chan struct{}, 1)
	id := tp.mu.Lock()
	tp.subscribers = append(tp.subscribers, updateChan)
	tp.mu.Unlock(id)

	// Assebmle all of the objects in to a TpoolTester
	tpt = &TpoolTester{
		cs:         cs,
		tpool:      tp,
		miner:      m,
		wallet:     w,
		updateChan: updateChan,
		t:          t,
	}

	// Mine blocks until there is money in the wallet.
	for i := 0; i <= consensus.MaturityDelay; i++ {
		for {
			var found bool
			_, found, err = tpt.miner.FindBlock()
			if err != nil {
				t.Fatal(err)
			}
			if found {
				break
			}
		}
	}

	return
}
