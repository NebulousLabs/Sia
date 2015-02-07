package consensus

import (
	"testing"
)

// testBlockTimestamps submits a block to the state with a timestamp that is
// too early and a timestamp that is too late, and verifies that each get
// rejected.
func (a *assistant) testBlockTimestamps() {
	// Create a block with a timestamp that is too early.
	block, err := mineTestingBlock(a.state.CurrentBlock().ID(), a.state.EarliestTimestamp()-1, a.payouts(a.state.Height()+1, ZeroCurrency), nil, a.state.CurrentTarget())
	if err != nil {
		a.tester.Fatal(err)
	}
	err = a.state.AcceptBlock(block)
	if err != EarlyTimestampErr {
		a.tester.Error("unexpected error when submitting a too early timestamp:", err)
	}

	// Create a block with a timestamp that is too late.
	block, err = mineTestingBlock(a.state.CurrentBlock().ID(), currentTime()+10+FutureThreshold, a.payouts(a.state.Height()+1, ZeroCurrency), nil, a.state.CurrentTarget())
	if err != nil {
		a.tester.Fatal(err)
	}
	err = a.state.AcceptBlock(block)
	if err != FutureBlockErr {
		a.tester.Error("unexpected error when submitting a too-early timestamp:", err)
	}
}

// testEmptyBlock adds an empty block to the state and checks for errors.
func (a *assistant) testEmptyBlock() {
	// Get the hash of the state before the block was added.
	beforeStateHash := a.state.StateHash()

	// Mine and submit a block
	block := a.mineAndApplyValidBlock()

	// Get the hash of the state after the block was added.
	afterStateHash := a.state.StateHash()
	if afterStateHash == beforeStateHash {
		a.tester.Error("state hash is unchanged after mining a block")
	}

	// Check that the newly mined block is recognized as the current block.
	if a.state.CurrentBlock().ID() != block.ID() {
		a.tester.Error("the state's current block is not reporting as the recently mined block.")
	}

	// These functions break the convention of only using exported functions.
	// But they provide useful checks by making sure that the internals of the
	// state have established in the necessary ways.
	if a.state.currentPath[a.state.Height()] != block.ID() {
		a.tester.Error("the state's current path didn't update correctly after accepting a new block")
	}
	bn, exists := a.state.blockMap[block.ID()]
	if !exists {
		a.tester.Error("the state's block map did not update correctly after getting an empty block")
	}
	if !bn.diffsGenerated {
		a.tester.Error("diffs were not generated on the new block")
	}

	// These functions manipulate the state using unexported functions, which
	// breaks proposed conventions. However, they provide useful information
	// about the accuracy of invertRecentBlock and applyBlockNode.
	a.state.invertRecentBlock()
	if beforeStateHash != a.state.StateHash() {
		a.tester.Error("state is different after applying and removing diffs")
	}
	a.state.applyBlockNode(bn)
	if afterStateHash != a.state.StateHash() {
		a.tester.Error("state is different after generateApply, remove, and applying diffs")
	}
}

// testLargeBlock creates a block that is too large to be accepted by the state
// and checks that it actually gets rejected.
func (a *assistant) testLargeBlock() {
	// Create a transaction that puts the block over the size limit.
	txns := make([]Transaction, 1)
	bigData := string(make([]byte, BlockSizeLimit))
	txns[0] = Transaction{
		ArbitraryData: []string{bigData},
	}

	// Mine and submit a block, checking for the too large error.
	block, err := a.mineCurrentBlock(a.payouts(a.state.Height()+1, ZeroCurrency), txns)
	if err != nil {
		a.tester.Fatal(err)
	}
	err = a.state.AcceptBlock(block)
	if err != LargeBlockErr {
		a.tester.Error(err)
	}
}

// testSinglePayout creates a block with a single miner payout. An incorrect
// and a correct payout get submitted.
func (a *assistant) testSingleNoFeePayout() {
	// Mine a block that has no fees, and an incorrect payout. Compare the
	// before and after state hashes to see that they match.
	beforeHash := a.state.StateHash()
	payouts := []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(a.state.Height()), UnlockHash: ZeroAddress}}
	block, err := a.mineCurrentBlock(payouts, nil)
	if err != nil {
		a.tester.Fatal(err)
	}
	err = a.state.AcceptBlock(block)
	if err != MinerPayoutErr {
		a.tester.Error("Expecting miner payout error:", err)
	}
	afterHash := a.state.StateHash()
	if beforeHash != afterHash {
		a.tester.Error("state changed after invalid payouts")
	}

	// Mine a block that has no fees, and a correct payout, then check that the
	// payout made it into the delayedOutputs list.
	payouts = []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(a.state.Height() + 1), UnlockHash: ZeroAddress}}
	block, err = a.mineCurrentBlock(payouts, nil)
	if err != nil {
		a.tester.Fatal(err)
	}
	err = a.state.AcceptBlock(block)
	if err != nil {
		a.tester.Error("Expecting nil error:", err)
	}
	// Checking the state for correctness requires using an internal function.
	payoutID := block.MinerPayoutID(0)
	output, exists := a.state.delayedSiacoinOutputs[a.state.Height()][payoutID]
	if !exists {
		a.tester.Error("could not find payout in delayedOutputs")
	}
	if output.Value.Cmp(CalculateCoinbase(a.state.Height())) != 0 {
		a.tester.Error("payout dooes not pay the correct amount")
	}
}

