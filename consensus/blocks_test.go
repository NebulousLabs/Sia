package consensus

import (
	"math/big"
	"testing"
	"time"
)

// mineTestingBlock accepts a bunch of parameters for a block and then grinds
// blocks until a block with the appropriate target is found.
func mineTestingBlock(parent BlockID, timestamp Timestamp, minerAddress CoinAddress, txns []Transaction, target Target) (b Block, err error) {
	b = Block{
		ParentBlockID: parent,
		Timestamp:     timestamp,
		MinerAddress:  minerAddress,
		Transactions:  txns,
	}

	for !b.CheckTarget(target) && b.Nonce < 1000*1000 {
		b.Nonce++
	}
	if !b.CheckTarget(target) {
		panic("mineTestingBlock failed!")
	}
	return
}

// mineValidBlock picks valid/legal parameters for a block and then uses them
// to call mineTestingBlock.
func mineValidBlock(s *State) (b Block, err error) {
	return mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), CoinAddress{}, nil, s.CurrentTarget())
}

// testBlockTimestamps submits a block to the state with a timestamp that is
// too early and a timestamp that is too late, and verifies that each get
// rejected.
func testBlockTimestamps(t *testing.T, s *State) {
	// Create a block with a timestamp that is too early.
	b, err := mineTestingBlock(s.CurrentBlock().ID(), s.EarliestTimestamp()-1, CoinAddress{}, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != EarlyTimestampErr {
		t.Error("unexpected error when submitting a too-early timestamp:", err)
	}

	// Create a block with a timestamp that is too late.
	b, err = mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix())+10+FutureThreshold, CoinAddress{}, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != FutureBlockErr {
		t.Error("unexpected error when submitting a too-early timestamp:", err)
	}
}

// testEmptyBlock adds an empty block to the state and checks for errors.
func testEmptyBlock(t *testing.T, s *State) {
	// Get prior stats about the state.
	bbLen := len(s.badBlocks)
	bmLen := len(s.blockMap)
	mpLen := len(s.missingParents)
	cpLen := len(s.currentPath)
	uoLen := len(s.unspentOutputs)
	ocLen := len(s.openContracts)
	beforeStateHash := s.StateHash()

	// Mine and submit a block
	b, err := mineValidBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	afterStateHash := s.StateHash()
	if afterStateHash == beforeStateHash {
		t.Error("StateHash is unchanged after applying an empty block")
	}

	// Check that the state has updated as expected:
	//		bad blocks should not change
	//		blockMap should get 1 new member
	//		missingParents should not change
	//		currentPath should get 1 new member
	//		unspentOutputs should grow by at least 1 (missedProofs can make it grow by more)
	//		openContracts should not grow (contracts may close during the block though)
	if bbLen != len(s.badBlocks) ||
		bmLen != len(s.blockMap)-1 ||
		mpLen != len(s.missingParents) ||
		cpLen != len(s.currentPath)-1 ||
		uoLen > len(s.unspentOutputs)-1 ||
		ocLen < len(s.openContracts) {
		t.Error("state changed unexpectedly after accepting an empty block")
	}
	if s.currentBlockID != b.ID() {
		t.Error("the state's current block id did not change after getting a new block")
	}
	if s.currentPath[s.Height()] != b.ID() {
		t.Error("the state's current path didn't update correctly after accepting a new block")
	}
	bn, exists := s.blockMap[b.ID()]
	if !exists {
		t.Error("the state's block map did not update correctly after getting an empty block")
	}
	_, exists = s.unspentOutputs[b.SubsidyID()]
	if !exists {
		t.Error("the blocks subsidy output did not get added to the set of unspent outputs")
	}

	// Check that the diffs have been generated, and that they represent the
	// actual changes to the state.
	if !bn.diffsGenerated {
		t.Error("diffs were not generated on the new block")
	}
	s.invertRecentBlock()
	if beforeStateHash != s.StateHash() {
		t.Error("state is different after applying and removing diffs")
	}
	s.applyBlockNode(bn)
	if afterStateHash != s.StateHash() {
		t.Error("state is different after generateApply, remove, and applying diffs")
	}
}

