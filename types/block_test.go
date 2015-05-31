package types

import (
	"testing"
)

// TestCalculateCoinbase probes the CalculateCoinbase function. The test code
// is probably too similar to the function code to be of value.
func TestCalculateCoinbase(t *testing.T) {
	c := CalculateCoinbase(0)
	if c.Cmp(NewCurrency64(InitialCoinbase).Mul(NewCurrency(CoinbaseAugment))) != 0 {
		t.Error("Unexpected CalculateCoinbase result")
	}

	c = CalculateCoinbase(1)
	if c.Cmp(NewCurrency64(InitialCoinbase-1).Mul(NewCurrency(CoinbaseAugment))) != 0 {
		t.Error("Unexpected CalculateCoinbase result")
	}

	c = CalculateCoinbase(295000)
	if c.Cmp(NewCurrency64(MinimumCoinbase).Mul(NewCurrency(CoinbaseAugment))) != 0 {
		t.Error(c)
		t.Error(NewCurrency64(MinimumCoinbase).Mul(NewCurrency(CoinbaseAugment)))
		t.Error("Unexpected CalculateCoinbase result")
	}

	c = CalculateCoinbase(1000000000)
	if c.Cmp(NewCurrency64(MinimumCoinbase).Mul(NewCurrency(CoinbaseAugment))) != 0 {
		t.Error(c)
		t.Error(NewCurrency64(MinimumCoinbase).Mul(NewCurrency(CoinbaseAugment)))
		t.Error("Unexpected CalculateCoinbase result")
	}
}

// TestBlockID probes the ID function of the block type.
func TestBlockID(t *testing.T) {
	// Create a bunch of different blocks and check that all of them have
	// unique ids.
	var b Block
	var ids []BlockID

	ids = append(ids, b.ID())
	b.ParentID[0] = 1
	ids = append(ids, b.ID())
	b.Nonce = 45
	ids = append(ids, b.ID())
	b.Timestamp = CurrentTimestamp()
	ids = append(ids, b.ID())
	b.MinerPayouts = append(b.MinerPayouts, SiacoinOutput{Value: CalculateCoinbase(0)})
	ids = append(ids, b.ID())
	b.MinerPayouts = append(b.MinerPayouts, SiacoinOutput{Value: CalculateCoinbase(0)})
	ids = append(ids, b.ID())
	b.Transactions = append(b.Transactions, Transaction{MinerFees: []Currency{CalculateCoinbase(1)}})
	ids = append(ids, b.ID())
	b.Transactions = append(b.Transactions, Transaction{MinerFees: []Currency{CalculateCoinbase(1)}})
	ids = append(ids, b.ID())

	knownIDs := make(map[BlockID]struct{})
	for i, id := range ids {
		_, exists := knownIDs[id]
		if exists {
			t.Error("id repeat for index", i)
		}
		knownIDs[id] = struct{}{}
	}
}

// TestBlockCalculateSubsidy probes the CalculateSubsidy function of the block
// type.
func TestBlockCalculateSubsidy(t *testing.T) {
	// All tests are done at height = 0.
	coinbase := CalculateCoinbase(0)

	// Calculate the subsidy on a block with 0 fees at height 0. Result should
	// be 300,000.
	var b Block
	if b.CalculateSubsidy(0).Cmp(coinbase) != 0 {
		t.Error("subsidy is miscalculated for an empty block")
	}

	// Calculate when there is a fee in a transcation.
	expected := coinbase.Add(NewCurrency64(123))
	txn := Transaction{
		MinerFees: []Currency{NewCurrency64(123)},
	}
	b.Transactions = append(b.Transactions, txn)
	if b.CalculateSubsidy(0).Cmp(expected) != 0 {
		t.Error("subsidy is miscalculated for a block with a single transaction")
	}

	// Add a single no-fee transaction and check again.
	txn = Transaction{
		ArbitraryData: []string{"NonSia"},
	}
	b.Transactions = append(b.Transactions, txn)
	if b.CalculateSubsidy(0).Cmp(expected) != 0 {
		t.Error("subsidy is miscalculated with empty transactions.")
	}

	// Add a transaction with multiple fees.
	expected = expected.Add(NewCurrency64(1 + 2 + 3))
	txn = Transaction{
		MinerFees: []Currency{
			NewCurrency64(1),
			NewCurrency64(2),
			NewCurrency64(3),
		},
	}
	b.Transactions = append(b.Transactions, txn)
	if b.CalculateSubsidy(0).Cmp(expected) != 0 {
		t.Error("subsidy is miscalculated for a block with a single transaction")
	}

	// Add an empty transaction to the beginning.
	txn = Transaction{
		ArbitraryData: []string{"NonSia"},
	}
	b.Transactions = append([]Transaction{txn}, b.Transactions...)
	if b.CalculateSubsidy(0).Cmp(expected) != 0 {
		t.Error("subsidy is miscalculated with empty transactions.")
	}
}

