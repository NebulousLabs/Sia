package transactionpool

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// A tpoolTester is used during testing to initialize a transaction pool and
// useful helper modules.
type tpoolTester struct {
	cs        modules.ConsensusSet
	gateway   modules.Gateway
	tpool     *TransactionPool
	miner     modules.TestMiner
	wallet    modules.Wallet
	walletKey crypto.TwofishKey

	persistDir string
}

// blankTpoolTester returns a ready-to-use tpool tester, with all modules
// initialized, without mining a block.
func blankTpoolTester(name string) (*tpoolTester, error) {
	// Initialize the modules.
	testdir := build.TempDir(modules.TransactionPoolDir, name)
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	var key crypto.TwofishKey
	fastrand.Read(key[:])
	_, err = w.Encrypt(key)
	if err != nil {
		return nil, err
	}
	err = w.Unlock(key)
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}

	// Assemble all of the objects into a tpoolTester
	return &tpoolTester{
		cs:        cs,
		gateway:   g,
		tpool:     tp,
		miner:     m,
		wallet:    w,
		walletKey: key,

		persistDir: testdir,
	}, nil
}

// createTpoolTester returns a ready-to-use tpool tester, with all modules
// initialized.
func createTpoolTester(name string) (*tpoolTester, error) {
	tpt, err := blankTpoolTester(name)
	if err != nil {
		return nil, err
	}

	// Mine blocks until there is money in the wallet.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		b, _ := tpt.miner.FindBlock()
		err = tpt.cs.AcceptBlock(b)
		if err != nil {
			return nil, err
		}
	}

	return tpt, nil
}

// Close safely closes the tpoolTester, calling a panic in the event of an
// error since there isn't a good way to errcheck when deferring a Close.
func (tpt *tpoolTester) Close() error {
	errs := []error{
		tpt.cs.Close(),
		tpt.gateway.Close(),
		tpt.tpool.Close(),
		tpt.miner.Close(),
		tpt.wallet.Close(),
	}
	if err := build.JoinErrors(errs, "; "); err != nil {
		panic(err)
	}
	return nil
}

// TestIntegrationNewNilInputs tries to trigger a panic with nil inputs.
func TestIntegrationNewNilInputs(t *testing.T) {
	// Create a gateway and consensus set.
	testdir := build.TempDir(modules.TransactionPoolDir, t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tpDir := filepath.Join(testdir, modules.TransactionPoolDir)

	// Try all combinations of nil inputs.
	_, err = New(nil, nil, tpDir)
	if err == nil {
		t.Error(err)
	}
	_, err = New(nil, g, tpDir)
	if err != errNilCS {
		t.Error(err)
	}
	_, err = New(cs, nil, tpDir)
	if err != errNilGateway {
		t.Error(err)
	}
	_, err = New(cs, g, tpDir)
	if err != nil {
		t.Error(err)
	}
}

// TestGetTransaction verifies that the transaction pool's Transaction() method
// works correctly.
func TestGetTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	value := types.NewCurrency64(35e6)
	fee := types.NewCurrency64(3e2)
	emptyUH := types.UnlockConditions{}.UnlockHash()
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(value)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddMinerFee(fee)
	output := types.SiacoinOutput{
		Value:      value.Sub(fee),
		UnlockHash: emptyUH,
	}
	txnBuilder.AddSiacoinOutput(output)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	childrenSet := []types.Transaction{{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID: txnSet[len(txnSet)-1].SiacoinOutputID(0),
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      value.Sub(fee),
			UnlockHash: emptyUH,
		}},
	}}

	superSet := append(txnSet, childrenSet...)
	err = tpt.tpool.AcceptTransactionSet(superSet)
	if err != nil {
		t.Fatal(err)
	}

	targetTxn := childrenSet[0]
	txn, parents, exists := tpt.tpool.Transaction(targetTxn.ID())
	if !exists {
		t.Fatal("transaction set did not exist")
	}
	if txn.ID() != targetTxn.ID() {
		t.Fatal("returned the wrong transaction")
	}
	if len(parents) != len(txnSet) {
		t.Fatal("transaction had wrong number of parents")
	}
	for i, txn := range txnSet {
		if parents[i].ID() != txn.ID() {
			t.Fatal("returned the wrong parent")
		}
	}
}

