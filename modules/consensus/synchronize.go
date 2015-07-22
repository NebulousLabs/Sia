package consensus

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	MaxCatchUpBlocks          = 50
	MaxSynchronizeAttempts    = 8
	ResynchronizePeerTimeout  = time.Second * 30
	ResynchronizeBatchTimeout = time.Minute * 3
)

// receiveBlocks is the calling end of the SendBlocks RPC.
func (s *ConsensusSet) receiveBlocks(conn modules.PeerConn) error {
	// get blockIDs to send
	lockID := s.mu.RLock()
	history := s.blockHistory()
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
			s.verificationRigor = fullVerification
			acceptErr := s.acceptBlock(block)
			s.mu.Unlock(lockID)
			// these errors are benign
			if acceptErr == modules.ErrNonExtendingBlock || acceptErr == ErrBlockKnown {
				acceptErr = nil
			}
			if acceptErr != nil {
				return acceptErr
			}
		}
	}

	return nil
}

// blockHistory returns up to 32 BlockIDs, starting with the 12 most recent
// BlockIDs and then doubling in step size until the genesis block is reached.
// The genesis block is always included. This array of BlockIDs is used to
// establish a shared commonality between peers during synchronization.
func (s *ConsensusSet) blockHistory() (blockIDs [32]types.BlockID) {
	knownBlocks := make([]types.BlockID, 0, 32)
	step := types.BlockHeight(1)
	for height := s.height(); ; height -= step {
		knownBlocks = append(knownBlocks, s.db.path(height))

		// after 12, start doubling
		if len(knownBlocks) >= 12 {
			step *= 2
		}

		// this check has to come before height -= step;
		// otherwise we might underflow
		if height <= step {
			break
		}
	}
	// always include the genesis block
	knownBlocks = append(knownBlocks, s.db.path(0))

	copy(blockIDs[:], knownBlocks)
	return
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
	for _, id := range knownBlocks {
		bn, exists := s.blockMap[id]
		if exists && bn.height <= s.height() && id == s.db.path(bn.height) {
			found = true
			start = bn.height + 1 // start at child
			break
		}
	}
	s.mu.RUnlock(lockID)

	// If no matching blocks are found, or if the caller has all known blocks,
	// don't send any blocks.
	if !found || start > s.Height() {
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
		{
			height := s.height()
			// TODO: unit test for off-by-one errors here
			for i := start; i <= height && i < start+MaxCatchUpBlocks; i++ {
				node, exists := s.blockMap[s.db.path(i)]
				if build.DEBUG && !exists {
					panic("blockMap is missing a block whose ID is in the current path")
				}
				blocks = append(blocks, node.block)
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

// Synchronize synchronizes the local consensus set (i.e. the blockchain) with
// the network consensus set. The process is as follows: synchronize asks a
// peer for new blocks. The requester sends 32 block IDs, starting with the 12
// most recent and then progressing exponentially backwards to the genesis
// block. The receiver uses these blocks to find the most recent block seen by
// both peers. From this starting height, it transmits blocks sequentially.
// The requester then integrates these blocks into its consensus set. Multiple
// such transmissions may be required to fully synchronize.
//
// TODO: Synchronize is a blocking call that involved network traffic. This
// seems to break convention, but I'm not certain. It does seem weird though.
func (s *ConsensusSet) Synchronize(peer modules.NetAddress) error {
	return s.gateway.RPC(peer, "SendBlocks", s.receiveBlocks)
}
