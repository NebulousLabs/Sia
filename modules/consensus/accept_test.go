package consensus

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// mockDbBucket is an implementation of dbBucket for unit testing.
type mockDbBucket struct{}

// Get returns the value associated with a given key.
func (bucket mockDbBucket) Get(key []byte) []byte {
	return nil
}

// mockDbTx is an implementation of dbTx for unit testing. It uses an in-memory
// key/value store to mock a database.
type mockDbTx struct {
	buckets map[string]dbBucket
}

// Bucket returns a mock dbBucket object associated with the given bucket name.
func (db mockDbTx) Bucket(name []byte) dbBucket {
	return db.buckets[string(name)]
}

// TestUnitValidateHeaderFailsWhenBlockMapDoesNotExist checks that
// validateHeader correctly rejects blocks when no block map is found in the
// database.
func TestUnitValidateHeaderFailsWhenBlockMapDoesNotExist(t *testing.T) {
	// TODO(mtlynch): Fix block and tx so that it does not assume an ordering of
	// validation checks in validateHeader (i.e. everything should be valid except
	// the missing block map).
	block := types.Block{Timestamp: 100}

	// Create a transaction with no associated DB buckets.
	tx := mockDbTx{}
	cs := ConsensusSet{}
	err := cs.validateHeader(tx, block)
	if err != errNoBlockMap {
		t.Fatalf("expected to fail with errNoBlockMap, got: %v", err)
	}
}

// TestUnitValidateHeaderRejectsKnownBadBlocks checks that validateHeader
// correctly rejects saved bad blocks.
func TestUnitValidateHeaderRejectsKnownBadBlocks(t *testing.T) {
	// TODO(mtlynch): Fix block and tx so that it does not assume an ordering of
	// validation checks in validateHeader (i.e. everything should be valid except
	// the known bad block).
	block := types.Block{Timestamp: 100}
	dosBlocks := map[types.BlockID]struct{}{
		block.ID(): struct{}{},
	}

	tx := mockDbTx{}
	cs := ConsensusSet{dosBlocks: dosBlocks}
	err := cs.validateHeader(tx, block)
	if err != errDoSBlock {
		t.Fatalf("expected to fail with errDoSBlock, got: %v", err)
	}
}

// TestUnitValidateHeaderRejectsOrphanBlocks checks that validateHeader rejects
// a block if its parent block does not appear in the block database.
func TestUnitValidateHeaderRejectsOrphanBlocks(t *testing.T) {
	// TODO(mtlynch): Fix block and tx so that it does not assume an ordering of
	// validation checks in validateHeader (i.e. everything should be valid except
	// the known bad block).
	block := types.Block{Timestamp: 100}
	dosBlocks := map[types.BlockID]struct{}{}

	tx := mockDbTx{map[string]dbBucket{
		string(BlockMap): mockDbBucket{},
	}}
	cs := ConsensusSet{dosBlocks: dosBlocks}
	err := cs.validateHeader(tx, block)
	if err != errOrphan {
		t.Fatalf("expected to fail with errOrphan, got: %v", err)
	}
}