// testMinerPayouts tries to submit miner payouts in various legal and illegal
// forms and verifies that the state handles the payouts correctly each time.
//
// CONTRIBUTE: We need to test across multiple payouts, multiple fees, payouts
// that are too high, payouts that are too low, and several other potential
// ways that someone might slip illegal payouts through.
func testMinerPayouts(t *testing.T, s *State) {
	// Create a block with multiple miner payouts.
	var sc UnlockConditions
	coinbasePayout := CalculateCoinbase(s.Height() + 1)
	coinbasePayout.Sub(NewCurrency64(750))
	payout := []SiacoinOutput{
		SiacoinOutput{Value: coinbasePayout, UnlockHash: sc.UnlockHash()},
		SiacoinOutput{Value: NewCurrency64(250), UnlockHash: sc.UnlockHash()},
		SiacoinOutput{Value: NewCurrency64(500), UnlockHash: sc.UnlockHash()},
	}
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), payout, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}
	// Check that all three payouts made it into the output list.
	_, exists := s.siacoinOutputs[b.MinerPayoutID(0)]
	if !exists {
		t.Error("miner payout not found in the list of unspent outputs")
	}
	_, exists = s.siacoinOutputs[b.MinerPayoutID(1)]
	output250 := b.MinerPayoutID(1)
	if !exists {
		t.Error("miner payout not found in the list of unspent outputs")
	}
	_, exists = s.siacoinOutputs[b.MinerPayoutID(2)]
	output500 := b.MinerPayoutID(2)
	if !exists {
		t.Error("miner payout not found in the list of unspent outputs")
	}

	// Test legal multiple payouts when there are multiple miner fees.
	txn1 := Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{ParentID: output250},
		},
		MinerFees: []Currency{
			NewCurrency64(50),
			NewCurrency64(75),
			NewCurrency64(125),
		},
	}
	txn2 := Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{ParentID: output500},
		},
		MinerFees: []Currency{
			NewCurrency64(100),
			NewCurrency64(150),
			NewCurrency64(250),
		},
	}
	coinbasePayout = CalculateCoinbase(s.Height() + 1)
	coinbasePayout.Add(NewCurrency64(25))
	payout = []SiacoinOutput{
		SiacoinOutput{Value: coinbasePayout},
		SiacoinOutput{Value: NewCurrency64(650), UnlockHash: sc.UnlockHash()},
		SiacoinOutput{Value: NewCurrency64(75), UnlockHash: sc.UnlockHash()},
	}
	b, err = mineTestingBlock(s.CurrentBlock().ID(), currentTime(), payout, []Transaction{txn1, txn2}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}
	// Check that the payout outputs made it into the state.
	_, exists = s.siacoinOutputs[b.MinerPayoutID(0)]
	if !exists {
		t.Error("miner payout did not make it into the state")
	}
	_, exists = s.siacoinOutputs[b.MinerPayoutID(1)]
	output650 := b.MinerPayoutID(1)
	if !exists {
		t.Error("miner payout did not make it into the state")
	}
	_, exists = s.siacoinOutputs[b.MinerPayoutID(2)]
	output75 := b.MinerPayoutID(2)
	if !exists {
		t.Error("miner payout did not make it into the state")
	}

	// Test too large multiple payouts when there are multiple miner fees.
	txn1 = Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{ParentID: output650},
		},
		MinerFees: []Currency{
			NewCurrency64(100),
			NewCurrency64(50),
			NewCurrency64(500),
		},
	}
	txn2 = Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{ParentID: output75},
		},
		MinerFees: []Currency{
			NewCurrency64(10),
			NewCurrency64(15),
			NewCurrency64(50),
		},
	}
	coinbasePayout = CalculateCoinbase(s.Height() + 1)
	coinbasePayout.Add(NewCurrency64(25))
	payout = []SiacoinOutput{
		SiacoinOutput{Value: coinbasePayout},
		SiacoinOutput{Value: NewCurrency64(650), UnlockHash: sc.UnlockHash()},
		SiacoinOutput{Value: NewCurrency64(75), UnlockHash: sc.UnlockHash()},
	}
	b, err = mineTestingBlock(s.CurrentBlock().ID(), currentTime(), payout, []Transaction{txn1, txn2}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != MinerPayoutErr {
		t.Error("Expecting different error:", err)
	}
}