// testLargeBlock creates a block that is too large to be accepted by the state
// and checks that it actually gets rejected.
func testLargeBlock(t *testing.T, s *State) {
	txns := make([]Transaction, 1)
	bigData := string(make([]byte, BlockSizeLimit))
	txns[0] = Transaction{
		ArbitraryData: []string{bigData},
	}
	b, err := mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), CoinAddress{}, txns, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}

	err = s.AcceptBlock(b)
	if err != LargeBlockErr {
		t.Error(err)
	}
}

// testMissedTarget tries to submit a block that does not meet the target for the next block.
func testMissedTarget(t *testing.T, s *State) {
	// Mine a block that doesn't meet the target.
	b := Block{
		ParentBlockID: s.CurrentBlock().ID(),
		Timestamp:     Timestamp(time.Now().Unix()),
	}
	for b.CheckTarget(s.CurrentTarget()) && b.Nonce < 1000*1000 {
		b.Nonce++
	}
	if b.CheckTarget(s.CurrentTarget()) {
		panic("unable to mine a block with a failing target (lol)")
	}

	err := s.AcceptBlock(b)
	if err != MissedTargetErr {
		t.Error("Block with low target is not being rejected")
	}
}

// testMultiOrphanBlock creates multiple orphans to a single parent, with one
// set of orphans that goes two deep. It then checks that after forking, all
// orphans have been added to the tree and removed from the orphan map.
func testMultiOrphanBlock(t *testing.T, s *State) {
	// Mine the parent block.
	parent, err := mineValidBlock(s)
	if err != nil {
		t.Fatal(err)
	}

	// Get the orphan and orphan2 targets.
	parentTarget := s.CurrentTarget()
	orphanRat := new(big.Rat).Mul(parentTarget.Rat(), MaxAdjustmentDown)
	orphanTarget := RatToTarget(orphanRat)
	orphan2Rat := new(big.Rat).Mul(orphanRat, MaxAdjustmentDown)
	orphan2Target := RatToTarget(orphan2Rat)

	// Mine 3 orphans with 'parent' as parent, and one orphan with another
	// orphan as a parent.
	//
	// The timestamp gets incremented each time so that we don't accidentally
	// mine the same block twice or end up with a too early block.
	orphanA, err := mineTestingBlock(parent.ID(), Timestamp(time.Now().Unix()), CoinAddress{}, nil, orphanTarget)
	if err != nil {
		t.Fatal(err)
	}
	orphanB, err := mineTestingBlock(parent.ID(), Timestamp(time.Now().Unix()+1), CoinAddress{}, nil, orphanTarget)
	if err != nil {
		t.Fatal(err)
	}
	orphanC, err := mineTestingBlock(parent.ID(), Timestamp(time.Now().Unix()+2), CoinAddress{}, nil, orphanTarget)
	if err != nil {
		t.Fatal(err)
	}
	orphan2, err := mineTestingBlock(orphanA.ID(), Timestamp(time.Now().Unix()+3), CoinAddress{}, nil, orphan2Target)
	if err != nil {
		t.Fatal(err)
	}

	// Submit the orphans to the state, followed by the parent.
	err = s.AcceptBlock(orphan2)
	if err != UnknownOrphanErr {
		t.Error("unexpected error, expecting UnknownOrphanErr:", err)
	}
	err = s.AcceptBlock(orphanA)
	if err != UnknownOrphanErr {
		t.Error("unexpected error, expecting UnknownOrphanErr:", err)
	}
	err = s.AcceptBlock(orphanB)
	if err != UnknownOrphanErr {
		t.Error("unexpected error, expecting UnknownOrphanErr:", err)
	}
	err = s.AcceptBlock(orphanC)
	if err != UnknownOrphanErr {
		t.Error("unexpected error, expecting UnknownOrphanErr:", err)
	}
	err = s.AcceptBlock(parent)
	if err != nil {
		t.Error("unexpected error:", err)
	}

	// Check that all blocks made it into the block tree and that the state
	// forked to the orphan2 block.
	_, exists := s.blockMap[orphan2.ID()]
	if !exists {
		t.Error("second layer orphan never made it into block map")
	}
	_, exists = s.blockMap[orphanA.ID()]
	if !exists {
		t.Error("first layer orphan never made it into block map")
	}
	_, exists = s.blockMap[orphanB.ID()]
	if !exists {
		t.Error("first layer orphan never made it into block map")
	}
	_, exists = s.blockMap[orphanC.ID()]
	if !exists {
		t.Error("first layer orphan never made it into block map")
	}
	_, exists = s.blockMap[parent.ID()]
	if !exists {
		t.Error("parent never made it into block map")
	}
	if s.currentBlockID != orphan2.ID() {
		t.Error("orphan 2 is not updates as the head block")
	}
	_, exists = s.missingParents[orphanA.ID()]
	if exists {
		t.Error("first orphan was never deleted from missing parents")
	}
	_, exists = s.missingParents[parent.ID()]
	if exists {
		t.Error("first orphan was never deleted from missing parents")
	}
}

