package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// testBlockTimestamps submits a block to the state with a timestamp that is
// too early and a timestamp that is too late, and verifies that each get
// rejected.
func (ct *ConsensusTester) testBlockTimestamps() {
	// Create a block with a timestamp that is too early.
	block, err := MineTestingBlock(ct.CurrentBlock().ID(), ct.EarliestTimestamp()-1, ct.Payouts(ct.Height()+1, nil), nil, ct.CurrentTarget())
	if err != nil {
		ct.Fatal(err)
	}
	err = ct.AcceptBlock(block)
	if err != ErrEarlyTimestamp {
		ct.Error("unexpected error when submitting a too early timestamp:", err)
	}

	// Create a block with a timestamp that is too late.
	block, err = MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp()+10+FutureThreshold, ct.Payouts(ct.Height()+1, nil), nil, ct.CurrentTarget())
	if err != nil {
		ct.Fatal(err)
	}
	err = ct.AcceptBlock(block)
	if err != ErrFutureTimestamp {
		ct.Error("unexpected error when submitting a too-early timestamp:", err)
	}
}

// testEmptyBlock adds an empty block to the state and checks for errors.
func (ct *ConsensusTester) testEmptyBlock() {
	// Get the hash of the state before the block was added.
	beforeStateHash := ct.StateHash()

	// Mine and submit a block
	block := ct.MineAndApplyValidBlock()

	// Get the hash of the state after the block was added.
	afterStateHash := ct.StateHash()
	if afterStateHash == beforeStateHash {
		ct.Error("state hash is unchanged after mining a block")
	}

	// Check that the newly mined block is recognized as the current block.
	if ct.CurrentBlock().ID() != block.ID() {
		ct.Error("the state's current block is not reporting as the recently mined block.")
	}

	// These functions break the convention of only using exported functions.
	// But they provide useful checks by making sure that the internals of the
	// state have established in the necessary ways.
	if ct.currentPath[ct.Height()] != block.ID() {
		ct.Error("the state's current path didn't update correctly after accepting a new block")
	}
	bn, exists := ct.blockMap[block.ID()]
	if !exists {
		ct.Error("the state's block map did not update correctly after getting an empty block")
	}
	if !bn.diffsGenerated {
		ct.Error("diffs were not generated on the new block")
	}

	// These functions manipulate the state using unexported functions, which
	// breaks proposed conventions. However, they provide useful information
	// about the accuracy of invertRecentBlock and applyBlockNode.
	cbn := ct.currentBlockNode()
	ct.commitDiffSet(cbn, DiffRevert)
	if beforeStateHash != ct.StateHash() {
		ct.Error("state is different after applying and removing diffs")
	}
	ct.commitDiffSet(cbn, DiffApply)
	if afterStateHash != ct.StateHash() {
		ct.Error("state is different after generateApply, remove, and applying diffs")
	}
}

// testLargeBlock creates a block that is too large to be accepted by the state
// and checks that it actually gets rejected.
func (ct *ConsensusTester) testLargeBlock() {
	// Create a transaction that puts the block over the size limit.
	txns := make([]Transaction, 1)
	bigData := string(make([]byte, BlockSizeLimit))
	txns[0] = Transaction{
		ArbitraryData: []string{bigData},
	}

	// Mine and submit a block, checking for the too large error.
	block, err := ct.MineCurrentBlock(txns)
	if err != nil {
		ct.Fatal(err)
	}
	err = ct.AcceptBlock(block)
	if err != ErrLargeBlock {
		ct.Error(err)
	}
}

// testSinglePayout creates a block with a single miner payout. An incorrect
// and a correct payout get submitted.
func (ct *ConsensusTester) testSingleNoFeePayout() {
	// Mine a block that has no fees, and an incorrect payout. Compare the
	// before and after state hashes to see that they match.
	beforeHash := ct.StateHash()
	payouts := []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(ct.Height()), UnlockHash: ZeroUnlockHash}}
	block, err := MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp(), payouts, nil, ct.CurrentTarget())
	if err != nil {
		ct.Fatal(err)
	}
	err = ct.AcceptBlock(block)
	if err != ErrMinerPayout {
		ct.Error("Expecting miner payout error:", err)
	}
	afterHash := ct.StateHash()
	if beforeHash != afterHash {
		ct.Error("state changed after invalid payouts")
	}

	// Mine a block that has no fees, and a correct payout, then check that the
	// payout made it into the delayedOutputs list.
	payouts = []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(ct.Height() + 1), UnlockHash: ZeroUnlockHash}}
	block, err = MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp(), payouts, nil, ct.CurrentTarget())
	if err != nil {
		ct.Fatal(err)
	}
	err = ct.AcceptBlock(block)
	if err != nil {
		ct.Error("Expecting nil error:", err)
	}
	// Checking the state for correctness requires using an internal function.
	payoutID := block.MinerPayoutID(0)
	output, exists := ct.delayedSiacoinOutputs[ct.Height()][payoutID]
	if !exists {
		ct.Error("could not find payout in delayedOutputs")
	}
	if output.Value.Cmp(CalculateCoinbase(ct.Height())) != 0 {
		ct.Error("payout dooes not pay the correct amount")
	}
}

