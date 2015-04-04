package consensus

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestCeilingTarget submits block repeatedly until the ceiling target is
// reached. This messes with the adjustment maximums to hit the ceiling target
// (it's pretty unrealistic that the ceiling target would be hit otherwise).
func TestCeilingTarget(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	oldMaxAdjustmentUp := types.MaxAdjustmentUp
	oldMaxAdjustmentDown := types.MaxAdjustmentDown
	types.MaxAdjustmentUp = big.NewRat(3, 1)
	types.MaxAdjustmentDown = big.NewRat(1, 3)
	defer func() { types.MaxAdjustmentUp = oldMaxAdjustmentUp }()
	defer func() { types.MaxAdjustmentDown = oldMaxAdjustmentDown }()

	s := createGenesisState(0, types.ZeroUnlockHash, types.ZeroUnlockHash)
	ct := NewConsensusTester(t, s)

	for i := 0; i < 20; i++ {
		block := ct.MineCurrentBlock(nil)
		err := s.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.CurrentTarget() != types.RootDepth {
		t.Error("ceiling target not reached:", s.CurrentTarget())
	}
}