// TestIntegrationDoSBlockHandling checks that saved bad blocks are correctly
// ignored.
func TestIntegrationDoSBlockHandling(t *testing.T) {
	// TestIntegrationDoSBlockHandling catches a wide array of simple errors,
	// and therefore is included in the short tests despite being somewhat
	// computationally expensive.
	cst, err := createConsensusSetTester("TestIntegrationDoSBlockHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Mine a block that is valid except for containing a buried invalid
	// transaction. The transaction has more siacoin inputs than outputs.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(types.NewCurrency64(50))
	if err != nil {
		t.Fatal(err)
	}
	txnSet, err := txnBuilder.Sign(true) // true sets the 'wholeTransaction' flag
	if err != nil {
		t.Fatal(err)
	}

	// Mine and submit the invalid block to the consensus set. The first time
	// around, the complaint should be about the rule-breaking transaction.
	block, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.Transactions = append(block.Transactions, txnSet...)
	dosBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.AcceptBlock(dosBlock)
	if err != errSiacoinInputOutputMismatch {
		t.Fatalf("expected %v, got %v", errSiacoinInputOutputMismatch, err)
	}

	// Submit the same block a second time. The complaint should be that the
	// block is already known to be invalid.
	err = cst.cs.AcceptBlock(dosBlock)
	if err != errDoSBlock {
		t.Fatalf("expected %v, got %v", errDoSBlock, err)
	}
}

// TestBlockKnownHandling submits known blocks to the consensus set.
func TestBlockKnownHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBlockKnownHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Get a block destined to be stale.
	block, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	staleBlock, _ := cst.miner.SolveBlock(block, target)

	// Add two new blocks to the consensus set to block the stale block.
	block1, err := cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	block2, err := cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Submit the stale block.
	err = cst.cs.AcceptBlock(staleBlock)
	if err != nil && err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}

	// Submit all the blocks again, looking for a 'stale block' error.
	err = cst.cs.AcceptBlock(block1)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}
	err = cst.cs.AcceptBlock(block2)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}
	err = cst.cs.AcceptBlock(staleBlock)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}

	// Try submitting the genesis block.
	id, err := cst.cs.dbGetPath(0)
	if err != nil {
		t.Fatal(err)
	}
	genesisBlock, err := cst.cs.dbGetBlockMap(id)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.cs.AcceptBlock(genesisBlock.Block)
	if err != modules.ErrBlockKnown {
		t.Fatalf("expected %v, got %v", modules.ErrBlockKnown, err)
	}
}

// TestOrphanHandling passes an orphan block to the consensus set.
func TestOrphanHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestOrphanHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Try submitting an orphan block to the consensus set. The empty block can
	// be used, because looking for a parent is one of the first checks the
	// consensus set performs.
	orphan := types.Block{}
	err = cst.cs.AcceptBlock(orphan)
	if err != errOrphan {
		t.Fatalf("expected %v, got %v", errOrphan, err)
	}
	err = cst.cs.AcceptBlock(orphan)
	if err != errOrphan {
		t.Fatalf("expected %v, got %v", errOrphan, err)
	}
}

// TestMissedTarget submits a block that does not meet the required target.
func TestMissedTarget(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMissedTarget")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Mine a block that doesn't meet the target.
	block, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	for checkTarget(block, target) && block.Nonce[0] != 255 {
		block.Nonce[0]++
	}
	if checkTarget(block, target) {
		t.Fatal("unable to find a failing target")
	}
	err = cst.cs.AcceptBlock(block)
	if err != modules.ErrBlockUnsolved {
		t.Fatalf("expected %v, got %v", modules.ErrBlockUnsolved, err)
	}
}

// TestMinerPayoutHandling checks that blocks with incorrect payouts are
// rejected.
func TestMinerPayoutHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMinerPayoutHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block with the wrong miner payout structure - testing can be
	// light here because there is heavier testing in the 'types' package,
	// where the logic is defined.
	block, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.MinerPayouts = append(block.MinerPayouts, types.SiacoinOutput{Value: types.NewCurrency64(1)})
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.AcceptBlock(solvedBlock)
	if err != errBadMinerPayouts {
		t.Fatalf("expected %v, got %v", errBadMinerPayouts, err)
	}
}

// testFutureTimestampHandling checks that blocks in the future (but not
// extreme future) are handled correctly.
func TestFutureTimestampHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestFutureTimestampHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Submit a block with a timestamp in the future, but not the extreme
	// future.
	block, target, err := cst.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	block.Timestamp = types.CurrentTimestamp() + 2 + types.FutureThreshold
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.AcceptBlock(solvedBlock)
	if err != errFutureTimestamp {
		t.Fatalf("expected %v, got %v", errFutureTimestamp, err)
	}

	// Check that after waiting until the block is no longer too far in the
	// future, the block gets added to the consensus set.
	time.Sleep(time.Second * 3) // 3 seconds, as the block was originally 2 seconds too far into the future.
	_, err = cst.cs.dbGetBlockMap(solvedBlock.ID())
	if err == errNilItem {
		t.Fatalf("future block was not added to the consensus set after waiting the appropriate amount of time")
	}
}

