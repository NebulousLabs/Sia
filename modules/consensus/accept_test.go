package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// testBlockTimestamps submits a block to the state with a timestamp that is
// too early and a timestamp that is too late, and verifies that each get
// rejected.
func (ct *ConsensusTester) testBlockTimestamps() {
	// Create a block with a timestamp that is too early.
	block := MineTestingBlock(ct.CurrentBlock().ID(), ct.EarliestTimestamp()-1, ct.Payouts(ct.Height()+1, nil), nil, ct.CurrentTarget())
	err := ct.AcceptBlock(block)
	if err != ErrEarlyTimestamp {
		ct.Error("unexpected error when submitting a too early timestamp:", err)
	}

	// Create a block with a timestamp that is too late.
	block = MineTestingBlock(ct.CurrentBlock().ID(), types.CurrentTimestamp()+10+types.FutureThreshold, ct.Payouts(ct.Height()+1, nil), nil, ct.CurrentTarget())
	err = ct.AcceptBlock(block)
	if err != ErrFutureTimestamp {
		ct.Error("unexpected error when submitting a too-early timestamp:", err)
	}
}

// testSinglePayout creates a block with a single miner payout. An incorrect
// and a correct payout get submitted.
func (ct *ConsensusTester) testSingleNoFeePayout() {
	// Mine a block that has no fees, and an incorrect payout. Compare the
	// before and after state hashes to see that they match.
	beforeHash := ct.StateHash()
	payouts := []types.SiacoinOutput{types.SiacoinOutput{Value: types.CalculateCoinbase(ct.Height()), UnlockHash: types.ZeroUnlockHash}}
	block := MineTestingBlock(ct.CurrentBlock().ID(), types.CurrentTimestamp(), payouts, nil, ct.CurrentTarget())
	err := ct.AcceptBlock(block)
	if err != ErrMinerPayout {
		ct.Error("Expecting miner payout error:", err)
	}
	afterHash := ct.StateHash()
	if beforeHash != afterHash {
		ct.Error("state changed after invalid payouts")
	}

	// Mine a block that has no fees, and a correct payout, then check that the
	// payout made it into the delayedOutputs list.
	payouts = []types.SiacoinOutput{types.SiacoinOutput{Value: types.CalculateCoinbase(ct.Height() + 1), UnlockHash: types.ZeroUnlockHash}}
	block = MineTestingBlock(ct.CurrentBlock().ID(), types.CurrentTimestamp(), payouts, nil, ct.CurrentTarget())
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
	if output.Value.Cmp(types.CalculateCoinbase(ct.Height())) != 0 {
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
	txn := ct.AddSiacoinInputToTransaction(types.Transaction{}, siacoinInput)
	txn2 := ct.AddSiacoinInputToTransaction(types.Transaction{}, input2)
	txn.MinerFees = append(txn.MinerFees, value)
	txn2.MinerFees = append(txn2.MinerFees, value2)
	payouts := ct.Payouts(ct.Height()+1, []types.Transaction{txn, txn2})
	block := MineTestingBlock(ct.CurrentBlock().ID(), types.CurrentTimestamp(), payouts, []types.Transaction{txn}, ct.CurrentTarget())
	err := ct.AcceptBlock(block)
	if err != ErrMinerPayout {
		ct.Error("Expecting miner payout error:", err)
	}

	// Mine a block with mutliple fees and a correct payout to multiple
	// addresses.
	block = MineTestingBlock(ct.CurrentBlock().ID(), types.CurrentTimestamp(), payouts, []types.Transaction{txn, txn2}, ct.CurrentTarget())
	err = ct.AcceptBlock(block)
	if err != nil {
		ct.Error(err)
	}
}

// TestBlockTimestamps creates a new testing environment and uses it to call
// testBlockTimestamps.
func TestBlockTimestamps(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	ct := NewTestingEnvironment("TestBlockTimestamps", t)
	ct.testBlockTimestamps()
}

// TestSingleNoFeePayouts creates a new testing environment and uses it to call
// testSingleNoFeePayouts.
func TestSingleNoFeePayout(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	ct := NewTestingEnvironment("TestSingleNoFeePayout", t)
	ct.testSingleNoFeePayout()
}

// TestMultipleFeesMultiplePayouts creates a new testing environment and uses
// it to call testMultipleFeesMultiplePayouts.
func TestMultipleFeesMultiplePayouts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	ct := NewTestingEnvironment("TestMultipleFeesMultiplePayouts", t)
	ct.testMultipleFeesMultiplePayouts()
}
