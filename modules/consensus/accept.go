package consensus

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrDoSBlock               = errors.New("block is known to be invalid")
	ErrBlockKnown             = errors.New("block exists in block map")
	ErrEarlyTimestamp         = errors.New("block timestamp is too early")
	ErrExtremeFutureTimestamp = errors.New("block timestamp too far in future, discarded")
	ErrFutureTimestamp        = errors.New("block timestamp too far in future, but saved for later use")
	ErrOrphan                 = errors.New("block has no known parent")
	ErrLargeBlock             = errors.New("block is too large to be accepted")
	ErrBadMinerPayouts        = errors.New("miner payout sum does not equal block subsidy")
	ErrMissedTarget           = errors.New("block does not meet target")
)

// validHeader does some early, low computation verification on the block.
func (cs *State) validHeader(b types.Block) error {
	// Grab the parent of the block.
	parent, exists := cs.blockMap[b.ParentID]
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

	// If the block is in the extreme future, return an error and do nothing
	// more with the block. There is an assumption that by the time the extreme
	// future arrives, this block will no longer be a part of the longest fork
	// because it will have been ignored by all of the miners.
	if b.Timestamp > types.CurrentTimestamp()+types.ExtremeFutureThreshold {
		return ErrExtremeFutureTimestamp
	}

	// Verify that the miner payouts sum to the total amount of fees allowed to
	// be collected by the miners.
	if !b.CheckMinerPayouts(parent.height + 1) {
		return ErrBadMinerPayouts
	}

	// If the block is in the near future, but too far to be acceptable, then
	// the block will be saved and added to the consensus set after it is no
	// longer too far in the future. This is the last check because it's an
	// expensive check, and not worth performing if the payouts are incorrect.
	if b.Timestamp > types.CurrentTimestamp()+types.FutureThreshold {
		go func() {
			time.Sleep(time.Duration(b.Timestamp-(types.CurrentTimestamp()+types.FutureThreshold)) * time.Second)
			lockID := cs.mu.Lock()
			defer cs.mu.Unlock(lockID)
			cs.acceptBlock(b) // TODO: How does this error get handled?
		}()
		return ErrFutureTimestamp
	}

	return nil
}

// addBlockToTree inserts a block into the blockNode tree by adding it to its
// parent's list of children. If the new blockNode is heavier than the current
// node, the blockchain is forked.
func (cs *State) addBlockToTree(b types.Block) (revertedNodes, appliedNodes []*blockNode, err error) {
	parentNode := cs.blockMap[b.ParentID]
	newNode := parentNode.newChild(b)

	// Add the node to the block map
	cs.blockMap[b.ID()] = newNode

	if newNode.heavierThan(cs.currentBlockNode()) {
		revertedNodes, appliedNodes, err = cs.forkBlockchain(newNode)
		if err != nil {
			return nil, nil, err
		}
		return revertedNodes, appliedNodes, nil
	}
	return nil, nil, nil
}

// acceptBlock is the internal consensus function for adding blocks. There is
// no block relaying.
func (cs *State) acceptBlock(b types.Block) error {
	_, exists := cs.dosBlocks[b.ID()]
	if exists {
		return ErrDoSBlock
	}
	_, exists = cs.blockMap[b.ID()]
	if exists {
		return ErrBlockKnown
	}

	// Check that the header is valid given the other blocks we know. This
	// happens before checking that the transactions are intrinsically valid
	// because it's a much cheaper operation for us to verify, and it's
	// expensive for an attacker to spoof the header.
	err := cs.validHeader(b)
	if err != nil {
		return err
	}

	// Try adding the block to the tree.
	revertedNodes, appliedNodes, err := cs.addBlockToTree(b)
	if err != nil {
		return err
	}
	if len(appliedNodes) > 0 {
		cs.updateSubscribers(revertedNodes, appliedNodes)
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
func (cs *State) AcceptBlock(b types.Block) error {
	lockID := cs.mu.Lock()
	defer cs.mu.Unlock(lockID)

	// Set the flag to do full verification.
	cs.fullVerification = true
	err := cs.acceptBlock(b)
	if err != nil {
		return err
	}

	// Broadcast the new block to all peers. This is an expensive operation, and not necessary during synchronize or
	go cs.gateway.Broadcast("RelayBlock", b)

	return nil
}

// RelayBlock is an RPC that accepts a block from a peer.
func (cs *State) RelayBlock(conn modules.PeerConn) error {
	// Decode the block from the connection.
	var b types.Block
	err := encoding.ReadObject(conn, &b, types.BlockSizeLimit)
	if err != nil {
		return err
	}

	// Submit the block to the state.
	err = cs.AcceptBlock(b)
	if err == ErrOrphan {
		// If the block is an orphan, try to find the parents. The block is
		// thrown away, will be received again during the synchronize.
		go cs.Synchronize(modules.NetAddress(conn.RemoteAddr().String()))
	}
	if err != nil {
		return err
	}

	// Check if the block is in the current path (sanity check first). If the
	// block is not in the current path, then it not a part of the longest
	// known fork. Broadcast is not called and an error is returned.
	lockID := cs.mu.RLock()
	defer cs.mu.RUnlock(lockID)
	height, exists := cs.heightOfBlock(b.ID())
	if !exists {
		if build.DEBUG {
			panic("could not get the height of a block that did not return an error when being accepted into the state")
		}
		return errors.New("consensus set malfunction")
	}
	currentPathBlock, exists := cs.blockAtHeight(height)
	if !exists || b.ID() != currentPathBlock.ID() {
		return errors.New("block added, but it does not extend the consensus set height")
	}

	return nil
}