// TestBuriedBadTransaction tries submitting a block with a bad transaction
// that is buried under good transactions.
func TestBuriedBadTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBuriedBadTransaction")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	pb := cst.cs.dbCurrentProcessedBlock()

	// Create a good transaction using the wallet.
	txnValue := types.NewCurrency64(1200)
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(txnValue)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: txnValue})
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}

	// Create a bad transaction
	badTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{}},
	}
	txns := append(cst.tpool.TransactionList(), badTxn)

	// Create a block with a buried bad transaction.
	block := types.Block{
		ParentID:     pb.Block.ID(),
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height + 1)}},
		Transactions: txns,
	}
	block, _ = cst.miner.SolveBlock(block, pb.ChildTarget)
	err = cst.cs.AcceptBlock(block)
	if err == nil {
		t.Error("buried transaction didn't cause an error")
	}
}

// TestInconsistencyCheck puts the consensus set in to an inconsistent state
// and makes sure that the santiy checks are triggering panics.
func TestInconsistentCheck(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestInconsistentCheck")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Corrupt the consensus set by adding a new siafund output.
	sfo := types.SiafundOutput{
		Value: types.NewCurrency64(1),
	}
	cst.cs.dbAddSiafundOutput(types.SiafundOutputID{}, sfo)

	// Catch a panic that should be caused by the inconsistency check after a
	// block is mined.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("inconsistency panic not triggered by corrupted database")
		}
	}()
	cst.miner.AddBlock()
}

// COMPATv0.4.0
//
// This test checks that the hardfork scheduled for block 21,000 rolls through
// smoothly.
func TestTaxHardfork(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestTaxHardfork")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a file contract with a payout that is put into the blockchain
	// before the hardfork block but expires after the hardfork block.
	payout := types.NewCurrency64(400e6)
	outputSize := types.PostTax(cst.cs.dbBlockHeight(), payout)
	fc := types.FileContract{
		WindowStart:        cst.cs.dbBlockHeight() + 12,
		WindowEnd:          cst.cs.dbBlockHeight() + 14,
		Payout:             payout,
		ValidProofOutputs:  []types.SiacoinOutput{{Value: outputSize}},
		MissedProofOutputs: []types.SiacoinOutput{{Value: outputSize}},
		UnlockHash:         types.UnlockConditions{}.UnlockHash(), // The empty UC is anyone-can-spend
	}

	// Create and fund a transaction with a file contract.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}
	fcIndex := txnBuilder.AddFileContract(fc)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the siafund pool was increased by the faulty float amount.
	siafundPool := cst.cs.dbGetSiafundPool()
	if siafundPool.Cmp(types.NewCurrency64(15590e3)) != 0 {
		t.Fatal("siafund pool was not increased correctly")
	}

	// Mine blocks until the hardfork is reached.
	for i := 0; i < 10; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Submit a file contract revision and check that the payouts are able to
	// be the same.
	fcid := txnSet[len(txnSet)-1].FileContractID(fcIndex)
	fcr := types.FileContractRevision{
		ParentID:          fcid,
		UnlockConditions:  types.UnlockConditions{},
		NewRevisionNumber: 1,

		NewFileSize:           1,
		NewWindowStart:        cst.cs.dbBlockHeight() + 2,
		NewWindowEnd:          cst.cs.dbBlockHeight() + 4,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
	}
	txnBuilder = cst.wallet.StartTransaction()
	txnBuilder.AddFileContractRevision(fcr)
	txnSet, err = txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Mine blocks until the revision goes through, such that the sanity checks
	// can be run.
	for i := 0; i < 6; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check that the siafund pool did not change after the submitted revision.
	siafundPool = cst.cs.dbGetSiafundPool()
	if siafundPool.Cmp(types.NewCurrency64(15590e3)) != 0 {
		t.Fatal("siafund pool was not increased correctly")
	}
}
