package consensus

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// mineInvalidSignatureBlock will mine a block that is valid on the longest
// fork except for having an illegal signature, and then will mine `i` more
// blocks after that which are valid.
func (ct *ConsensusTester) MineInvalidSignatureBlockSet(depth int) (blocks []types.Block) {
	siacoinInput, value := ct.FindSpendableSiacoinInput()
	txn := ct.AddSiacoinInputToTransaction(types.Transaction{}, siacoinInput)
	txn.MinerFees = append(txn.MinerFees, value)

	// Invalidate the signature.
	byteSig := []byte(txn.Signatures[0].Signature)
	byteSig[0]++
	txn.Signatures[0].Signature = types.Signature(byteSig)

	// Mine a block with this transcation.
	block := ct.MineCurrentBlock([]types.Transaction{txn})
	blocks = append(blocks, block)

	// Mine several more blocks.
	recentID := block.ID()
	for i := 0; i < depth; i++ {
		intTarget := ct.CurrentTarget().Int()
		safeIntTarget := intTarget.Div(intTarget, big.NewInt(2))
		block = MineTestingBlock(recentID, types.CurrentTimestamp(), ct.Payouts(ct.Height()+2+types.BlockHeight(i), nil), nil, types.IntToTarget(safeIntTarget))
		blocks = append(blocks, block)
		recentID = block.ID()
	}

	return
}

// TestComplexForking creates multiple states and sets up multiple forking
// scenarios between them to check consistency during forking.
func TestComplexForking(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Need to grab a single time to make sure that each state ends up with the
	// same genesis hash.
	time := types.CurrentTimestamp()
	s1 := createGenesisState(time, types.ZeroUnlockHash, types.ZeroUnlockHash)
	s2 := createGenesisState(time, types.ZeroUnlockHash, types.ZeroUnlockHash)
	s3 := createGenesisState(time, types.ZeroUnlockHash, types.ZeroUnlockHash)
	a1 := NewConsensusTester(t, s1)
	a2 := NewConsensusTester(t, s2)
	a3 := NewConsensusTester(t, s3)

	// Verify that the three states have the same initial hash.
	if s1.StateHash() != s2.StateHash() {
		t.Fatal("starting states have different hashes - can't run test")
	}
	if s1.StateHash() != s3.StateHash() {
		t.Fatal("starting states have different hashes - can't run test")
	}

	// Get state1 and state2 on different forks, s3 will follow s1 at this
	// point.
	block1 := MineTestingBlock(s1.CurrentBlock().ID(), time, a1.Payouts(s1.Height()+1, nil), nil, s1.CurrentTarget())
	err := s1.AcceptBlock(block1)
	if err != nil {
		t.Fatal(err)
	}
	block2 := MineTestingBlock(s2.CurrentBlock().ID(), time+1, a2.Payouts(s2.Height()+1, nil), nil, s2.CurrentTarget())
	err = s2.AcceptBlock(block2)
	if err != nil {
		t.Fatal(err)
	}
	if s1.StateHash() == s2.StateHash() {
		t.Fatal("failed to get states on different forks")
	}
	err = s3.AcceptBlock(block1)
	if err != nil {
		t.Fatal(err)
	}

	// Mine several blocks on each state.
	for i := 0; i < 2; i++ {
		// state 1 mining.
		block1 = a1.MineCurrentBlock(nil)
		err = s1.AcceptBlock(block1)
		if err != nil {
			t.Fatal(err)
		}
		err = s3.AcceptBlock(block1)
		if err != nil {
			t.Fatal(err)
		}

		// state 2 mining.
		block2 = a2.MineCurrentBlock(nil)
		err = s2.AcceptBlock(block2)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Consistency checks, sprinkled throughout the forking process to help
	// catch any latent problems.
	a1.ConsistencyChecks()
	a2.ConsistencyChecks()
	a3.ConsistencyChecks()

	// Put state2 ahead of state1 and then give all of the state2 blocks to
	// state1, causing state1 to fork. State3 is left alone.
	for i := 0; i < 2; i++ {
		block2 = a2.MineCurrentBlock(nil)
		err = s2.AcceptBlock(block2)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := types.BlockHeight(1); i <= s2.Height(); i++ {
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

	// Consistency checks, sprinkled throughout the forking process to help
	// catch any latent problems.
	a1.ConsistencyChecks()
	a2.ConsistencyChecks()
	a3.ConsistencyChecks()

	// Put state3 ahead of state 1&2 on state1's original block path. Then feed
	// all of state 3's blocks to state1, which will cause state1 to fork to
	// state3. State1 will be applying diffs for many of the blocks instead of
	// generating them, which means a different codepath will be followed from
	// the previous fork.
	s3InitialHeight := s3.Height()
	for i := 0; i < 4; i++ {
		block3 := a3.MineCurrentBlock(nil)
		err = s3.AcceptBlock(block3)
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

	// Consistency checks, sprinkled throughout the forking process to help
	// catch any latent problems.
	a1.ConsistencyChecks()
	a2.ConsistencyChecks()
	a3.ConsistencyChecks()

	// State1 hash should match state3 hash.
	if s1.StateHash() != s3.StateHash() {
		t.Fatal("hashes don't match after trying to force an applying fork")
	}

	// Mine a bunch of blocks on state2 where the first block has an invalid
	// signature. Feed them all to state1. The result should be that state1
	// attempts to fork, finds the invalid singature, and then reverts to its
	// original position with state3.
	invalidBlocks := a2.MineInvalidSignatureBlockSet(2)
	for _, block := range invalidBlocks[:len(invalidBlocks)-1] {
		err = s1.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = s1.AcceptBlock(invalidBlocks[len(invalidBlocks)-1])
	if err != crypto.ErrInvalidSignature {
		t.Fatal("expecting invalid signature:", err)
	}

	// State1 hash should match state3 hash.
	if s1.StateHash() != s3.StateHash() {
		t.Fatal("hashes don't match after trying to force an invalid fork")
	}

	// Consistency checks, sprinkled throughout the forking process to help
	// catch any latent problems.
	a1.ConsistencyChecks()
	a2.ConsistencyChecks()
	a3.ConsistencyChecks()
}