// testOrphanBlock creates an orphan block and submits it to the state to check
// that orphans are handled correctly. Then it sumbmits the orphan's parent to
// check that the reconnection happens correctly.
func testOrphanBlock(t *testing.T, s *State) {
	beforeStateHash := s.StateHash()
	beforeHeight := s.Height()

	// Mine the parent of the orphan.
	parent, err := mineValidBlock(s)
	if err != nil {
		t.Fatal(err)
	}

	// Mine the orphan using a target that's guaranteed to be sufficient.
	parentTarget := s.CurrentTarget()
	orphanRat := new(big.Rat).Mul(parentTarget.Rat(), MaxAdjustmentDown)
	orphanTarget := RatToTarget(orphanRat)
	orphan, err := mineTestingBlock(parent.ID(), Timestamp(time.Now().Unix()), CoinAddress{}, nil, orphanTarget)
	if err != nil {
		t.Fatal(err)
	}

	// Submit the orphan and check that the block was ignored.
	err = s.AcceptBlock(orphan)
	if err != UnknownOrphanErr {
		t.Error("unexpected error upon submitting an unknown orphan block:", err)
	}
	if s.StateHash() != beforeStateHash {
		t.Error("state hash changed after submitting an orphan block")
	}
	_, exists := s.blockMap[orphan.ID()]
	if exists {
		t.Error("orphan got added to the block map")
	}

	// Check that the KnownOrphan code is working as well.
	err = s.AcceptBlock(orphan)
	if err != KnownOrphanErr {
		t.Error("unexpected error upong submitting a known orphan:", err)
	}
	if s.StateHash() != beforeStateHash {
		t.Error("state hash changed after submitting an orphan block")
	}
	_, exists = s.blockMap[orphan.ID()]
	if exists {
		t.Error("orphan got added to the block map")
	}

	// Submit the parent and check that both the orphan and the parent get
	// accepted.
	err = s.AcceptBlock(parent)
	if err != nil {
		t.Error("unexpected error upon submitting the parent to an orphan:", err)
	}
	_, exists = s.blockMap[parent.ID()]
	if !exists {
		t.Error("parent block is not in the block map")
	}
	_, exists = s.blockMap[orphan.ID()]
	if !exists {
		t.Error("orphan block is not in the block map after being reconnected")
	}
	if s.currentBlockID != orphan.ID() {
		t.Error("the states current block is not the reconnected orphan")
	}
	if beforeHeight != s.Height()-2 {
		t.Error("height should now be reporting 2 new blocks.")
	}

	// Check that the orphan has been removed from the orphan map.
	_, exists = s.missingParents[parent.ID()]
	if exists {
		t.Error("orphan map was not cleaned out after orphans were connected")
	}
}

