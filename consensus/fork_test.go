package consensus

import (
	"testing"
)

// TestRewindingFork creates a situation where multiple blocks need to be
// removed during a fork.
func TestRewindingFork(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Need to grab a single time to make sure that each state ends up with the
	// same genesis hash.
	time := CurrentTime()
	s1 := createGenesisState(time, ZeroUnlockHash, ZeroUnlockHash)
	s2 := createGenesisState(time, ZeroUnlockHash, ZeroUnlockHash)
	a1 := NewAssistant(t, s1)
	a2 := NewAssistant(t, s2)

	// Verify that the two states have the same initial hash.
	if s1.StateHash() != s2.StateHash() {
		t.Fatal("starting states have different hashes - can't run test")
	}

	// Get each state on a different fork.
	b1, err := MineTestingBlock(s1.CurrentBlock().ID(), time, a1.Payouts(s1.Height()+1, nil), nil, s1.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s1.AcceptBlock(b1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := MineTestingBlock(s2.CurrentBlock().ID(), time+1, a2.Payouts(s2.Height()+1, nil), nil, s2.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s2.AcceptBlock(b2)
	if err != nil {
		t.Fatal(err)
	}
	if s1.StateHash() == s2.StateHash() {
		t.Fatal("failed to get states on different forks")
	}

	// Mine several blocks on each state.
	for i := 0; i < 3; i++ {
		// state 1 mining.
		b1, err = a1.MineCurrentBlock(nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s1.AcceptBlock(b1)
		if err != nil {
			t.Fatal(err)
		}

		// state 2 mining.
		b2, err = a2.MineCurrentBlock(nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s2.AcceptBlock(b2)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Put state2 ahead of state1 and then give all of the state2 blocks to
	// state1, causing state1 to fork.
	for i := 0; i < 3; i++ {
		b2, err = a2.MineCurrentBlock(nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s2.AcceptBlock(b2)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := BlockHeight(1); i <= s2.Height(); i++ {
		b, exists := s2.BlockAtHeight(i)
		if !exists {
			t.Fatal("error when moving blocks from s2 to s1")
		}
		err = s1.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}

	// State1 hash should not match state2 hash.
	if s1.StateHash() != s2.StateHash() {
		t.Fatal("hashes don't match after trying to force a rewinding fork")
	}
}
