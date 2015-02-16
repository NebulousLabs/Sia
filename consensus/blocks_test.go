package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
)

// testBlockTimestamps submits a block to the state with a timestamp that is
// too early and a timestamp that is too late, and verifies that each get
// rejected.
func (a *Assistant) testBlockTimestamps() {
	// Create a block with a timestamp that is too early.
	block, err := MineTestingBlock(a.State.CurrentBlock().ID(), a.State.EarliestTimestamp()-1, a.Payouts(a.State.Height()+1, nil), nil, a.State.CurrentTarget())
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != EarlyTimestampErr {
		a.Tester.Error("unexpected error when submitting a too early timestamp:", err)
	}

	// Create a block with a timestamp that is too late.
	block, err = MineTestingBlock(a.State.CurrentBlock().ID(), CurrentTimestamp()+10+FutureThreshold, a.Payouts(a.State.Height()+1, nil), nil, a.State.CurrentTarget())
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != FutureTimestampErr {
		a.Tester.Error("unexpected error when submitting a too-early timestamp:", err)
	}
}

// testEmptyBlock adds an empty block to the state and checks for errors.
func (a *Assistant) testEmptyBlock() {
	// Get the hash of the state before the block was added.
	beforeStateHash := a.State.StateHash()

	// Mine and submit a block
	block := a.MineAndApplyValidBlock()

	// Get the hash of the state after the block was added.
	afterStateHash := a.State.StateHash()
	if afterStateHash == beforeStateHash {
		a.Tester.Error("state hash is unchanged after mining a block")
	}

	// Check that the newly mined block is recognized as the current block.
	if a.State.CurrentBlock().ID() != block.ID() {
		a.Tester.Error("the state's current block is not reporting as the recently mined block.")
	}

	// These functions break the convention of only using exported functions.
	// But they provide useful checks by making sure that the internals of the
	// state have established in the necessary ways.
	if a.State.currentPath[a.State.Height()] != block.ID() {
		a.Tester.Error("the state's current path didn't update correctly after accepting a new block")
	}
	bn, exists := a.State.blockMap[block.ID()]
	if !exists {
		a.Tester.Error("the state's block map did not update correctly after getting an empty block")
	}
	if !bn.diffsGenerated {
		a.Tester.Error("diffs were not generated on the new block")
	}

	// These functions manipulate the state using unexported functions, which
	// breaks proposed conventions. However, they provide useful information
	// about the accuracy of invertRecentBlock and applyBlockNode.
	cbn := a.State.currentBlockNode()
	a.State.applyDiffSet(cbn, DiffRevert)
	if beforeStateHash != a.State.StateHash() {
		a.Tester.Error("state is different after applying and removing diffs")
	}
	a.State.applyDiffSet(cbn, DiffApply)
	if afterStateHash != a.State.StateHash() {
		a.Tester.Error("state is different after generateApply, remove, and applying diffs")
	}
}

// testLargeBlock creates a block that is too large to be accepted by the state
// and checks that it actually gets rejected.
func (a *Assistant) testLargeBlock() {
	// Create a transaction that puts the block over the size limit.
	txns := make([]Transaction, 1)
	bigData := string(make([]byte, BlockSizeLimit))
	txns[0] = Transaction{
		ArbitraryData: []string{bigData},
	}

	// Mine and submit a block, checking for the too large error.
	block, err := a.MineCurrentBlock(txns)
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != LargeBlockErr {
		a.Tester.Error(err)
	}
}

// testSinglePayout creates a block with a single miner payout. An incorrect
// and a correct payout get submitted.
func (a *Assistant) testSingleNoFeePayout() {
	// Mine a block that has no fees, and an incorrect payout. Compare the
	// before and after state hashes to see that they match.
	beforeHash := a.State.StateHash()
	payouts := []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(a.State.Height()), UnlockHash: ZeroUnlockHash}}
	block, err := MineTestingBlock(a.State.CurrentBlock().ID(), CurrentTimestamp(), payouts, nil, a.State.CurrentTarget())
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != MinerPayoutErr {
		a.Tester.Error("Expecting miner payout error:", err)
	}
	afterHash := a.State.StateHash()
	if beforeHash != afterHash {
		a.Tester.Error("state changed after invalid payouts")
	}

	// Mine a block that has no fees, and a correct payout, then check that the
	// payout made it into the delayedOutputs list.
	payouts = []SiacoinOutput{SiacoinOutput{Value: CalculateCoinbase(a.State.Height() + 1), UnlockHash: ZeroUnlockHash}}
	block, err = MineTestingBlock(a.State.CurrentBlock().ID(), CurrentTimestamp(), payouts, nil, a.State.CurrentTarget())
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(block)
	if err != nil {
		a.Tester.Error("Expecting nil error:", err)
	}
	// Checking the state for correctness requires using an internal function.
	payoutID := block.MinerPayoutID(0)
	output, exists := a.State.delayedSiacoinOutputs[a.State.Height()][payoutID]
	if !exists {
		a.Tester.Error("could not find payout in delayedOutputs")
	}
	if output.Value.Cmp(CalculateCoinbase(a.State.Height())) != 0 {
		a.Tester.Error("payout dooes not pay the correct amount")
	}
}

