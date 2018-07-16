package consensus

import (
	"testing"

	"gitlab.com/NebulousLabs/Sia/types"
)

// mockMarshaler is a mock implementation of the encoding.GenericMarshaler
// interface that allows the client to pre-define the length of the marshaled
// data.
type mockMarshaler struct {
	marshalLength uint64
}

// Marshal marshals an object into an empty byte slice of marshalLength.
func (m mockMarshaler) Marshal(interface{}) []byte {
	return make([]byte, m.marshalLength)
}

// Unmarshal is not implemented.
func (m mockMarshaler) Unmarshal([]byte, interface{}) error {
	panic("not implemented")
}

// mockClock is a mock implementation of the types.Clock interface that allows
// the client to pre-define a return value for Now().
type mockClock struct {
	now types.Timestamp
}

// Now returns mockClock's pre-defined Timestamp.
func (c mockClock) Now() types.Timestamp {
	return c.now
}

var validateBlockTests = []struct {
	now            types.Timestamp
	minTimestamp   types.Timestamp
	blockTimestamp types.Timestamp
	blockSize      uint64
	errWant        error
	msg            string
}{
	{
		minTimestamp:   types.Timestamp(5),
		blockTimestamp: types.Timestamp(4),
		errWant:        errEarlyTimestamp,
		msg:            "ValidateBlock should reject blocks with timestamps that are too early",
	},
	{
		blockSize: types.BlockSizeLimit + 1,
		errWant:   errLargeBlock,
		msg:       "ValidateBlock should reject excessively large blocks",
	},
	{
		now:            types.Timestamp(50),
		blockTimestamp: types.Timestamp(50) + types.ExtremeFutureThreshold + 1,
		errWant:        errExtremeFutureTimestamp,
		msg:            "ValidateBlock should reject blocks timestamped in the extreme future",
	},
}

// TestUnitValidateBlock runs a series of unit tests for ValidateBlock.
func TestUnitValidateBlock(t *testing.T) {
	// TODO(mtlynch): Populate all parameters to ValidateBlock so that everything
	// is valid except for the attribute that causes validation to fail. (i.e.
	// don't assume an ordering to the implementation of the validation function).
	for _, tt := range validateBlockTests {
		b := types.Block{
			Timestamp: tt.blockTimestamp,
		}
		blockValidator := stdBlockValidator{
			marshaler: mockMarshaler{
				marshalLength: tt.blockSize,
			},
			clock: mockClock{
				now: tt.now,
			},
		}
		err := blockValidator.ValidateBlock(b, b.ID(), tt.minTimestamp, types.RootDepth, 0, nil)
		if err != tt.errWant {
			t.Errorf("%s: got %v, want %v", tt.msg, err, tt.errWant)
		}
	}
}

// TestCheckMinerPayouts probes the checkMinerPayouts function.
func TestCheckMinerPayouts(t *testing.T) {
	// All tests are done at height = 0.
	coinbase := types.CalculateCoinbase(0)

	// Create a block with a single valid payout.
	b := types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase},
		},
	}
	if !checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there is only one payout.")
	}

	// Try a block with an incorrect payout.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase.Sub(types.NewCurrency64(1))},
		},
	}
	if checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there is a too-small payout")
	}

	// Try a block with 2 payouts.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase.Sub(types.NewCurrency64(1))},
			{Value: types.NewCurrency64(1)},
		},
	}
	if !checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there are 2 payouts")
	}

	// Try a block with 2 payouts that are too large.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase},
			{Value: coinbase},
		},
	}
	if checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there are two large payouts")
	}

	// Create a block with an empty payout.
	b = types.Block{
		MinerPayouts: []types.SiacoinOutput{
			{Value: coinbase},
			{},
		},
	}
	if checkMinerPayouts(b, 0) {
		t.Error("payouts evaluated incorrectly when there is only one payout.")
	}
}

// TestCheckTarget probes the checkTarget function.
func TestCheckTarget(t *testing.T) {
	var b types.Block
	lowTarget := types.RootDepth
	highTarget := types.Target{}
	sameTarget := types.Target(b.ID())

	if !checkTarget(b, b.ID(), lowTarget) {
		t.Error("CheckTarget failed for a low target")
	}
	if checkTarget(b, b.ID(), highTarget) {
		t.Error("CheckTarget passed for a high target")
	}
	if !checkTarget(b, b.ID(), sameTarget) {
		t.Error("CheckTarget failed for a same target")
	}
}
