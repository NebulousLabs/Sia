package consensus

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// validateBlockParamsGot stores the parameters passed to the most recent call
	// to mockBlockValidator.ValidateBlock.
	validateBlockParamsGot validateBlockParams

	mockValidBlock = types.Block{
		Timestamp: 100,
		ParentID:  mockParentID(),
	}
	mockInvalidBlock = types.Block{
		Timestamp: 500,
		ParentID:  mockParentID(),
	}
	// parentBlockSerialized is a mock serialized form of a processedBlock.
	parentBlockSerialized = []byte{3, 2, 1}

	parentBlockUnmarshaler = mockBlockMarshaler{
		[]predefinedBlockUnmarshal{
			{parentBlockSerialized, mockParent(), nil},
		},
	}

	unmarshalFailedErr = errors.New("mock unmarshal failed")

	failingBlockUnmarshaler = mockBlockMarshaler{
		[]predefinedBlockUnmarshal{
			{parentBlockSerialized, processedBlock{}, unmarshalFailedErr},
		},
	}

	serializedParentBlockMap = []blockMapPair{
		{mockValidBlock.ParentID[:], parentBlockSerialized},
	}
)

type (
	// mockDbBucket is an implementation of dbBucket for unit testing.
	mockDbBucket struct {
		values map[string][]byte
	}

	// mockDbTx is an implementation of dbTx for unit testing. It uses an
	// in-memory key/value store to mock a database.
	mockDbTx struct {
		buckets map[string]dbBucket
	}

	// predefinedBlockUnmarshal is a predefined response from mockBlockMarshaler.
	// It defines the unmarshaled processedBlock and error code that
	// mockBlockMarshaler should return in response to an input serialized byte
	// slice.
	predefinedBlockUnmarshal struct {
		serialized  []byte
		unmarshaled processedBlock
		err         error
	}

	// mockBlockMarshaler is an implementation of the encoding.GenericMarshaler
	// interface for unit testing. It allows clients to specify mappings of
	// serialized bytes into unmarshaled blocks.
	mockBlockMarshaler struct {
		p []predefinedBlockUnmarshal
	}

	// mockBlockRuleHelper is an implementation of the blockRuleHelper interface
	// for unit testing.
	mockBlockRuleHelper struct {
		minTimestamp types.Timestamp
	}

	// mockBlockValidator is an implementation of the blockValidator interface for
	// unit testing.
	mockBlockValidator struct {
		err error
	}

	// validateBlockParams stores the set of parameters passed to ValidateBlock.
	validateBlockParams struct {
		called       bool
		b            types.Block
		minTimestamp types.Timestamp
		target       types.Target
		height       types.BlockHeight
	}

	// blockMapPair represents a key-value pair in the mock block map.
	blockMapPair struct {
		key []byte
		val []byte
	}
)

// Get returns the value associated with a given key.
func (bucket mockDbBucket) Get(key []byte) []byte {
	return bucket.values[string(key)]
}

// Set adds a named value to a mockDbBucket.
func (bucket mockDbBucket) Set(key []byte, value []byte) {
	bucket.values[string(key)] = value
}

// Bucket returns a mock dbBucket object associated with the given bucket name.
func (db mockDbTx) Bucket(name []byte) dbBucket {
	return db.buckets[string(name)]
}

// Marshal is not implemented and panics if called.
func (m mockBlockMarshaler) Marshal(interface{}) []byte {
	panic("not implemented")
}

// Unmarshal unmarshals a byte slice into an object based on a pre-defined map
// of deserialized objects.
func (m mockBlockMarshaler) Unmarshal(b []byte, v interface{}) error {
	for _, pu := range m.p {
		if bytes.Equal(b[:], pu.serialized[:]) {
			pv, ok := v.(*processedBlock)
			if !ok {
				panic("mockBlockMarshaler.Unmarshal expected v to be of type processedBlock")
			}
			*pv = pu.unmarshaled
			return pu.err
		}
	}
	panic("unmarshal failed: predefined unmarshal not found")
}

// AddPredefinedUnmarshal adds a predefinedBlockUnmarshal to mockBlockMarshaler.
func (m *mockBlockMarshaler) AddPredefinedUnmarshal(u predefinedBlockUnmarshal) {
	m.p = append(m.p, u)
}

// minimumValidChildTimestamp returns the minimum timestamp of pb that can be
// considered a valid block.
func (brh mockBlockRuleHelper) minimumValidChildTimestamp(blockMap dbBucket, pb *processedBlock) types.Timestamp {
	return brh.minTimestamp
}

