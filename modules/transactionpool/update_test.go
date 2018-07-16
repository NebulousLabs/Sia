package transactionpool

import (
	"sort"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"
)

// TestFindSets checks that the findSets functions is properly parsing and
// combining transactions into their minimal sets.
func TestFindSets(t *testing.T) {
	// Graph a graph which is a chain. Graph will be invalid, but we don't need
	// the consensus set, so no worries.
	graph1Size := 5
	edges := make([]types.TransactionGraphEdge, 0, graph1Size)
	for i := 0; i < graph1Size; i++ {
		edges = append(edges, types.TransactionGraphEdge{
			Dest:   i + 1,
			Fee:    types.NewCurrency64(5),
			Source: i,
			Value:  types.NewCurrency64(100),
		})
	}
	graph1, err := types.TransactionGraph(types.SiacoinOutputID{}, edges)
	if err != nil {
		t.Fatal(err)
	}

	// Split the graph using findSets. Result should be a single set with 5
	// transactions.
	sets := findSets(graph1)
	if len(sets) != 1 {
		t.Fatal("there should be only one set")
	}
	if len(sets[0]) != graph1Size {
		t.Error("findSets is not grouping the transactions correctly")
	}

	// Create a second graph to check it can handle two graphs.
	graph2Size := 6
	edges = make([]types.TransactionGraphEdge, 0, graph2Size)
	for i := 0; i < graph2Size; i++ {
		edges = append(edges, types.TransactionGraphEdge{
			Dest:   i + 1,
			Fee:    types.NewCurrency64(5),
			Source: i,
			Value:  types.NewCurrency64(100),
		})
	}
	graph2, err := types.TransactionGraph(types.SiacoinOutputID{1}, edges)
	if err != nil {
		t.Fatal(err)
	}
	sets = findSets(append(graph1, graph2...))
	if len(sets) != 2 {
		t.Fatal("there should be two sets")
	}
	lens := []int{len(sets[0]), len(sets[1])}
	sort.Ints(lens)
	expected := []int{graph1Size, graph2Size}
	sort.Ints(expected)
	if lens[0] != expected[0] || lens[1] != expected[1] {
		t.Error("Resulting sets do not have the right lengths")
	}

	// Create a diamond graph to make sure it can handle diamond graph.
	edges = make([]types.TransactionGraphEdge, 0, 5)
	sources := []int{0, 0, 1, 2, 3}
	dests := []int{1, 2, 3, 3, 4}
	for i := 0; i < 5; i++ {
		edges = append(edges, types.TransactionGraphEdge{
			Dest:   dests[i],
			Fee:    types.NewCurrency64(5),
			Source: sources[i],
			Value:  types.NewCurrency64(100),
		})
	}
	graph3, err := types.TransactionGraph(types.SiacoinOutputID{2}, edges)
	graph3Size := len(graph3)
	if err != nil {
		t.Fatal(err)
	}
	sets = findSets(append(graph1, append(graph2, graph3...)...))
	if len(sets) != 3 {
		t.Fatal("there should be two sets")
	}
	lens = []int{len(sets[0]), len(sets[1]), len(sets[2])}
	sort.Ints(lens)
	expected = []int{graph1Size, graph2Size, graph3Size}
	sort.Ints(expected)
	if lens[0] != expected[0] || lens[1] != expected[1] || lens[2] != expected[2] {
		t.Error("Resulting sets do not have the right lengths")
	}

	// Sporadically weave the transactions and make sure the set finder still
	// parses the sets correctly (sets can assumed to be ordered, but not all in
	// a row).
	var sporadic []types.Transaction
	for len(graph1) > 0 || len(graph2) > 0 || len(graph3) > 0 {
		if len(graph1) > 0 {
			sporadic = append(sporadic, graph1[0])
			graph1 = graph1[1:]
		}
		if len(graph2) > 0 {
			sporadic = append(sporadic, graph2[0])
			graph2 = graph2[1:]
		}
		if len(graph3) > 0 {
			sporadic = append(sporadic, graph3[0])
			graph3 = graph3[1:]
		}
	}
	if len(sporadic) != graph1Size+graph2Size+graph3Size {
		t.Error("sporadic block creation failed")
	}
	// Result of findSets should match previous result.
	sets = findSets(sporadic)
	if len(sets) != 3 {
		t.Fatal("there should be two sets")
	}
	lens = []int{len(sets[0]), len(sets[1]), len(sets[2])}
	sort.Ints(lens)
	expected = []int{graph1Size, graph2Size, graph3Size}
	sort.Ints(expected)
	if lens[0] != expected[0] || lens[1] != expected[1] || lens[2] != expected[2] {
		t.Error("Resulting sets do not have the right lengths")
	}
}

// TestArbDataOnly tries submitting a transaction with only arbitrary data to
// the transaction pool. Then a block is mined, putting the transaction on the
// blockchain. The arb data transaction should no longer be in the transaction
// pool.
func TestArbDataOnly(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()
	txn := types.Transaction{
		ArbitraryData: [][]byte{
			append(modules.PrefixNonSia[:], []byte("arb-data")...),
		},
	}
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 1 {
		t.Error("expecting to see a transaction in the transaction pool")
	}
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("transaction was not cleared from the transaction pool")
	}
}

