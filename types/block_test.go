package types

import (
	"testing"

	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/encoding"
)

// TestCalculateCoinbase probes the CalculateCoinbase function. The test code
// is probably too similar to the function code to be of value.
func TestCalculateCoinbase(t *testing.T) {
	c := CalculateCoinbase(0)
	if c.Cmp(NewCurrency64(InitialCoinbase).Mul(SiacoinPrecision)) != 0 {
		t.Error("Unexpected CalculateCoinbase result")
	}

	c = CalculateCoinbase(1)
	if c.Cmp(NewCurrency64(InitialCoinbase-1).Mul(SiacoinPrecision)) != 0 {
		t.Error("Unexpected CalculateCoinbase result")
	}

	c = CalculateCoinbase(295000)
	if c.Cmp(NewCurrency64(MinimumCoinbase).Mul(SiacoinPrecision)) != 0 {
		t.Error(c)
		t.Error(NewCurrency64(MinimumCoinbase).Mul(SiacoinPrecision))
		t.Error("Unexpected CalculateCoinbase result")
	}

	c = CalculateCoinbase(1000000000)
	if c.Cmp(NewCurrency64(MinimumCoinbase).Mul(SiacoinPrecision)) != 0 {
		t.Error(c)
		t.Error(NewCurrency64(MinimumCoinbase).Mul(SiacoinPrecision))
		t.Error("Unexpected CalculateCoinbase result")
	}
}

// TestCalculateNumSiacoins checks that the siacoin calculator is correctly
// determining the number of siacoins in circulation. The check is performed by
// doing a naive computation, instead of by doing the optimized computation.
func TestCalculateNumSiacoins(t *testing.T) {
	c := CalculateNumSiacoins(0)
	if c.Cmp(CalculateCoinbase(0)) != 0 {
		t.Error("unexpected circulation result for value 0, got", c)
	}

	if testing.Short() {
		t.SkipNow()
	}
	totalCoins := NewCurrency64(0)
	for i := BlockHeight(0); i < 500e3; i++ {
		totalCoins = totalCoins.Add(CalculateCoinbase(i))
		if totalCoins.Cmp(CalculateNumSiacoins(i)) != 0 {
			t.Fatal("coin miscalculation", i, totalCoins, CalculateNumSiacoins(i))
		}
	}
}

// TestBlockHeader checks that BlockHeader returns the correct value, and that
// the hash is consistent with the old method for obtaining the hash.
func TestBlockHeader(t *testing.T) {
	var b Block
	b.ParentID[1] = 1
	b.Nonce[2] = 2
	b.Timestamp = 3
	b.MinerPayouts = []SiacoinOutput{{Value: NewCurrency64(4)}}
	b.Transactions = []Transaction{{ArbitraryData: [][]byte{{'5'}}}}

	id1 := b.ID()
	id2 := BlockID(crypto.HashBytes(encoding.Marshal(b.Header())))
	id3 := BlockID(crypto.HashAll(
		b.ParentID,
		b.Nonce,
		b.Timestamp,
		b.MerkleRoot(),
	))

	if id1 != id2 || id2 != id3 || id3 != id1 {
		t.Error("Methods for getting block id don't return the same results")
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
	b.Nonce[0] = 45
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

// TestHeaderID probes the ID function of the BlockHeader type.
func TestHeaderID(t *testing.T) {
	// Create a bunch of different blocks and check that all of them have
	// unique ids.
	var blocks []Block
	var b Block

	blocks = append(blocks, b)
	b.ParentID[0] = 1
	blocks = append(blocks, b)
	b.Nonce[0] = 45
	blocks = append(blocks, b)
	b.Timestamp = CurrentTimestamp()
	blocks = append(blocks, b)
	b.MinerPayouts = append(b.MinerPayouts, SiacoinOutput{Value: CalculateCoinbase(0)})
	blocks = append(blocks, b)
	b.MinerPayouts = append(b.MinerPayouts, SiacoinOutput{Value: CalculateCoinbase(0)})
	blocks = append(blocks, b)
	b.Transactions = append(b.Transactions, Transaction{MinerFees: []Currency{CalculateCoinbase(1)}})
	blocks = append(blocks, b)
	b.Transactions = append(b.Transactions, Transaction{MinerFees: []Currency{CalculateCoinbase(1)}})
	blocks = append(blocks, b)

	knownIDs := make(map[BlockID]struct{})
	for i, block := range blocks {
		blockID := block.ID()
		headerID := block.Header().ID()
		if blockID != headerID {
			t.Error("headerID does not match blockID for index", i)
		}
		_, exists := knownIDs[headerID]
		if exists {
			t.Error("id repeat for index", i)
		}
		knownIDs[headerID] = struct{}{}
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
		ArbitraryData: [][]byte{{'6'}},
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
		ArbitraryData: [][]byte{{'7'}},
	}
	b.Transactions = append([]Transaction{txn}, b.Transactions...)
	if b.CalculateSubsidy(0).Cmp(expected) != 0 {
		t.Error("subsidy is miscalculated with empty transactions.")
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
