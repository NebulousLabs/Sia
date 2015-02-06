package consensus

import (
	"testing"
	"time"
)

// currentTime returns a Timestamp of the current time.
func currentTime() Timestamp {
	return Timestamp(time.Now().Unix())
}

// mineTestingBlock accepts a bunch of parameters for a block and then grinds
// blocks until a block with the appropriate target is found.
func mineTestingBlock(parent BlockID, timestamp Timestamp, minerPayouts []SiacoinOutput, txns []Transaction, target Target) (b Block, err error) {
	b = Block{
		ParentID:     parent,
		Timestamp:    timestamp,
		MinerPayouts: minerPayouts,
		Transactions: txns,
	}

	for !b.CheckTarget(target) && b.Nonce < 1000*1000 {
		b.Nonce++
	}
	if !b.CheckTarget(target) {
		panic("mineTestingBlock failed!")
	}
	return
}

// nullMinerPayouts returns an []Output for the miner payouts field of a block
// so that the block can be valid. It assumes the block will be at whatever
// height you use as input.
func nullMinerPayouts(height BlockHeight) []SiacoinOutput {
	return []SiacoinOutput{
		SiacoinOutput{
			Value: CalculateCoinbase(height),
		},
	}
}

// mineValidBlock picks valid/legal parameters for a block and then uses them
// to call mineTestingBlock.
func mineValidBlock(s *State) (b Block, err error) {
	return mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), nil, s.CurrentTarget())
}

// testBlockTimestamps submits a block to the state with a timestamp that is
// too early and a timestamp that is too late, and verifies that each get
// rejected.
func testBlockTimestamps(t *testing.T, s *State) {
	// Create a block with a timestamp that is too early.
	b, err := mineTestingBlock(s.CurrentBlock().ID(), s.EarliestTimestamp()-1, nullMinerPayouts(s.Height()+1), nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != EarlyTimestampErr {
		t.Error("unexpected error when submitting a too-early timestamp:", err)
	}

	// Create a block with a timestamp that is too late.
	b, err = mineTestingBlock(s.CurrentBlock().ID(), currentTime()+10+FutureThreshold, nullMinerPayouts(s.Height()+1), nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != FutureBlockErr {
		t.Error("unexpected error when submitting a too-early timestamp:", err)
	}
}

// testEmptyBlock adds an empty block to the state and checks for errors.
func testEmptyBlock(t *testing.T, s *State) {
	// Get prior stats about the state.
	bbLen := len(s.badBlocks)
	bmLen := len(s.blockMap)
	cpLen := len(s.currentPath)
	uoLen := len(s.unspentOutputs)
	ocLen := len(s.openContracts)
	beforeStateHash := s.StateHash()

	// Mine and submit a block
	b, err := mineValidBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	afterStateHash := s.StateHash()
	if afterStateHash == beforeStateHash {
		t.Error("StateHash is unchanged after applying an empty block")
	}

	// Check that the state has updated as expected:
	//		bad blocks should not change
	//		blockMap should get 1 new member
	//		missingParents should not change
	//		currentPath should get 1 new member
	//		unspentOutputs should grow by at least 1 (missedProofs can make it grow by more)
	//		openContracts should not grow (contracts may close during the block though)
	if bbLen != len(s.badBlocks) ||
		bmLen != len(s.blockMap)-1 ||
		cpLen != len(s.currentPath)-1 ||
		uoLen > len(s.unspentOutputs)-1 ||
		ocLen < len(s.openContracts) {
		t.Error("state changed unexpectedly after accepting an empty block")
	}
	if s.currentBlockID != b.ID() {
		t.Error("the state's current block id did not change after getting a new block")
	}
	if s.currentPath[s.Height()] != b.ID() {
		t.Error("the state's current path didn't update correctly after accepting a new block")
	}
	bn, exists := s.blockMap[b.ID()]
	if !exists {
		t.Error("the state's block map did not update correctly after getting an empty block")
	}
	_, exists = s.unspentOutputs[b.MinerPayoutID(0)]
	if !exists {
		t.Error("the blocks subsidy output did not get added to the set of unspent outputs")
	}

	// Check that the diffs have been generated, and that they represent the
	// actual changes to the state.
	if !bn.diffsGenerated {
		t.Error("diffs were not generated on the new block")
	}
	s.invertRecentBlock()
	if beforeStateHash != s.StateHash() {
		t.Error("state is different after applying and removing diffs")
	}
	s.applyBlockNode(bn)
	if afterStateHash != s.StateHash() {
		t.Error("state is different after generateApply, remove, and applying diffs")
	}
}

// testLargeBlock creates a block that is too large to be accepted by the state
// and checks that it actually gets rejected.
func testLargeBlock(t *testing.T, s *State) {
	txns := make([]Transaction, 1)
	bigData := string(make([]byte, BlockSizeLimit))
	txns[0] = Transaction{
		ArbitraryData: []string{bigData},
	}
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), txns, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}

	err = s.AcceptBlock(b)
	if err != LargeBlockErr {
		t.Error(err)
	}
}