// testMultipleFeesMultiplePayouts creates blocks with multiple fees and
// multiple payouts and checks that the state correctly accepts or rejects
// these blocks depending on the validity of the payouts.
func (a *Assistant) testMultipleFeesMultiplePayouts() {
	// Mine a block that has multiple fees and an incorrect payout to multiple
	// addresses, compare the before and after consensus hash and see if
	// everything matches.
	siacoinInput, value := a.FindSpendableSiacoinInput()
	input2, value2 := a.FindSpendableSiacoinInput()
	txn := a.AddSiacoinInputToTransaction(Transaction{}, siacoinInput)
	txn2 := a.AddSiacoinInputToTransaction(Transaction{}, input2)
	txn.MinerFees = append(txn.MinerFees, value)
	txn2.MinerFees = append(txn2.MinerFees, value2)
	payouts := a.Payouts(a.State.Height()+1, []Transaction{txn, txn2})
	b, err := MineTestingBlock(a.State.CurrentBlock().ID(), CurrentTimestamp(), payouts, []Transaction{txn}, a.State.CurrentTarget())
	if err != nil {
		a.Tester.Error(err)
	}
	err = a.State.AcceptBlock(b)
	if err != MinerPayoutErr {
		a.Tester.Error("Expecting miner payout error:", err)
	}

	// Mine a block with mutliple fees and a correct payout to multiple
	// addresses.
	b, err = MineTestingBlock(a.State.CurrentBlock().ID(), CurrentTimestamp(), payouts, []Transaction{txn, txn2}, a.State.CurrentTarget())
	if err != nil {
		a.Tester.Error(err)
	}
	err = a.State.AcceptBlock(b)
	if err != nil {
		a.Tester.Error(err)
	}
}

// testMissedTarget tries to submit a block that does not meet the target for
// the next block and verifies that the block gets rejected.
func (a *Assistant) testMissedTarget() {
	// Mine a block that doesn't meet the target.
	b, err := a.MineCurrentBlock(nil)
	if err != nil {
		a.Tester.Fatal(err)
	}
	for b.CheckTarget(a.State.CurrentTarget()) && b.Nonce < 1000*1000 {
		b.Nonce++
	}
	if b.CheckTarget(a.State.CurrentTarget()) {
		panic("unable to mine a block with a failing target (lol)")
	}

	err = a.State.AcceptBlock(b)
	if err != MissedTargetErr {
		a.Tester.Error("Block with low target is not being rejected")
	}
}

// testRepeatBlock submits a block to the state, and then submits the same
// block to the state, expecting nothing to change in the consensus set.
func (a *Assistant) testRepeatBlock() {
	// Add a non-repeat block to the state.
	b, err := a.MineCurrentBlock(nil)
	if err != nil {
		a.Tester.Fatal(err)
	}
	err = a.State.AcceptBlock(b)
	if err != nil {
		a.Tester.Fatal(err)
	}

	// Get the consensus set hash, submit the block, then check that the
	// consensus set hash hasn't changed.
	chash := a.State.StateHash()
	err = a.State.AcceptBlock(b)
	if err != BlockKnownErr {
		a.Tester.Error("expecting BlockKnownErr, got", err)
	}
	if chash != a.State.StateHash() {
		a.Tester.Error("consensus set hash changed after submitting a repeat block.")
	}
}

// testOrphan submits an orphan block to the state and checks that an orphan
// error is returned.
func (a *Assistant) testOrphan() {
	b, err := a.MineCurrentBlock(nil)
	if err != nil {
		a.Tester.Fatal(err)
	}
	b.ParentID[0]++
	err = a.State.AcceptBlock(b)
	if err != OrphanErr {
		a.Tester.Error("unexpected error, expecting OrphanErr:", err)
	}
}

// testBadBlock creates a bad block and then submits it to the state twice -
// the first time it should be processed and rejected, the second time it
// should be recognized as a bad block.
func (a *Assistant) testBadBlock() {
	badBlock := a.MineInvalidSignatureBlockSet(0)
	err := a.State.AcceptBlock(badBlock[0])
	if err != crypto.ErrInvalidSignature {
		a.Tester.Error("expecting invalid signature:", err)
	}
	err = a.State.AcceptBlock(badBlock[0])
	if err != BadBlockErr {
		a.Tester.Error("expecting bad block:", err)
	}
}

// TestBlockTimestamps creates a new testing environment and uses it to call
// TestBlockTimestamps.
func TestBlockTimestamps(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testBlockTimestamps()
}

// TestEmptyBlock creates a new testing environment and uses it to call
// testEmptyBlock.
func TestEmptyBlock(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testEmptyBlock()
}

// TestLargeBlock creates a new testing environment and uses it to call
// testLargeBlock.
func TestLargeBlock(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testLargeBlock()
}

// TestSingleNoFeePayouts creates a new testing environment and uses it to call
// testSingleNoFeePayouts.
func TestSingleNoFeePayout(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testSingleNoFeePayout()
}

// TestMultipleFeesMultiplePayouts creates a new testing environment and uses
// it to call testMultipleFeesMultiplePayouts.
func TestMultipleFeesMultiplePayouts(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testMultipleFeesMultiplePayouts()
}

// TestMissedTarget creates a new testing environment and uses it to call
// testMissedTarget.
func TestMissedTarget(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testMissedTarget()
}

// TestRepeatBlock creates a new testing environment and uses it to call
// testRepeatBlock.
func TestRepeatBlock(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testRepeatBlock()
}

// TestOrphan creates a new testing environment and uses it to call testOrphan.
func TestOrphan(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testOrphan()
}

// TestBadBlock creates a new testing environment and uses it to call
// testBadBlock.
func TestBadBlock(t *testing.T) {
	a := NewTestingEnvironment(t)
	a.testBadBlock()
}
