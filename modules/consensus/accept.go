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
	ErrBadMinerPayouts        = errors.New("miner payout sum does not equal block subsidy")
	ErrBlockKnown             = errors.New("block exists in block map")
	ErrDoSBlock               = errors.New("block is known to be invalid")
	ErrEarlyTimestamp         = errors.New("block timestamp is too early")
	ErrExtremeFutureTimestamp = errors.New("block timestamp too far in future, discarded")
	ErrFutureTimestamp        = errors.New("block timestamp too far in future, but saved for later use")
	ErrLargeBlock             = errors.New("block is too large to be accepted")
	ErrMissedTarget           = errors.New("block does not meet target")
	ErrOrphan                 = errors.New("block has no known parent")
)

// validHeader does some early, low computation verification on the block.
func (cs *ConsensusSet) validHeader(b types.Block) error {
	// Grab the parent of the block and verify the ID of the child meets the
	// target. This is done as early as possible to enforce that any
	// block-related DoS must use blocks that have sufficient work.
	parent, exists := cs.blockMap[b.ParentID]
	if !exists {
		return ErrOrphan
	}
	if !b.CheckTarget(parent.childTarget) {
		return ErrMissedTarget
	}

	// Check that the block is below the size limit.
	if uint64(len(encoding.Marshal(b))) > types.BlockSizeLimit {
		return ErrLargeBlock
	}

	// Check that the timestamp is not in 'the past', where the past is defined
	// by earliestChildTimestamp.
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

	// Verify that the miner payouts are valid.
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
			cs.acceptBlock(b) // NOTE: Error is not handled.
		}()
		return ErrFutureTimestamp
	}

	return nil
}

// addBlockToTree inserts a block into the blockNode tree by adding it to its
// parent's list of children. If the new blockNode is heavier than the current
// node, the blockchain is forked to put the new block and its parents at the
// tip. An error will be returned if block verification fails or if the block
// does not extend the longest fork.
func (cs *ConsensusSet) addBlockToTree(b types.Block) (revertedNodes, appliedNodes []*processedBlock, err error) {
	parentNode := cs.db.getBlockMap(b.ParentID)
	// COMPATv0.4.0
	//
	// When validating/accepting a block, the types height needs to be set to
	// the height of the block that's being analyzed. After analysis is
	// finished, the height needs to be set to the height of the current block.
	types.CurrentHeightLock.Lock()
	types.CurrentHeight = parentNode.Height
	types.CurrentHeightLock.Unlock()
	defer func() {
		types.CurrentHeightLock.Lock()
		types.CurrentHeight = cs.height()
		types.CurrentHeightLock.Unlock()
	}()

	newNode := parentNode.newChild(b, cs.db)
	bn := cs.pbToBn(newNode)
	cs.blockMap[b.ID()] = bn
	err = cs.db.addBlockMap(newNode)
	if err != nil {
		return nil, nil, err
	}
	if newNode.heavierThan(cs.currentProcessedBlock()) {
		return cs.forkBlockchain(newNode)
	}
	return nil, nil, modules.ErrNonExtendingBlock
}

// acceptBlock is the internal consensus function for adding blocks. There is
// no block relaying. The speed of 'acceptBlock' is effected by the value of
// 'cs.verificationRigor'. If rigor is set to 'fullVerification', all of the
// transactions will be checked and verified. This is a requirement when
// receiving blocks from untrusted sources. When set to 'partialVerification',
// verification of transactions is skipped. This is acceptable when receiving
// blocks from a trust source, such as blocks that were previously verified and
// saved to disk. The value of 'cs.verificationRigor' should be set before
// 'acceptBlock' is called.
func (cs *ConsensusSet) acceptBlock(b types.Block) error {
	// See if the block is known already.
	_, exists := cs.dosBlocks[b.ID()]
	if exists {
		return ErrDoSBlock
	}
	_, exists = cs.blockMap[b.ID()]
	if exists {
		return ErrBlockKnown
	}

	// Check that the header is valid. The header is checked first because it
	// is not computationally expensive to verify, but it is computationally
	// expensive to create.
	err := cs.validHeader(b)
	if err != nil {
		return err
	}

	// Try adding the block to the block tree. This call will perform
	// verification on the block before adding the block to the block tree. An
	// error is returned if verification fails or if the block does not extend
	// the longest fork.
	revertedNodes, appliedNodes, err := cs.addBlockToTree(b)
	if err != nil {
		return err
	}
	if len(appliedNodes) > 0 {
		cs.updateSubscribers(revertedNodes, appliedNodes)
	}

	// Sanity checks.
	if build.DEBUG {
		// If appliedNodes is 0, revertedNodes will also be 0.
		if len(appliedNodes) == 0 && len(revertedNodes) != 0 {
			panic("appliedNodes and revertedNodes are mismatched!")
		}

		// After applying a block, the consensus set should be in a consistent
		// state.
		err = cs.checkConsistency()
		if err != nil {
			panic(err)
		}
	}

	return nil
}

// AcceptBlock will add a block to the state, forking the blockchain if it is
// on a fork that is heavier than the current fork. If the block is accepted,
// it will be relayed to connected peers. This function should only be called
// for new, untrusted blocks.
func (cs *ConsensusSet) AcceptBlock(b types.Block) error {
	lockID := cs.mu.Lock()
	defer cs.mu.Unlock(lockID)

	// Set the flag to do full verification.
	cs.verificationRigor = fullVerification
	err := cs.acceptBlock(b)
	if err != nil {
		return err
	}

	// Broadcast the new block to all peers. This is an expensive operation,
	// and not necessary during synchronize or when loading from disk.
	go cs.gateway.Broadcast("RelayBlock", b)

	return nil
}

// RelayBlock is an RPC that accepts a block from a peer.
func (cs *ConsensusSet) RelayBlock(conn modules.PeerConn) error {
	// Decode the block from the connection.
	var b types.Block
	err := encoding.ReadObject(conn, &b, types.BlockSizeLimit)
	if err != nil {
		return err
	}

	// Submit the block to the consensus set.
	err = cs.AcceptBlock(b)
	if err == ErrOrphan {
		// If the block is an orphan, try to find the parents. The block
		// received from the peer is discarded and will be downloaded again if
		// the parent is found.
		go cs.Synchronize(modules.NetAddress(conn.RemoteAddr().String()))
	}
	if err != nil {
		return err
	}
	return nil
}