// testMinerPayouts tries to submit miner payouts in various legal and illegal
// forms and verifies that the state handles the payouts correctly each time.
//
// CONTRIBUTE: We need to test across multiple payouts, multiple fees, payouts
// that are too high, payouts that are too low, and several other potential
// ways that someone might slip illegal payouts through.
func testMinerPayouts(t *testing.T, s *State) {
	// Create a block with a single legal payout, no miner fees. The payout
	// goes to the hash of the empty spend conditions.
	var sc SpendConditions
	payout := []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(s.Height() + 1), SpendHash: sc.CoinAddress()}}
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), payout, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}
	// Check that the payout made it into the output list.
	_, exists := s.unspentOutputs[b.MinerPayoutID(0)]
	if !exists {
		t.Error("miner payout not found in the list of unspent outputs")
	}

	// Create a block with multiple miner payouts.
	coinbasePayout := CalculateCoinbase(s.Height() + 1)
	coinbasePayout.Sub(NewCurrency64(750))
	payout = []SiacoinOutput{
		SiacoinOutput{Value: coinbasePayout, SpendHash: sc.CoinAddress()},
		SiacoinOutput{Value: NewCurrency64(250), SpendHash: sc.CoinAddress()},
		SiacoinOutput{Value: NewCurrency64(500), SpendHash: sc.CoinAddress()},
	}
	b, err = mineTestingBlock(s.CurrentBlock().ID(), currentTime(), payout, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}
	// Check that all three payouts made it into the output list.
	_, exists = s.unspentOutputs[b.MinerPayoutID(0)]
	if !exists {
		t.Error("miner payout not found in the list of unspent outputs")
	}
	_, exists = s.unspentOutputs[b.MinerPayoutID(1)]
	output250 := b.MinerPayoutID(1)
	if !exists {
		t.Error("miner payout not found in the list of unspent outputs")
	}
	_, exists = s.unspentOutputs[b.MinerPayoutID(2)]
	output500 := b.MinerPayoutID(2)
	if !exists {
		t.Error("miner payout not found in the list of unspent outputs")
	}

	// Create a block with a too large payout.
	payout = []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(s.Height()), SpendHash: sc.CoinAddress()}}
	b, err = mineTestingBlock(s.CurrentBlock().ID(), currentTime(), payout, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != MinerPayoutErr {
		t.Error("Unexpected Error:", err)
	}
	// Check that the payout did not make it into the output list.
	_, exists = s.unspentOutputs[b.MinerPayoutID(0)]
	if exists {
		t.Error("miner payout made it into state despite being invalid.")
	}

	// Create a block with a too small payout.
	payout = []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(s.Height() + 2), SpendHash: sc.CoinAddress()}}
	b, err = mineTestingBlock(s.CurrentBlock().ID(), currentTime(), payout, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != MinerPayoutErr {
		t.Error("Unexpected Error:", err)
	}
	// Check that the payout did not make it into the output list.
	_, exists = s.unspentOutputs[b.MinerPayoutID(0)]
	if exists {
		t.Error("miner payout made it into state despite being invalid.")
	}

	// Test legal multiple payouts when there are multiple miner fees.
	txn1 := Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{OutputID: output250},
		},
		MinerFees: []Currency{
			NewCurrency64(50),
			NewCurrency64(75),
			NewCurrency64(125),
		},
	}
	txn2 := Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{OutputID: output500},
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
		SiacoinOutput{Value: NewCurrency64(650), SpendHash: sc.CoinAddress()},
		SiacoinOutput{Value: NewCurrency64(75), SpendHash: sc.CoinAddress()},
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
	_, exists = s.unspentOutputs[b.MinerPayoutID(0)]
	if !exists {
		t.Error("miner payout did not make it into the state")
	}
	_, exists = s.unspentOutputs[b.MinerPayoutID(1)]
	output650 := b.MinerPayoutID(1)
	if !exists {
		t.Error("miner payout did not make it into the state")
	}
	_, exists = s.unspentOutputs[b.MinerPayoutID(2)]
	output75 := b.MinerPayoutID(2)
	if !exists {
		t.Error("miner payout did not make it into the state")
	}

	// Test too large multiple payouts when there are multiple miner fees.
	txn1 = Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{OutputID: output650},
		},
		MinerFees: []Currency{
			NewCurrency64(100),
			NewCurrency64(50),
			NewCurrency64(500),
		},
	}
	txn2 = Transaction{
		SiacoinInputs: []SiacoinInput{
			SiacoinInput{OutputID: output75},
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
		SiacoinOutput{Value: NewCurrency64(650), SpendHash: sc.CoinAddress()},
		SiacoinOutput{Value: NewCurrency64(75), SpendHash: sc.CoinAddress()},
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
	uoLen := len(s.unspentOutputs)
	ocLen := len(s.openContracts)
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
		uoLen != len(s.unspentOutputs) ||
		ocLen != len(s.openContracts) ||
		stateHash != s.StateHash() {
		t.Error("state changed after getting a repeat block.")
	}
}

// TestBlockTimestamps creates a new state and uses it to call
// testBlockTimestamps.
func TestBlockTimestamps(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testBlockTimestamps(t, s)
}

// TestEmptyBlock creates a new state and uses it to call testEmptyBlock.
func TestEmptyBlock(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testEmptyBlock(t, s)
}

// TestLargeBlock creates a new state and uses it to call testLargeBlock.
func TestLargeBlock(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testLargeBlock(t, s)
}

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
