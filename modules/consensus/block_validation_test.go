package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

type mockMarshaler struct {
	marshalLength uint64
}

func (m mockMarshaler) Marshal(interface{}) []byte {
	return make([]byte, m.marshalLength)
}

func (m mockMarshaler) Unmarshal([]byte, interface{}) error {
	return nil
}

type mockClock struct {
	now types.Timestamp
}

func (c mockClock) Now() types.Timestamp {
	return c.now
}

// TestUnitValidateBlockEarlyTimestamp checks that stdBlockValidator
// rejects blocks with timestamps that are too early.
func TestUnitValidateBlockEarlyTimestamp(t *testing.T) {
	// TODO(mtlynch): Populate all parameters to ValidateBlock so that everything
	// is valid except for the timestamp (i.e. don't assume an ordering to the
	// implementation of the validation function).
	minTimestamp := types.Timestamp(5)
	blockValidator := stdBlockValidator{}
	b := types.Block{
		Timestamp: minTimestamp - 1,
	}
	err := blockValidator.ValidateBlock(b, minTimestamp, types.Target{}, 0)
	wantErr := errEarlyTimestamp
	if err != wantErr {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

// TestUnitValidateBlockLargeBlock checks that stdBlockValidator rejects
// excessively large blocks.
func TestUnitValidateBlockLargeBlock(t *testing.T) {
	// TODO(mtlynch): Populate all parameters to ValidateBlock so that everything
	// is valid except for the length (i.e. don't assume an ordering to the
	// implementation of the validation function).
	minTimestamp := types.Timestamp(5)
	blockValidator := stdBlockValidator{
		marshaler: mockMarshaler{
			marshalLength: types.BlockSizeLimit + 1,
		},
	}
	b := types.Block{
		Timestamp: minTimestamp,
	}
	err := blockValidator.ValidateBlock(b, minTimestamp, types.RootDepth, 0)
	wantErr := errLargeBlock
	if err != wantErr {
		t.Errorf("got %v, want %v", err, wantErr)
	}
}

// TestUnitValidateBlockExtremeFutureTimestamp checks that stdBlockValidator
// rejects blocks that are timestamped in the extreme future.
func TestUnitValidateBlockExtremeFutureTimestamp(t *testing.T) {
	// TODO(mtlynch): Populate all parameters to ValidateBlock so that everything
	// is valid except for the timestamp (i.e. don't assume an ordering to the
	// implementation of the validation function).
	minTimestamp := types.Timestamp(5)
	now := types.Timestamp(50)
	blockValidator := stdBlockValidator{
		marshaler: mockMarshaler{
			marshalLength: 1,
		},
		clock: mockClock{
			now: now,
		},
	}
	b := types.Block{
		Timestamp: now + types.ExtremeFutureThreshold + 1,
	}
	err := blockValidator.ValidateBlock(b, minTimestamp, types.RootDepth, 0)
	wantErr := errExtremeFutureTimestamp
	if err != wantErr {
		t.Errorf("got %v, want %v", err, wantErr)
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

	if !checkTarget(b, lowTarget) {
		t.Error("CheckTarget failed for a low target")
	}
	if checkTarget(b, highTarget) {
		t.Error("CheckTarget passed for a high target")
	}
	if !checkTarget(b, sameTarget) {
		t.Error("CheckTarget failed for a same target")
	}
}
