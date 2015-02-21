package consensus

import (
	"math/big"
	"testing"
)

// TestCeilingTarget submits block repeatedly until the ceiling target is
// reached. This messes with the adjustment maximums to hit the ceiling target
// (it's pretty unrealistic that the ceiling target would be hit otherwise).
func TestCeilingTarget(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	oldMaxAdjustmentUp := MaxAdjustmentUp
	oldMaxAdjustmentDown := MaxAdjustmentDown
	MaxAdjustmentUp = big.NewRat(3, 1)
	MaxAdjustmentDown = big.NewRat(1, 3)
	defer func() { MaxAdjustmentUp = oldMaxAdjustmentUp }()
	defer func() { MaxAdjustmentDown = oldMaxAdjustmentDown }()

	s := createGenesisState(0, ZeroUnlockHash, ZeroUnlockHash)
	ct := NewConsensusTester(t, s)

	for i := 0; i < 20; i++ {
		block := ct.MineCurrentBlock(nil)
		err := s.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.CurrentTarget() != RootDepth {
		t.Error("ceiling target not reached:", s.CurrentTarget())
	}
}
