package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrBadBlock        = errors.New("block is known to be invalid")
	ErrBlockKnown      = errors.New("block exists in block map")
	ErrEarlyTimestamp  = errors.New("block timestamp is too early")
	ErrFutureTimestamp = errors.New("block timestamp too far in future")
	ErrOrphan          = errors.New("block has no known parent")
	ErrLargeBlock      = errors.New("block is too large to be accepted")
	ErrMinerPayout     = errors.New("miner payout sum does not equal block subsidy")
	ErrMissedTarget    = errors.New("block does not meet target")
)

// checkMinerPayouts verifies that the sum of all the miner payouts is equal to
// the block subsidy (which is the coinbase + miner fees).
func (s *State) checkMinerPayouts(b types.Block) (err error) {
	// Sanity check - the block's parent needs to exist and be known.
	parentNode, exists := s.blockMap[b.ParentID]
	if !exists {
		if build.DEBUG {
			panic("misuse of checkMinerPayouts - block has no known parent")
		}
		return ErrOrphan
	}

	// Find the total subsidy for the miners: coinbase + fees.
	subsidy := types.CalculateCoinbase(parentNode.height + 1)
	for _, txn := range b.Transactions {
		for _, fee := range txn.MinerFees {
			subsidy = subsidy.Add(fee)
		}
	}

	// Find the sum of the miner payouts.
	var payoutSum types.Currency
	for _, payout := range b.MinerPayouts {
		if payout.Value.IsZero() {
			return errors.New("cannot have zero or negative miner payout")
		}
		payoutSum = payoutSum.Add(payout.Value)
	}

	// Return an error if the subsidy isn't equal to the payouts.
	if subsidy.Cmp(payoutSum) != 0 {
		return ErrMinerPayout
	}

	return
}

// validHeader does some early, low computation verification on the block.
func (s *State) validHeader(b types.Block) (err error) {
	// Grab the parent of the block.
	parent, exists := s.blockMap[b.ParentID]
	if !exists {
		return ErrOrphan
	}

	// Check the ID meets the target. This is one of the earliest checks to
	// enforce that blocks need to have committed to a large amount of work
	// before being verified - a DoS protection.
	if !b.CheckTarget(parent.target) {
		return ErrMissedTarget
	}

	// Check that the block is the correct size.
	if uint64(len(encoding.Marshal(b))) > types.BlockSizeLimit {
		return ErrLargeBlock
	}

	// If timestamp is too far in the past, reject and put in bad blocks.
	if parent.earliestChildTimestamp() > b.Timestamp {
		return ErrEarlyTimestamp
	}

	// Check that the block is not too far in the future. An external process
	// will need to be responsible for resubmitting the block once it is no
	// longer in the future.
	if b.Timestamp > types.CurrentTimestamp()+types.FutureThreshold {
		return ErrFutureTimestamp
	}

	// Verify that the miner payouts sum to the total amount of fees allowed to
	// be collected by the miners.
	err = s.checkMinerPayouts(b)
	if err != nil {
		return
	}

	return
}

// addBlockToTree inserts a block into the blockNode tree by adding it to its
// parent's list of children. If the new blockNode is heavier than the current
// node, the blockchain is forked.
func (s *State) addBlockToTree(b types.Block) (revertedNodes, appliedNodes []*blockNode, err error) {
	parentNode := s.blockMap[b.ParentID]
	newNode := parentNode.newChild(b)

	// Add the node to the block map
	s.blockMap[b.ID()] = newNode

	if newNode.heavierThan(s.currentBlockNode()) {
		revertedNodes, appliedNodes, err = s.forkBlockchain(newNode)
		if err != nil {
			return nil, nil, err
		}
		return revertedNodes, appliedNodes, nil
	}
	return nil, nil, nil
}

// acceptBlock is the internal consensus function for adding blocks. There is
// no block relaying.
func (s *State) acceptBlock(b types.Block) error {
	_, exists := s.badBlocks[b.ID()]
	if exists {
		return ErrBadBlock
	}
	_, exists = s.blockMap[b.ID()]
	if exists {
		return ErrBlockKnown
	}

	// Check that the header is valid given the other blocks we know. This
	// happens before checking that the transactions are intrinsically valid
	// because it's a much cheaper operation for us to verify, and it's
	// expensive for an attacker to spoof the header.
	err := s.validHeader(b)
	if err != nil {
		return err
	}

	// Try adding the block to the tree.
	revertedNodes, appliedNodes, err := s.addBlockToTree(b)
	if err != nil {
		return err
	}
	if len(appliedNodes) > 0 {
		s.updateSubscribers(revertedNodes, appliedNodes)
	}

	// Sanity check, if applied nodes is len 0, revertedNodes should also be
	// len 0.
	if build.DEBUG {
		if len(appliedNodes) == 0 && len(revertedNodes) != 0 {
			panic("appliedNodes and revertedNodes are mismatched!")
		}
	}

	return nil
}

// AcceptBlock will add a block to the state, forking the blockchain if it is
// on a fork that is heavier than the current fork. If the block is accepted,
// it will be relayed to connected peers. This function should only be called
// for new blocks.
func (s *State) AcceptBlock(b types.Block) error {
	lockID := s.mu.Lock()
	defer s.mu.Unlock(lockID)

	// Set the flag to do full verification.
	s.fullVerification = true
	err := s.acceptBlock(b)
	if err != nil {
		return err
	}

	// Broadcast the new block to all peers. This is an expensive operation, and not necessary during synchronize or
	go s.gateway.Broadcast("RelayBlock", b)

	return nil
}

// RelayBlock is an RPC that accepts a block from a peer.
func (s *State) RelayBlock(conn modules.PeerConn) error {
	// Decode the block from the connection.
	var b types.Block
	err := encoding.ReadObject(conn, &b, types.BlockSizeLimit)
	if err != nil {
		return err
	}

	// Submit the block to the state.
	err = s.AcceptBlock(b)
	if err == ErrOrphan {
		// If the block is an orphan, try to find the parents. The block is
		// thrown away, will be received again during the synchronize.
		go s.Synchronize(modules.NetAddress(conn.RemoteAddr().String()))
	}
	if err != nil {
		return err
	}

	// Check if the block is in the current path (sanity check first). If the
	// block is not in the current path, then it not a part of the longest
	// known fork. Broadcast is not called and an error is returned.
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)
	height, exists := s.heightOfBlock(b.ID())
	if !exists {
		if build.DEBUG {
			panic("could not get the height of a block that did not return an error when being accepted into the state")
		}
		return errors.New("consensus set malfunction")
	}
	currentPathBlock, exists := s.blockAtHeight(height)
	if !exists || b.ID() != currentPathBlock.ID() {
		return errors.New("block added, but it does not extend the consensus set height")
	}

	return nil
}
