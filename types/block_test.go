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
			SiacoinOutput{
				Value: CalculateCoinbase(0),
			},
			SiacoinOutput{
				Value: CalculateCoinbase(0),
			},
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
