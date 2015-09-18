package consensus

import (
	"errors"
	"time"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrBadMinerPayouts        = errors.New("miner payout sum does not equal block subsidy")
	ErrDoSBlock               = errors.New("block is known to be invalid")
	ErrEarlyTimestamp         = errors.New("block timestamp is too early")
	ErrExtremeFutureTimestamp = errors.New("block timestamp too far in future, discarded")
	ErrFutureTimestamp        = errors.New("block timestamp too far in future, but saved for later use")
	ErrInconsistentSet        = errors.New("consensus set is not in a consistent state")
	ErrLargeBlock             = errors.New("block is too large to be accepted")
	ErrMissedTarget           = errors.New("block does not meet target")
	ErrOrphan                 = errors.New("block has no known parent")
)

// validHeader does some early, low computation verification on the block.
func (cs *ConsensusSet) validHeader(tx *bolt.Tx, b types.Block) error {
	// See if the block is known already.
	id := b.ID()
	_, exists := cs.dosBlocks[id]
	if exists {
		return ErrDoSBlock
	}

	// Check if the block is already known.
	blockMap := tx.Bucket(BlockMap)
	if blockMap.Get(id[:]) != nil {
		return modules.ErrBlockKnown
	}

	// Check for the parent.
	parentBytes := blockMap.Get(b.ParentID[:])
	if parentBytes == nil {
		return ErrOrphan
	}
	var parent processedBlock
	err := encoding.Unmarshal(parentBytes, &parent)
	if err != nil {
		return err
	}

	// Check that the target of the new block is sufficient.
	if !b.CheckTarget(parent.ChildTarget) {
		return ErrMissedTarget
	}

	// Check that the timestamp is not too far in the past to be
	// acceptable.
	if earliestChildTimestamp(blockMap, &parent) > b.Timestamp {
		return ErrEarlyTimestamp
	}

	// Check that the block is below the size limit.
	if uint64(len(encoding.Marshal(b))) > types.BlockSizeLimit {
		return ErrLargeBlock
	}

	// If the block is in the extreme future, return an error and do nothing
	// more with the block. There is an assumption that by the time the extreme
	// future arrives, this block will no longer be a part of the longest fork
	// because it will have been ignored by all of the miners.
	if b.Timestamp > types.CurrentTimestamp()+types.ExtremeFutureThreshold {
		return ErrExtremeFutureTimestamp
	}

	// Verify that the miner payouts are valid.
	if !b.CheckMinerPayouts(parent.Height + 1) {
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
func (cs *ConsensusSet) addBlockToTree(b types.Block) (revertedBlocks, appliedBlocks []*processedBlock, err error) {
	var nonExtending bool
	err = cs.db.Update(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, b.ParentID)
		if build.DEBUG && err != nil {
			panic(err)
		}
		currentNode := currentProcessedBlock(tx)
		newNode, err := cs.newChild(tx, pb, b)
		if err != nil {
			return err
		}

		// modules.ErrNonExtendingBlock should be returned if the block does
		// not extend the current blockchain, however the changes from newChild
		// should be comitted (which means 'nil' must be returned). A flag is
		// set to indicate that modules.ErrNonExtending should be returned.
		nonExtending = !newNode.heavierThan(currentNode)
		if nonExtending {
			return nil
		}
		revertedBlocks, appliedBlocks, err = cs.forkBlockchain(tx, newNode)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	if nonExtending {
		return nil, nil, modules.ErrNonExtendingBlock
	}
	return revertedBlocks, appliedBlocks, nil
}

// acceptBlock is the internal consensus function for adding
// blocks. There is no block relaying. acceptBlock will verify all
// transactions. Trusted blocks, like those on disk, should already
// be processed and this function can be bypassed.
func (cs *ConsensusSet) acceptBlock(b types.Block) error {
	// Start verification inside of a bolt View tx.
	err := cs.db.View(func(tx *bolt.Tx) error {
		// Check that the header is valid. The header is checked first because it
		// is not computationally expensive to verify, but it is computationally
		// expensive to create.
		err := cs.validHeader(tx, b)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Try adding the block to the block tree. This call will perform
	// verification on the block before adding the block to the block tree. An
	// error is returned if verification fails or if the block does not extend
	// the longest fork.
	revertedBlocks, appliedBlocks, err := cs.addBlockToTree(b)
	if err != nil {
		// TODO: Add the incoming block to the list of DoS blocks.
		return err
	}
	if len(appliedBlocks) > 0 {
		cs.updateSubscribers(revertedBlocks, appliedBlocks)
	}

	// Sanity checks.
	if build.DEBUG {
		// If appliedBlocks is 0, revertedBlocks will also be 0.
		if len(appliedBlocks) == 0 && len(revertedBlocks) != 0 {
			panic("appliedBlocks and revertedBlocks are mismatched!")
		}
		// Check that the general setup makes sense.
		err = cs.db.View(func(tx *bolt.Tx) error {
			return checkConsistency(tx)
		})
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
		go cs.gateway.RPC(modules.NetAddress(conn.RemoteAddr().String()), "SendBlocks", cs.receiveBlocks)
	}
	if err != nil {
		return err
	}
	return nil
}
