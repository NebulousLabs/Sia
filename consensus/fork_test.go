package consensus

import (
	"testing"
)

// TestComplexForking creates multiple states and sets up multiple forking
// scenarios between them to check consistency during forking.
func TestComplexForking(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Need to grab a single time to make sure that each state ends up with the
	// same genesis hash.
	time := CurrentTime()
	s1 := createGenesisState(time, ZeroUnlockHash, ZeroUnlockHash)
	s2 := createGenesisState(time, ZeroUnlockHash, ZeroUnlockHash)
	s3 := createGenesisState(time, ZeroUnlockHash, ZeroUnlockHash)
	a1 := NewAssistant(t, s1)
	a2 := NewAssistant(t, s2)
	a3 := NewAssistant(t, s3)

	// Verify that the three states have the same initial hash.
	if s1.StateHash() != s2.StateHash() {
		t.Fatal("starting states have different hashes - can't run test")
	}
	if s1.StateHash() != s3.StateHash() {
		t.Fatal("starting states have different hashes - can't run test")
	}

	// Get state1 and state2 on different forks, s3 will follow s1 at this
	// point.
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
	err = s3.AcceptBlock(b1)
	if err != nil {
		t.Fatal(err)
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
		err = s3.AcceptBlock(b1)
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
	// state1, causing state1 to fork. State3 is left alone.
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

	// State1 hash should match state2 hash.
	if s1.StateHash() != s2.StateHash() {
		t.Fatal("hashes don't match after trying to force a rewinding fork")
	}

	// Put state3 ahead of state 1&2 on state1's original block path. Then feed
	// all of state 3's blocks to state1, which will cause state1 to fork to
	// state3. State1 will be applying diffs for many of the blocks instead of
	// generating them, which means a different codepath will be followed from
	// the previous fork.
	s3InitialHeight := s3.Height()
	for i := 0; i < 6; i++ {
		b3, err := a3.MineCurrentBlock(nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s3.AcceptBlock(b3)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := s3InitialHeight + 1; i <= s3.Height(); i++ {
		b, exists := s3.BlockAtHeight(i)
		if !exists {
			t.Fatal("error when moving blocks from s3 to s1")
		}
		err = s1.AcceptBlock(b)
		if err != nil {
			t.Fatal(err)
		}
	}

	// State1 hash should match state3 hash.
	if s1.StateHash() != s3.StateHash() {
		t.Fatal("hashes don't match after trying to force an applying fork")
	}
}
