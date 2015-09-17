package consensus

import (
	"errors"
	"runtime"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	MaxCatchUpBlocks       = 10
	MaxSynchronizeAttempts = 8
)

// blockHistory returns up to 32 block ids, starting with recent blocks and
// then proving exponentially increasingly less recent blocks. The genesis
// block is always included as the last block. This block history can be used
// to find a common parent that is reasonably recent, usually the most recent
// common parent is found, but always a common parent within a factor of 2 is
// found.
func (s *ConsensusSet) blockHistory(tx *bolt.Tx) (blockIDs [32]types.BlockID) {
	height := blockHeight(tx)
	step := types.BlockHeight(1)
	// The final step is to include the genesis block, which is why `step <
	// height` is used as opposed to `step <= height`.
	for i := 0; i < 31; i++ {
		// Include the next block.
		blockID, err := getPath(tx, height)
		if build.DEBUG && err != nil {
			panic(err)
		}
		blockIDs[i] = blockID

		// Determine the height of the next block to include and then increase
		// the step size. The height must be decreased first to prevent
		// underflow.
		//
		// `i >= 9` means that the first 10 blocks will be included, and then
		// skipping will start.
		if i >= 9 {
			step *= 2
		}
		if height < step {
			break
		}
		height -= step
	}
	// Include the genesis block as the last element
	blockID, err := getPath(tx, 0)
	if build.DEBUG && err != nil {
		panic(err)
	}
	blockIDs[31] = blockID
	return blockIDs
}

// receiveBlocks is the calling end of the SendBlocks RPC.
func (s *ConsensusSet) receiveBlocks(conn modules.PeerConn) error {
	// get blockIDs to send
	lockID := s.mu.RLock()
	if !s.db.open {
		s.mu.RUnlock(lockID)
		return errors.New("database not open")
	}
	var history [32]types.BlockID
	_ = s.db.View(func(tx *bolt.Tx) error {
		history = s.blockHistory(tx)
		return nil
	})
	s.mu.RUnlock(lockID)
	if err := encoding.WriteObject(conn, history); err != nil {
		return err
	}

	// loop until no more blocks are available
	moreAvailable := true
	for moreAvailable {
		var newBlocks []types.Block
		if err := encoding.ReadObject(conn, &newBlocks, MaxCatchUpBlocks*types.BlockSizeLimit); err != nil {
			return err
		}
		if err := encoding.ReadObject(conn, &moreAvailable, 1); err != nil {
			return err
		}

		// integrate received blocks.
		for _, block := range newBlocks {
			// Blocks received during synchronize aren't trusted; activate full
			// verification.
			lockID := s.mu.Lock()
			if !s.db.open {
				s.mu.Unlock(lockID)
				return errors.New("database not open")
			}
			acceptErr := s.acceptBlock(block)
			s.mu.Unlock(lockID)
			// ErrNonExtendingBlock must be ignored until headers-first block
			// sharing is implemented.
			if acceptErr == modules.ErrNonExtendingBlock {
				acceptErr = nil
			}
			if acceptErr != nil {
				return acceptErr
			}

			// Yield the processor to give other processes time to grab a lock.
			// The Lock/Unlock cycle in this loop is very tight, and has been
			// known to prevent interrupts from getting lock access quickly.
			runtime.Gosched()
		}
	}

	return nil
}

// sendBlocks is the receiving end of the SendBlocks RPC. It returns a
// sequential set of blocks based on the 32 input block IDs. The most recent
// known ID is used as the starting point, and up to 'MaxCatchUpBlocks' from
// that BlockHeight onwards are returned. It also sends a boolean indicating
// whether more blocks are available.
func (s *ConsensusSet) sendBlocks(conn modules.PeerConn) error {
	// Read known blocks.
	var knownBlocks [32]types.BlockID
	err := encoding.ReadObject(conn, &knownBlocks, 32*crypto.HashSize)
	if err != nil {
		return err
	}

	// Find the most recent block from knownBlocks in the current path.
	found := false
	var start types.BlockHeight
	lockID := s.mu.RLock()
	if !s.db.open {
		s.mu.RUnlock(lockID)
		return errors.New("database not open")
	}
	for _, id := range knownBlocks {
		if s.db.inBlockMap(id) {
			pb := s.db.getBlockMap(id)
			if pb.Height <= s.height() && id == s.db.getPath(pb.Height) {
				found = true
				start = pb.Height + 1 // start at child
				break
			}
		}
	}

	// If no matching blocks are found, or if the caller has all known blocks,
	// don't send any blocks.
	h := s.height()
	s.mu.RUnlock(lockID)
	if !found || start > h {
		// Send 0 blocks.
		err = encoding.WriteObject(conn, []types.Block{})
		if err != nil {
			return err
		}
		// Indicate that no more blocks are available.
		return encoding.WriteObject(conn, false)
	}

	// Send the caller all of the blocks that they are missing.
	moreAvailable := true
	for moreAvailable {
		// Get the set of blocks to send.
		var blocks []types.Block
		lockID = s.mu.RLock()
		if !s.db.open {
			s.mu.RUnlock(lockID)
			return errors.New("database not open")
		}

		{
			height := s.height()
			// TODO: unit test for off-by-one errors here
			for i := start; i <= height && i < start+MaxCatchUpBlocks; i++ {
				node := s.db.getBlockMap(s.db.getPath(i))
				blocks = append(blocks, node.Block)
			}

			// TODO: Check for off-by-one here too.
			moreAvailable = start+MaxCatchUpBlocks < height
			start += MaxCatchUpBlocks
		}
		s.mu.RUnlock(lockID)

		// Send a set of blocks to the caller + a flag indicating whether more
		// are available.
		if err = encoding.WriteObject(conn, blocks); err != nil {
			return err
		}
		if err = encoding.WriteObject(conn, moreAvailable); err != nil {
			return err
		}
	}

	return nil
}