// TestBlockCheckMinerPayouts probes the CheckMinerPayouts function of the
// block type.
func TestBlockCheckMinerPayouts(t *testing.T) {
	// All tests are done at height = 0.
	coinbase := CalculateCoinbase(0)

	// Create a block with a single valid payout.
	b := Block{
		MinerPayouts: []SiacoinOutput{
			{Value: coinbase},
		},
	}
	if !b.CheckMinerPayouts(0) {
		t.Error("payouts evaluated incorrectly when there is only one payout.")
	}

	// Try a block with an incorrect payout.
	b = Block{
		MinerPayouts: []SiacoinOutput{
			{Value: coinbase.Sub(NewCurrency64(1))},
		},
	}
	if b.CheckMinerPayouts(0) {
		t.Error("payouts evaluated incorrectly when there is a too-small payout")
	}

	// Try a block with 2 payouts.
	b = Block{
		MinerPayouts: []SiacoinOutput{
			{Value: coinbase.Sub(NewCurrency64(1))},
			{Value: NewCurrency64(1)},
		},
	}
	if !b.CheckMinerPayouts(0) {
		t.Error("payouts evaluated incorrectly when there are 2 payouts")
	}

	// Try a block with 2 payouts that are too large.
	b = Block{
		MinerPayouts: []SiacoinOutput{
			{Value: coinbase},
			{Value: coinbase},
		},
	}
	if b.CheckMinerPayouts(0) {
		t.Error("payouts evaluated incorrectly when there are two large payouts")
	}

	// Create a block with an empty payout.
	b = Block{
		MinerPayouts: []SiacoinOutput{
			{Value: coinbase},
			{},
		},
	}
	if b.CheckMinerPayouts(0) {
		t.Error("payouts evaluated incorrectly when there is only one payout.")
	}
}

// TestBlockCheckTarget probes the CheckTarget function of the block type.
func TestBlockCheckTarget(t *testing.T) {
	var b Block
	lowTarget := RootDepth
	highTarget := Target{}
	sameTarget := Target(b.ID())

	if !b.CheckTarget(lowTarget) {
		t.Error("CheckTarget failed for a low target")
	}
	if b.CheckTarget(highTarget) {
		t.Error("CheckTarget passed for a high target")
	}
	if !b.CheckTarget(sameTarget) {
		t.Error("CheckTarget failed for a same target")
	}
}

// TestBlockMinerPayoutID probes the MinerPayout function of the block type.
func TestBlockMinerPayoutID(t *testing.T) {
	// Create a block with 2 miner payouts, and check that each payout has a
	// different id, and that the id is dependent on the block id.
	var ids []SiacoinOutputID
	b := Block{
		MinerPayouts: []SiacoinOutput{
			{Value: CalculateCoinbase(0)},
			{Value: CalculateCoinbase(0)},
		},
	}
	ids = append(ids, b.MinerPayoutID(1), b.MinerPayoutID(2))
	b.ParentID[0] = 1
	ids = append(ids, b.MinerPayoutID(1), b.MinerPayoutID(2))

	knownIDs := make(map[SiacoinOutputID]struct{})
	for i, id := range ids {
		_, exists := knownIDs[id]
		if exists {
			t.Error("id repeat for index", i)
		}
		knownIDs[id] = struct{}{}
	}
}