// TestValidRevertedTransaction verifies that if a transaction appears in a
// block's reverted transactions, it is added correctly to the pool.
func TestValidRevertedTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()
	tpt2, err := blankTpoolTester(t.Name() + "-tpt2")
	if err != nil {
		t.Fatal(err)
	}
	defer tpt2.Close()

	// connect the testers and wait for them to have the same current block
	err = tpt2.gateway.Connect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success := false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	// disconnect the testers
	err = tpt2.gateway.Disconnect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	tpt.gateway.Disconnect(tpt2.gateway.Address())

	// make some transactions on tpt
	var txnSets [][]types.Transaction
	for i := 0; i < 5; i++ {
		txns, err := tpt.wallet.SendSiacoins(types.SiacoinPrecision.Mul64(1000), types.UnlockHash{})
		if err != nil {
			t.Fatal(err)
		}
		txnSets = append(txnSets, txns)
	}
	// mine some blocks to cause a re-org
	for i := 0; i < 3; i++ {
		_, err = tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// put tpt2 at a higher height
	for i := 0; i < 10; i++ {
		_, err = tpt2.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// connect the testers and wait for them to have the same current block
	err = tpt.gateway.Connect(tpt2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success = false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	// verify the transaction pool still has the reorged txns
	for _, txnSet := range txnSets {
		for _, txn := range txnSet {
			_, _, exists := tpt.tpool.Transaction(txn.ID())
			if !exists {
				t.Error("Transaction was not re-added to the transaction pool after being re-orged out of the blockchain:", txn.ID())
			}
		}
	}

	// Try to get the transactoins into a block.
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("Does not seem that the transactions were added to the transaction pool.")
	}
}

// TestTransactionPoolPruning verifies that the transaction pool correctly
// prunes transactions older than maxTxnAge.
func TestTransactionPoolPruning(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()
	tpt2, err := blankTpoolTester(t.Name() + "-tpt2")
	if err != nil {
		t.Fatal(err)
	}
	defer tpt2.Close()

	// connect the testers and wait for them to have the same current block
	err = tpt2.gateway.Connect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success := false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	// disconnect tpt, create an unconfirmed transaction on tpt, mine maxTxnAge
	// blocks on tpt2 and reconnect. The unconfirmed transactions should be
	// removed from tpt's pool.
	err = tpt.gateway.Disconnect(tpt2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	tpt2.gateway.Disconnect(tpt.gateway.Address())
	txns, err := tpt.wallet.SendSiacoins(types.SiacoinPrecision.Mul64(1000), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	for i := types.BlockHeight(0); i < maxTxnAge+1; i++ {
		_, err = tpt2.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// reconnect the testers
	err = tpt.gateway.Connect(tpt2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success = false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	for _, txn := range txns {
		_, _, exists := tpt.tpool.Transaction(txn.ID())
		if exists {
			t.Fatal("transaction pool had a transaction that should have been pruned")
		}
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Fatal("should have no unconfirmed transactions")
	}
	if len(tpt.tpool.knownObjects) != 0 {
		t.Fatal("should have no known objects")
	}
	if len(tpt.tpool.transactionSetDiffs) != 0 {
		t.Fatal("should have no transaction set diffs")
	}
	if tpt.tpool.transactionListSize != 0 {
		t.Fatal("transactionListSize should be zero")
	}
}

// TestUpdateBlockHeight verifies that the transactionpool updates its internal
// block height correctly.
func TestUpdateBlockHeight(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tpt, err := blankTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	targetHeight := 20
	for i := 0; i < targetHeight; i++ {
		_, err = tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	if tpt.tpool.blockHeight != types.BlockHeight(targetHeight) {
		t.Fatalf("transaction pool had the wrong block height, got %v wanted %v\n", tpt.tpool.blockHeight, targetHeight)
	}
}

// TestDatabaseUpgrade verifies that the database will upgrade correctly from
// v1.3.1 or earlier to the new sanity check persistence, by clearing out the
// persistence at various points in the process of a reorg.
func TestDatabaseUpgrade(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()
	tpt2, err := blankTpoolTester(t.Name() + "-tpt2")
	if err != nil {
		t.Fatal(err)
	}
	defer tpt2.Close()

	// connect the testers and wait for them to have the same current block
	err = tpt2.gateway.Connect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success := false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}

	// disconnect the testers
	err = tpt2.gateway.Disconnect(tpt.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	tpt.gateway.Disconnect(tpt2.gateway.Address())

	// make some transactions on tpt
	var txnSets [][]types.Transaction
	for i := 0; i < 5; i++ {
		txns, err := tpt.wallet.SendSiacoins(types.SiacoinPrecision.Mul64(1000), types.UnlockHash{})
		if err != nil {
			t.Fatal(err)
		}
		txnSets = append(txnSets, txns)
	}
	// mine some blocks to cause a re-org, first clearing the persistence to
	// simulate an un-upgraded database.
	err = tpt.tpool.dbTx.Bucket(bucketRecentConsensusChange).Delete(fieldRecentBlockID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_, err = tpt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// put tpt2 at a higher height
	for i := 0; i < 10; i++ {
		_, err = tpt2.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// connect the testers and wait for them to have the same current block,
	// first clearing the persistence to simulate an un-upgraded database.
	err = tpt.tpool.dbTx.Bucket(bucketRecentConsensusChange).Delete(fieldRecentBlockID)
	if err != nil {
		t.Fatal(err)
	}
	err = tpt.gateway.Connect(tpt2.gateway.Address())
	if err != nil {
		t.Fatal(err)
	}
	success = false
	for start := time.Now(); time.Since(start) < time.Minute; time.Sleep(time.Millisecond * 100) {
		if tpt.cs.CurrentBlock().ID() == tpt2.cs.CurrentBlock().ID() {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("testers did not have the same block height after one minute")
	}
}
