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
	if testing.Short() || !build.VLONG {
		t.Skip("Tpool is too slow to run this test regularly")
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

// TestTpoolScalability fills the whole transaction pool with complex
// transactions, then mines enough blocks to empty it out. Running sequentially,
// the test should take less than 250ms per mb that the transaction pool fills
// up, and less than 250ms per mb to empty out - indicating linear scalability
// and tolerance for a larger pool size.
func TestTpoolScalability(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.Skip("Tpool is too slow to run this test regularly")
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Mine a few more blocks to get some extra funding.
	for i := 0; i < 3; i++ {
		_, err := tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Prepare a bunch of outputs for a series of graphs to fill up the
	// transaction pool.
	rows := 10                                         // needs to factor into exclusively '2's and '5's.
	graphSize := 11796                                 // Measured with logging. Change if 'rows' changes.
	numGraphs := TransactionPoolSizeTarget / graphSize // Enough to fill the transaction pool.
	graphFund := types.SiacoinPrecision.Mul64(2000)
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

		// Create the root of the graph.
		feeValues := types.SiacoinPrecision
		firstRowValues := graphFund.Sub(feeValues.Mul64(uint64(rows))).Div64(uint64(rows))
		for j := 0; j < rows; j++ {
			edges = append(edges, types.TransactionGraphEdge{
				Dest:   j + 1,
				Fee:    types.SiacoinPrecision,
				Source: 0,
				Value:  firstRowValues,
			})
		}

		// Create each row of the graph.
		var firstNodeValue types.Currency
		nodeIndex := 1
		for j := 0; j < rows; j++ {
			// Create the first node in the row, which has an increasing
			// balance.
			rowValue := firstRowValues.Sub(types.SiacoinPrecision.Mul64(uint64(j + 1)))
			firstNodeValue = firstNodeValue.Add(rowValue)
			edges = append(edges, types.TransactionGraphEdge{
				Dest:   nodeIndex + (rows - j),
				Fee:    types.SiacoinPrecision,
				Source: nodeIndex,
				Value:  firstNodeValue,
			})
			nodeIndex++

			// Create the remaining nodes in this row.
			for k := j + 1; k < rows; k++ {
				edges = append(edges, types.TransactionGraphEdge{
					Dest:   nodeIndex + (rows - (j + 1)),
					Fee:    types.SiacoinPrecision,
					Source: nodeIndex,
					Value:  rowValue,
				})
				nodeIndex++
			}
		}

		// Build the graph and add it to the stack of graphs.
		graph, err := types.TransactionGraph(finalTxn.SiacoinOutputID(uint64(i)), edges)
		if err != nil {
			t.Fatal(err)
		}
		graphs = append(graphs, graph)
	}

	// Add all of the root transactions to the blockchain to throw off the
	// parent math off for the transaction pool.
	for _, graph := range graphs {
		err := tpt.tpool.AcceptTransactionSet([]types.Transaction{graph[0]})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Add all of the transactions in each graph into the tpool, one transaction
	// at a time, interweaved, chaotically.
	for i := 1; i < len(graphs[0]); i++ {
		for j := 0; j < len(graphs); j++ {
			err := tpt.tpool.AcceptTransactionSet([]types.Transaction{graphs[j][i]})
			if err != nil {
				t.Fatal(err, i, j)
			}
		}
	}

	// Mine blocks until the tpool is gone.
	for tpt.tpool.transactionListSize > 0 {
		_, err := tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestHeapFees creates a large number of transaction graphs with increasing fee
// value. Then it checks that those sets with higher value transaction fees are
// prioritized for placement in blocks.
func TestHeapFees(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Mine a few more blocks to get some extra funding.
	for i := 0; i < 4; i++ {
		_, err := tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create transaction graph setup.
	coinFrac := types.SiacoinPrecision
	numGraphs := 110
	graphFund := coinFrac.Mul64(12210)
	var outputs []types.SiacoinOutput
	for i := 0; i < numGraphs; i++ {
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
	// transaction sets. This guarantees that the parent of every graph will be
	// its own output.
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	finalTxn := txns[len(txns)-1]
	// For each output, create 250 transactions
	var graphs [][]types.Transaction
	for i := 0; i < numGraphs; i++ {
		var edges []types.TransactionGraphEdge
		var cumFee types.Currency
		for j := 0; j < numGraphs; j++ {
			fee := coinFrac.Mul64(uint64((j + 1)))
			cumFee = cumFee.Add(fee)
			edges = append(edges, types.TransactionGraphEdge{
				Dest:   j + 1,
				Fee:    fee,
				Source: 0,
				Value:  fee,
			})
		}
		for k := 0; k < numGraphs; k++ {
			fee := coinFrac.Mul64(uint64(k + 1)).Div64(2)
			cumFee = cumFee.Add(fee)
			edges = append(edges, types.TransactionGraphEdge{
				Dest:   k + 251,
				Fee:    fee,
				Source: k + 1,
				Value:  fee,
			})
		}
		graph, err := types.TransactionGraph(finalTxn.SiacoinOutputID(uint64(i)), edges)
		if err != nil {
			t.Fatal(err)
		}
		graphs = append(graphs, graph)

	}
	// Accept the parent node of each graph so that its outputs we can test
	// spending its outputs after mining the next block.
	for _, graph := range graphs {
		err := tpt.tpool.AcceptTransactionSet([]types.Transaction{graph[0]})
		if err != nil {
			t.Fatal(err)
		}
	}
	block, err := tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Now accept all the other nodes of each graph.
	for _, graph := range graphs {
		for _, txn := range graph[1:] {
			err := tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	// Now we mine 2 blocks in sequence and check that higher fee transactions
	// show up to the first block.
	block, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	var totalFee1 types.Currency
	expectedFee1 := coinFrac.Mul64(321915)

	// Add up total fees
	numTxns1 := 0
	maxFee1 := types.SiacoinPrecision.Div64(1000000)
	minFee1 := types.SiacoinPrecision.Mul64(1000000)
	for _, txn := range block.Transactions {
		for _, fee := range txn.MinerFees {
			if fee.Cmp(maxFee1) >= 0 {
				maxFee1 = fee
			}
			if fee.Cmp(minFee1) <= 0 {
				minFee1 = fee
			}
			totalFee1 = totalFee1.Add(fee)
			numTxns1++
		}
	}
	avgFee1 := totalFee1.Div64(uint64(numTxns1))
	if totalFee1.Cmp(expectedFee1) != 0 {
		t.Error("totalFee1 different than expected fee.", totalFee1.String(), expectedFee1.String())
		//t.Log(totalFee1.Sub(expectedFee1).HumanString())

	}

	// Mine the next block so we can check the transactions inside
	block, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	var totalFee2 types.Currency
	expectedFee2 := coinFrac.Mul64(13860)

	// Add up total fees
	numTxns2 := 0
	maxFee2 := types.SiacoinPrecision.Div64(1000000)
	minFee2 := types.SiacoinPrecision.Mul64(1000000)
	for _, txn := range block.Transactions {
		for _, fee := range txn.MinerFees {
			if fee.Cmp(maxFee2) >= 0 {
				maxFee2 = fee
			}
			if fee.Cmp(minFee2) <= 0 {
				minFee2 = fee
			}
			totalFee2 = totalFee2.Add(fee)
			numTxns2++
		}
	}
	avgFee2 := totalFee2.Div64(uint64(numTxns2))
	if totalFee2.Cmp(expectedFee2) != 0 {
		t.Error("totalFee2 different than expected fee.", totalFee2.String(), expectedFee2.String())
		//t.Log(totalFee2.Sub(expectedFee2).HumanString())
	}

	if avgFee1.Cmp(avgFee2) <= 0 {
		t.Error("Expected average fee from first block to be greater than average fee from second block.")
	}

	if totalFee1.Cmp(totalFee2) <= 0 {
		t.Error("Expected total fee from first block to be greater than total fee from second block.")
	}

	if numTxns1 < numTxns2 {
		t.Error("Expected more transactions in the first block than second block.")
	}

	if maxFee1.Cmp(maxFee2) <= 0 {
		t.Error("Expected highest fee from first block to be greater than highest fee from second block.")
	}

	if minFee1.Cmp(maxFee2) < 0 {
		t.Error("Expected lowest fee from first block to be greater than or equal to than highest fee from second block.")
	}

	if maxFee1.Cmp(minFee1) <= 0 {
		t.Error("Expected highest fee from first block to be greater than lowest fee from first block.")
	}

	if maxFee2.Cmp(minFee2) <= 0 {
		t.Error("Expected highest fee from second block to be greater than lowest fee from second block.")
	}

}