// testRepeatBlock submits a block to the state, and then submits the same
// block to the state. If anything in the state has changed, an error is noted.
func testRepeatBlock(t *testing.T, s *State) {
	// Add a non-repeat block to the state.
	b, err := mineValidBlock(s)
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Collect metrics about the state.
	bbLen := len(s.badBlocks)
	bmLen := len(s.blockMap)
	mpLen := len(s.missingParents)
	cpLen := len(s.currentPath)
	uoLen := len(s.unspentOutputs)
	ocLen := len(s.openContracts)
	stateHash := s.StateHash()

	// Submit the repeat block.
	err = s.AcceptBlock(b)
	if err != BlockKnownErr {
		t.Error("expecting BlockKnownErr, got", err)
	}

	// Compare the metrics and report an error if something has changed.
	if bbLen != len(s.badBlocks) ||
		bmLen != len(s.blockMap) ||
		mpLen != len(s.missingParents) ||
		cpLen != len(s.currentPath) ||
		uoLen != len(s.unspentOutputs) ||
		ocLen != len(s.openContracts) ||
		stateHash != s.StateHash() {
		t.Error("state changed after getting a repeat block.")
	}
}

// TestBlockTimestamps creates a new state and uses it to call
// testBlockTimestamps.
func TestBlockTimestamps(t *testing.T) {
	s := CreateGenesisState()
	testBlockTimestamps(t, s)
}

// TestEmptyBlock creates a new state and uses it to call testEmptyBlock.
func TestEmptyBlock(t *testing.T) {
	s := CreateGenesisState()
	testEmptyBlock(t, s)
}

// TestLargeBlock creates a new state and uses it to call testLargeBlock.
func TestLargeBlock(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	s := CreateGenesisState()
	testLargeBlock(t, s)
}

// TestMissedTarget creates a new state and uses it to call testMissedTarget.
func TestMissedTarget(t *testing.T) {
	s := CreateGenesisState()
	testMissedTarget(t, s)
}

// TestDoubleOrphanBlock creates a new state and used it to call
// testDoubleOrphanBlock.
func TestMultiOrphanBlock(t *testing.T) {
	s := CreateGenesisState()
	testMultiOrphanBlock(t, s)
}

// TestOrphanBlock creates a new state and uses it to call testOrphanBlock.
func TestOrphanBlock(t *testing.T) {
	s := CreateGenesisState()
	testOrphanBlock(t, s)
}

// TestRepeatBlock creates a new state and uses it to call testRepeatBlock.
func TestRepeatBlock(t *testing.T) {
	s := CreateGenesisState()
	testRepeatBlock(t, s)
}

// TODO: Complex transaction building => Financial transactions, contract
// transactions, and invalid forms of each. Bad outputs, many outputs, many
// inputs, many fees, bad fees, overflows, bad proofs, early proofs, arbitrary
// datas, bad signatures, too many signatures, repeat signatures.

// TODO: Build those transaction building functions as separate things, because
// you want to be able to probe complex transactions that have lots of juicy
// stuff.

// TODO: Fork probing. Fork between complex sets of blocks and see that the
// state hashes always end up at the same point, make sure long forking works
// well and that reversing when a transaction fails also works well.

// TODO: Test the actually method which is used to calculated the earliest
// legal timestamp for the next block. Like have some examples that should work
// out algebraically and make sure that earliest timestamp follows the rules
// layed out by the protocol. This should be done after we decide that the
// algorithm for calculating the earliest allowed timestamp is sufficient.

// TODO: Probe the difficulty adjustments, make sure that they are happening
// according to specification, moving as much as they should and that the
// clamps are being effective.