// ValidateBlock stores the parameters it receives and returns the mock error
// defined by mockBlockValidator.err.
func (bv mockBlockValidator) ValidateBlock(b types.Block, minTimestamp types.Timestamp, target types.Target, height types.BlockHeight) error {
	validateBlockParamsGot = validateBlockParams{true, b, minTimestamp, target, height}
	return bv.err
}

// mockParentID returns a mock BlockID value.
func mockParentID() (parentID types.BlockID) {
	parentID[0] = 42
	return parentID
}

// mockParent returns a mock processedBlock with its ChildTarget member
// initialized to a dummy value.
func mockParent() (parent processedBlock) {
	var mockTarget types.Target
	mockTarget[0] = 56
	parent.ChildTarget = mockTarget
	return parent
}

// TestUnitValidateHeader runs a series of unit tests for validateHeader.
func TestUnitValidateHeader(t *testing.T) {
	var tests = []struct {
		block                  types.Block
		dosBlocks              map[types.BlockID]struct{}
		blockMapPairs          []blockMapPair
		earliestValidTimestamp types.Timestamp
		marshaler              mockBlockMarshaler
		useNilBlockMap         bool
		validateBlockErr       error
		errWant                error
		msg                    string
	}{
		{
			block:                  mockValidBlock,
			dosBlocks:              make(map[types.BlockID]struct{}),
			useNilBlockMap:         true,
			earliestValidTimestamp: mockValidBlock.Timestamp,
			marshaler:              parentBlockUnmarshaler,
			errWant:                errNoBlockMap,
			msg:                    "validateHeader should fail when no block map is found in the database",
		},
		{
			block: mockValidBlock,
			// Create a dosBlocks map where mockValidBlock is marked as a bad block.
			dosBlocks: map[types.BlockID]struct{}{
				mockValidBlock.ID(): struct{}{},
			},
			earliestValidTimestamp: mockValidBlock.Timestamp,
			marshaler:              parentBlockUnmarshaler,
			errWant:                errDoSBlock,
			msg:                    "validateHeader should reject known bad blocks",
		},
		{
			block:                  mockValidBlock,
			dosBlocks:              make(map[types.BlockID]struct{}),
			earliestValidTimestamp: mockValidBlock.Timestamp,
			marshaler:              parentBlockUnmarshaler,
			errWant:                errOrphan,
			msg:                    "validateHeader should reject a block if its parent block does not appear in the block database",
		},
		{
			block:                  mockValidBlock,
			dosBlocks:              make(map[types.BlockID]struct{}),
			blockMapPairs:          serializedParentBlockMap,
			earliestValidTimestamp: mockValidBlock.Timestamp,
			marshaler:              failingBlockUnmarshaler,
			errWant:                unmarshalFailedErr,
			msg:                    "validateHeader should fail when unmarshaling the parent block fails",
		},
		{
			block:     mockInvalidBlock,
			dosBlocks: make(map[types.BlockID]struct{}),
			blockMapPairs: []blockMapPair{
				{mockInvalidBlock.ParentID[:], parentBlockSerialized},
			},
			earliestValidTimestamp: mockInvalidBlock.Timestamp,
			marshaler:              parentBlockUnmarshaler,
			validateBlockErr:       errBadMinerPayouts,
			errWant:                errBadMinerPayouts,
			msg:                    "validateHeader should reject a block if ValidateBlock returns an error for the block",
		},
		{
			block:                  mockValidBlock,
			dosBlocks:              make(map[types.BlockID]struct{}),
			blockMapPairs:          serializedParentBlockMap,
			earliestValidTimestamp: mockValidBlock.Timestamp,
			marshaler:              parentBlockUnmarshaler,
			errWant:                nil,
			msg:                    "validateHeader should accept a valid block",
		},
	}
	for _, tt := range tests {
		// Initialize the blockmap in the tx.
		bucket := mockDbBucket{map[string][]byte{}}
		for _, mapPair := range tt.blockMapPairs {
			bucket.Set(mapPair.key, mapPair.val)
		}
		dbBucketMap := map[string]dbBucket{}
		if tt.useNilBlockMap {
			dbBucketMap[string(BlockMap)] = nil
		} else {
			dbBucketMap[string(BlockMap)] = bucket
		}
		tx := mockDbTx{dbBucketMap}

		mockParent := mockParent()
		cs := ConsensusSet{
			dosBlocks: tt.dosBlocks,
			marshaler: tt.marshaler,
			blockRuleHelper: mockBlockRuleHelper{
				minTimestamp: tt.earliestValidTimestamp,
			},
			blockValidator: mockBlockValidator{tt.validateBlockErr},
		}
		// Reset the stored parameters to ValidateBlock.
		validateBlockParamsGot = validateBlockParams{}
		err := cs.validateHeader(tx, tt.block)
		if err != tt.errWant {
			t.Errorf("%s: expected to fail with `%v', got: `%v'", tt.msg, tt.errWant, err)
		}
		if err == nil || validateBlockParamsGot.called {
			if validateBlockParamsGot.b.ID() != tt.block.ID() {
				t.Errorf("%s: incorrect parameter passed to ValidateBlock - got: %v, want: %v", tt.msg, validateBlockParamsGot.b, tt.block)
			}
			if validateBlockParamsGot.minTimestamp != tt.earliestValidTimestamp {
				t.Errorf("%s: incorrect parameter passed to ValidateBlock - got: %v, want: %v", tt.msg, validateBlockParamsGot.minTimestamp, tt.earliestValidTimestamp)
			}
			if validateBlockParamsGot.target != mockParent.ChildTarget {
				t.Errorf("%s: incorrect parameter passed to ValidateBlock - got: %v, want: %v", tt.msg, validateBlockParamsGot.target, mockParent.ChildTarget)
			}
		}
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
	defer cst.Close()

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
	defer cst.Close()

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
	defer cst.Close()

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
	defer cst.Close()

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
	defer cst.Close()

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
	defer cst.Close()

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
		t.Errorf("future block was not added to the consensus set after waiting the appropriate amount of time")
	}
	time.Sleep(time.Second * 3)
	_, err = cst.cs.dbGetBlockMap(solvedBlock.ID())
	if err == errNilItem {
		t.Errorf("Future block not added to consensus set.\nCurrent Timestamp %v\nFutureThreshold: %v\nBlock Timestamp %v\n", types.CurrentTimestamp(), types.FutureThreshold, block.Timestamp)
		time.Sleep(time.Second * 3)
		_, err = cst.cs.dbGetBlockMap(solvedBlock.ID())
		if err == errNilItem {
			t.Error("waiting double long did not help.")
		}
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
	defer cst.Close()
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
	defer cst.Close()

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

// mockGateway implements modules.Gateway to mock the Broadcast method.
type mockGateway struct {
	modules.Gateway
	numBroadcasts int
	mu            sync.RWMutex
}

func (g *mockGateway) Broadcast(string, interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.numBroadcasts++
	return
}

// TestAcceptBlockBroadcasts tests that AcceptBlock broadcasts valid blocks and
// that managedAcceptBlock does not.
func TestAcceptBlockBroadcasts(t *testing.T) {
	cst, err := blankConsensusSetTester("TestAcceptBlockBroadcasts")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	mg := &mockGateway{
		Gateway: cst.cs.gateway,
	}
	cst.cs.gateway = mg

	// Test that Broadcast is called for valid blocks.
	b, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	// Sleep to wait for possible calls to Broadcast to complete. We cannot
	// wait on a channel because we don't know how many times broadcast has
	// been called.
	time.Sleep(1)
	mg.mu.RLock()
	numBroadcasts := mg.numBroadcasts
	mg.mu.RUnlock()
	if numBroadcasts != 1 {
		t.Errorf("expected AcceptBlock to broadcast a valid block 1 time, instead it broadcasted %d times", mg.numBroadcasts)
	}

	// Test that Broadcast is not called for invalid blocks.
	mg.mu.Lock()
	mg.numBroadcasts = 0
	mg.mu.Unlock()
	err = cst.cs.AcceptBlock(types.Block{})
	if err == nil {
		t.Fatal("expected AcceptBlock to error on an invalid block")
	}
	// Sleep one second to wait for a possible call to g.Broadcast.
	time.Sleep(1)
	mg.mu.RLock()
	numBroadcasts = mg.numBroadcasts
	mg.mu.RUnlock()
	if numBroadcasts != 0 {
		t.Error("AcceptBlock broadcasted an invalid block")
	}

	// Test that Broadcast is not called in managedAcceptBlock.
	mg.mu.Lock()
	mg.numBroadcasts = 0
	mg.mu.Unlock()
	b, _ = cst.miner.FindBlock()
	err = cst.cs.managedAcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1)
	mg.mu.RLock()
	numBroadcasts = mg.numBroadcasts
	mg.mu.RUnlock()
	if numBroadcasts != 0 {
		t.Errorf("expected managedAcceptBlock to not broadcast any blocks, instead it broadcasted %d times", mg.numBroadcasts)
	}
}
