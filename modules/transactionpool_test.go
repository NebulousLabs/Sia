package modules

import (
	"testing"
)

// TestConsensusConflict checks that the consensus conflict type is correctly
// assembling consensus conflict errors.
func TestConsensusConflict(t *testing.T) {
	ncc := NewConsensusConflict("problem")
	if ncc.Error() != "consensus conflict: problem" {
		t.Error("wrong error message being reported in a consensus conflict")
	}

	err := func() error {
		return ncc
	}()
	if err.Error() != "consensus conflict: problem" {
		t.Error("wrong error message being reported in a consensus conflict")
	}
	if _, ok := err.(ConsensusConflict); !ok {
		t.Error("error is not maintaining consensus conflict type")
	}
}