// TestBlockFeeEstimation checks that the fee estimation algorithm is reasonably
// on target when the tpool is relying on blockchain based fee estimation.
func TestFeeEstimation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Prepare a bunch of outputs for a series of graphs to fill up the
	// transaction pool.
	graphLens := 400                                                                     // 80 kb per graph
	numGraphs := int(types.BlockSizeLimit) * blockFeeEstimationDepth / (graphLens * 206) // Enough to fill 'estimation depth' blocks.
	graphFund := types.SiacoinPrecision.Mul64(1000)
	var outputs []types.SiacoinOutput
	for i := 0; i < numGraphs+1; i++ {
		outputs = append(outputs, types.SiacoinOutput{
			UnlockHash: types.UnlockConditions{}.UnlockHash(),
			Value:      graphFund,
		})
	}
	txns, err := tpt.wallet.SendSiacoinsMulti(outputs)
	if err != nil {
		t.Error(err)
	}

	// Mine the graph setup in the consensus set so that the graph outputs are
	// distinct transaction sets.
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Create all of the graphs.
	finalTxn := txns[len(txns)-1]
	var graphs [][]types.Transaction
	for i := 0; i < numGraphs; i++ {
		var edges []types.TransactionGraphEdge
		var cumFee types.Currency
		for j := 0; j < graphLens; j++ {
			fee := types.SiacoinPrecision.Mul64(uint64(j + i + 1)).Div64(200)
			cumFee = cumFee.Add(fee)
			edges = append(edges, types.TransactionGraphEdge{
				Dest:   j + 1,
				Fee:    fee,
				Source: j,
				Value:  graphFund.Sub(cumFee),
			})
		}
		graph, err := types.TransactionGraph(finalTxn.SiacoinOutputID(uint64(i)), edges)
		if err != nil {
			t.Fatal(err)
		}
		graphs = append(graphs, graph)
	}

	// One block at a time, add graphs to the tpool and blockchain. Then check
	// the median fee estimation and see that it's the right value.
	var prevMin types.Currency
	for i := 0; i < blockFeeEstimationDepth; i++ {
		// Insert enough graphs to fill a block.
		for j := 0; j < numGraphs/blockFeeEstimationDepth; j++ {
			err = tpt.tpool.AcceptTransactionSet(graphs[0])
			if err != nil {
				t.Fatal(err)
			}
			graphs = graphs[1:]
		}

		// Add a block to the transaction pool.
		_, err = tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}

		// Check that max is always greater than min.
		min, max := tpt.tpool.FeeEstimation()
		if min.Cmp(max) > 0 {
			t.Error("max fee is less than min fee estimation")
		}

		// If we're over halfway through the depth, the suggested fee should
		// start to exceed the default.
		if i > blockFeeEstimationDepth/2 {
			if min.Cmp(minEstimation) <= 0 {
				t.Error("fee estimation does not seem to be increasing")
			}
			if min.Cmp(prevMin) <= 0 {
				t.Error("fee estimation does not seem to be increasing")
			}
		}
		prevMin = min

		// Reset the tpool to verify that the persist structures are
		// functioning.
		//
		// TODO: For some reason, closing and re-opeining the tpool results in
		// incredible performance penalties.
		/*
			err = tpt.tpool.Close()
			if err != nil {
				t.Fatal(err)
			}
			tpt.tpool, err = New(tpt.cs, tpt.gateway, tpt.persistDir)
			if err != nil {
				t.Fatal(err)
			}
		*/
	}

	// Mine a few blocks and then check that the fee estimation has returned to
	// minimum as congestion clears up.
	for i := 0; i < (blockFeeEstimationDepth/2)+1; i++ {
		_, err = tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	min, _ := tpt.tpool.FeeEstimation()
	if !(min.Cmp(minEstimation) == 0) {
		t.Error("fee estimator does not seem to be reducing with empty blocks.")
	}
}
