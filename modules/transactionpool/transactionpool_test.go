package transactionpool

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// A tpoolTester is used during testing to initialize a transaction pool and
// useful helper modules. The update channels are used to synchronize updates
// that occur during testing. Any time that an update is submitted to the
// transaction pool or consensus set, updateWait() should be called or
// desynchronization could be introduced.
type tpoolTester struct {
	cs      *consensus.State
	gateway modules.Gateway
	tpool   *TransactionPool
	miner   modules.Miner
	wallet  modules.Wallet

	csUpdateChan     <-chan struct{}
	tpoolUpdateChan  <-chan struct{}
	minerUpdateChan  <-chan struct{}
	walletUpdateChan <-chan struct{}

	t *testing.T
}

// emptyUnlockTransaction creates a transaction with empty UnlockConditions,
// meaning it's trivial to spend the output.
func (tpt *tpoolTester) emptyUnlockTransaction() types.Transaction {
	// Send money to an anyone-can-spend address.
	emptyHash := types.UnlockConditions{}.UnlockHash()
	txn, err := tpt.spendCoins(types.NewCurrency64(1), emptyHash)
	if err != nil {
		tpt.t.Fatal(err)
	}
	outputID := txn.SiacoinOutputID(0)

	// Create a transaction spending the coins.
	txn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			types.SiacoinInput{
				ParentID: outputID,
			},
		},
		SiacoinOutputs: []types.SiacoinOutput{
			types.SiacoinOutput{
				Value:      types.NewCurrency64(1),
				UnlockHash: emptyHash,
			},
		},
	}

	return txn
}

// csUpdateWait listens on all channels until a consensus set update has
// reached all modules.
func (tpt *tpoolTester) csUpdateWait() {
	<-tpt.csUpdateChan
	tpt.tpUpdateWait()
}

// tpUpdateWait listens on all channels until a transaction pool update has
// reached all modules.
func (tpt *tpoolTester) tpUpdateWait() {
	<-tpt.tpoolUpdateChan
	<-tpt.minerUpdateChan
	<-tpt.walletUpdateChan
}

// spendCoins sends the desired amount of coins to the desired address, calling
// wait at all of the appropriate places to assist synchronization.
func (tpt *tpoolTester) spendCoins(amount types.Currency, dest types.UnlockHash) (t types.Transaction, err error) {
	output := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: dest,
	}
	id, err := tpt.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	_, err = tpt.wallet.FundTransaction(id, amount)
	if err != nil {
		return
	}
	tpt.tpUpdateWait()
	_, _, err = tpt.wallet.AddSiacoinOutput(id, output)
	if err != nil {
		return
	}
	t, err = tpt.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = tpt.tpool.AcceptTransaction(t)
	if err != nil {
		return
	}
	tpt.tpUpdateWait()
	return
}

// newTpoolTester returns a ready-to-use tpool tester, with all modules
// initialized.
func newTpoolTester(name string, t *testing.T) *tpoolTester {
	testdir := build.TempDir("transactionpool", name)

	// Create the gateway.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the consensus set.
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the transaction pool.
	tp, err := New(cs, g)
	if err != nil {
		t.Fatal(err)
	}

	// Create the wallet.
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}

	// Create the miner.
	m, err := miner.New(cs, tp, w)
	if err != nil {
		t.Fatal(err)
	}

	// Assebmle all of the objects in to a tpoolTester
	tpt := &tpoolTester{
		cs:      cs,
		gateway: g,
		tpool:   tp,
		miner:   m,
		wallet:  w,

		csUpdateChan:     cs.ConsensusSetNotify(),
		tpoolUpdateChan:  tp.TransactionPoolNotify(),
		minerUpdateChan:  m.MinerNotify(),
		walletUpdateChan: w.WalletNotify(),

		t: t,
	}
	tpt.csUpdateWait()

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, _, err = tpt.miner.FindBlock()
		if err != nil {
			t.Fatal(err)
		}
		tpt.csUpdateWait()
	}

	return tpt
}

// TestNewNilInputs tries to trigger a panic with nil inputs.
func TestNewNilInputs(t *testing.T) {
	testdir := build.TempDir("transactionpool", "TestNewNilInputs")

	// Create a gateway and consensus set.
	g, err := gateway.New(":0", filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	New(nil, nil)
	New(cs, nil)
	New(nil, g)
}