// testMultipleFeesMultiplePayouts creates blocks with multiple fees and
// multiple payouts and checks that the state correctly accepts or rejects
// these blocks depending on the validity of the payouts.
func (ct *ConsensusTester) testMultipleFeesMultiplePayouts() {
	// Mine a block that has multiple fees and an incorrect payout to multiple
	// addresses, compare the before and after consensus hash and see if
	// everything matches.
	siacoinInput, value := ct.FindSpendableSiacoinInput()
	input2, value2 := ct.FindSpendableSiacoinInput()
	txn := ct.AddSiacoinInputToTransaction(Transaction{}, siacoinInput)
	txn2 := ct.AddSiacoinInputToTransaction(Transaction{}, input2)
	txn.MinerFees = append(txn.MinerFees, value)
	txn2.MinerFees = append(txn2.MinerFees, value2)
	payouts := ct.Payouts(ct.Height()+1, []Transaction{txn, txn2})
	b, err := MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp(), payouts, []Transaction{txn}, ct.CurrentTarget())
	if err != nil {
		ct.Error(err)
	}
	err = ct.AcceptBlock(b)
	if err != ErrMinerPayout {
		ct.Error("Expecting miner payout error:", err)
	}

	// Mine a block with mutliple fees and a correct payout to multiple
	// addresses.
	b, err = MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp(), payouts, []Transaction{txn, txn2}, ct.CurrentTarget())
	if err != nil {
		ct.Error(err)
	}
	err = ct.AcceptBlock(b)
	if err != nil {
		ct.Error(err)
	}
}

// testMissedTarget tries to submit a block that does not meet the target for
// the next block and verifies that the block gets rejected.
func (ct *ConsensusTester) testMissedTarget() {
	// Mine a block that doesn't meet the target.
	b, err := ct.MineCurrentBlock(nil)
	if err != nil {
		ct.Fatal(err)
	}
	for b.CheckTarget(ct.CurrentTarget()) && b.Nonce < 1000*1000 {
		b.Nonce++
	}
	if b.CheckTarget(ct.CurrentTarget()) {
		panic("unable to mine a block with a failing target (lol)")
	}

	err = ct.AcceptBlock(b)
	if err != ErrMissedTarget {
		ct.Error("Block with low target is not being rejected")
	}
}

// testRepeatBlock submits a block to the state, and then submits the same
// block to the state, expecting nothing to change in the consensus set.
func (ct *ConsensusTester) testRepeatBlock() {
	// Add a non-repeat block to the state.
	b, err := ct.MineCurrentBlock(nil)
	if err != nil {
		ct.Fatal(err)
	}
	err = ct.AcceptBlock(b)
	if err != nil {
		ct.Fatal(err)
	}

	// Get the consensus set hash, submit the block, then check that the
	// consensus set hash hasn't changed.
	chash := ct.StateHash()
	err = ct.AcceptBlock(b)
	if err != ErrBlockKnown {
		ct.Error("expecting BlockKnownErr, got", err)
	}
	if chash != ct.StateHash() {
		ct.Error("consensus set hash changed after submitting a repeat block.")
	}
}

// testOrphan submits an orphan block to the state and checks that an orphan
// error is returned.
func (ct *ConsensusTester) testOrphan() {
	b, err := ct.MineCurrentBlock(nil)
	if err != nil {
		ct.Fatal(err)
	}
	b.ParentID[0]++
	err = ct.AcceptBlock(b)
	if err != ErrOrphan {
		ct.Error("unexpected error, expecting OrphanErr:", err)
	}
}

// testBadBlock creates a bad block and then submits it to the state twice -
// the first time it should be processed and rejected, the second time it
// should be recognized as a bad block.
func (ct *ConsensusTester) testBadBlock() {
	badBlock := ct.MineInvalidSignatureBlockSet(0)[0]
	err := ct.AcceptBlock(badBlock)
	if err != crypto.ErrInvalidSignature {
		ct.Error("expecting invalid signature:", err)
	}
	err = ct.AcceptBlock(badBlock)
	if err != ErrBadBlock {
		ct.Error("expecting bad block:", err)
	}
}

// TestBlockTimestamps creates a new testing environment and uses it to call
// testBlockTimestamps.
func TestBlockTimestamps(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testBlockTimestamps()
}

// TestEmptyBlock creates a new testing environment and uses it to call
// testEmptyBlock.
func TestEmptyBlock(t *testing.T) {
	ct := NewTestingEnvironment(t)
	ct.testEmptyBlock()
}

// TestLargeBlock creates a new testing environment and uses it to call
// testLargeBlock.
func TestLargeBlock(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testLargeBlock()
}

// TestSingleNoFeePayouts creates a new testing environment and uses it to call
// testSingleNoFeePayouts.
func TestSingleNoFeePayout(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testSingleNoFeePayout()
}

// TestMultipleFeesMultiplePayouts creates a new testing environment and uses
// it to call testMultipleFeesMultiplePayouts.
func TestMultipleFeesMultiplePayouts(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testMultipleFeesMultiplePayouts()
}

// TestMissedTarget creates a new testing environment and uses it to call
// testMissedTarget.
func TestMissedTarget(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testMissedTarget()
}

// TestRepeatBlock creates a new testing environment and uses it to call
// testRepeatBlock.
func TestRepeatBlock(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testRepeatBlock()
}

// TestOrphan creates a new testing environment and uses it to call testOrphan.
func TestOrphan(t *testing.T) {
	ct := NewTestingEnvironment(t)
	ct.testOrphan()
}

// TestBadBlock creates a new testing environment and uses it to call
// testBadBlock.
func TestBadBlock(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	ct := NewTestingEnvironment(t)
	ct.testBadBlock()
}