// testMissedTarget tries to submit a block that does not meet the target for the next block.
func testMissedTarget(t *testing.T, s *State) {
	// Mine a block that doesn't meet the target.
	b := Block{
		ParentID:  s.CurrentBlock().ID(),
		Timestamp: currentTime(),
	}
	for b.CheckTarget(s.CurrentTarget()) && b.Nonce < 1000*1000 {
		b.Nonce++
	}
	if b.CheckTarget(s.CurrentTarget()) {
		panic("unable to mine a block with a failing target (lol)")
	}

	err := s.AcceptBlock(b)
	if err != MissedTargetErr {
		t.Error("Block with low target is not being rejected")
	}
}

// testRepeatBlock submits a block to the state, and then submits the same
// block to the state. If anything in the state has changed, an error is noted.
func testRepeatBlock(t *testing.T, s *State) {
	// Add a non-repeat block to the state.
	b, err := mineValidBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Collect metrics about the state.
	bbLen := len(s.badBlocks)
	bmLen := len(s.blockMap)
	cpLen := len(s.currentPath)
	uoLen := len(s.siacoinOutputs)
	ocLen := len(s.fileContracts)
	stateHash := s.StateHash()

	// Submit the repeat block.
	err = s.AcceptBlock(b)
	if err != BlockKnownErr {
		t.Error("expecting BlockKnownErr, got", err)
	}

	// Compare the metrics and report an error if something has changed.
	if bbLen != len(s.badBlocks) ||
		bmLen != len(s.blockMap) ||
		cpLen != len(s.currentPath) ||
		uoLen != len(s.siacoinOutputs) ||
		ocLen != len(s.fileContracts) ||
		stateHash != s.StateHash() {
		t.Error("state changed after getting a repeat block.")
	}
}

// TestBlockTimestamps creates a new testing environment and uses it to call
// TestBlockTimestamps.
func TestBlockTimestamps(t *testing.T) {
	a := newTestingEnvironment(t)
	a.testBlockTimestamps()
}

// TestEmptyBlock creates a new testing environment and uses it to call
// testEmptyBlock.
func TestEmptyBlock(t *testing.T) {
	a := newTestingEnvironment(t)
	a.testEmptyBlock()
}

// TestLargeBlock creates a new testing environment and uses it to call
// testLargeBlock.
func TestLargeBlock(t *testing.T) {
	a := newTestingEnvironment(t)
	a.testLargeBlock()
}

// TestSingleNoFeePayouts creates a new testing environment and uses it to call
// testSingleNoFeePayouts.
func TestSingleNoFeePayouts(t *testing.T) {
	a := newTestingEnvironment(t)
	a.testLargeBlock()
}

/*
// TestMinerPayouts creates a new state and uses it to call testMinerPayouts.
func TestMinerPayouts(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testMinerPayouts(t, s)
}

// TestMissedTarget creates a new state and uses it to call testMissedTarget.
func TestMissedTarget(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testMissedTarget(t, s)
}

// TestRepeatBlock creates a new state and uses it to call testRepeatBlock.
func TestRepeatBlock(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testRepeatBlock(t, s)
}
*/

// TODO: Complex transaction building => Financial transactions, contract
// transactions, and invalid forms of each. Bad outputs, many outputs, many
// inputs, many fees, bad fees, overflows, bad proofs, early proofs, arbitrary
// datas, bad signatures, too many signatures, repeat signatures.
//
// Build those transaction building functions as separate things, because
// you want to be able to probe complex transactions that have lots of juicy
// stuff.

// TODO: Test the actual method which is used to calculate the earliest legal
// timestamp for the next block. Like have some examples that should work out
// algebraically and make sure that earliest timestamp follows the rules layed
// out by the protocol. This should be done after we decide that the algorithm
// for calculating the earliest allowed timestamp is sufficient.

// TODO: Probe the target adjustments, make sure that they are happening
// according to specification, moving as much as they should and that the
// clamps are being effective.

// TODO: Submit orphan blocks that have errors in them.

// TODO: Make sure that the code operates correctly when a block is found with
// an error halfway through validation.
