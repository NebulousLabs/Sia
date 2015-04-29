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
	MaxCatchUpBlocks = 50
)

// threadedResynchronize continuously calls Synchronize on a random peer every
// few minutes. This helps prevent unintentional desychronization in the event
// that broadcasts start failing.
func (s *State) threadedResynchronize() {
	for {
		go func() {
			// The set of connected peers is small and randomly ordered, so
			// just naively iterate through them until a Synchronize succeeds.
			for _, peer := range s.gateway.Peers() {
				if s.Synchronize(peer) == nil {
					break
				}
			}
		}()
		time.Sleep(time.Minute * 2)
	}
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
// TODO: don't run two Synchronize threads at the same time
func (s *State) Synchronize(peer modules.NetAddress) error {
	// loop until there are no more blocks available
	moreAvailable := true
	for moreAvailable {
		// get blockIDs to send
		id := s.mu.RLock()
		history := s.blockHistory()
		s.mu.RUnlock(id)

		// perform RPC
		var newBlocks []types.Block
		err := s.gateway.RPC(peer, "SendBlocks", func(conn modules.PeerConn) error {
			err := encoding.WriteObject(conn, history)
			if err != nil {
				return err
			}
			err = encoding.ReadObject(conn, &newBlocks, MaxCatchUpBlocks*types.BlockSizeLimit)
			if err != nil {
				return err
			}
			return encoding.ReadObject(conn, &moreAvailable, 1)
		})
		if err != nil {
			return err
		}

		// integrate received blocks
		for _, block := range newBlocks {
			// TODO: don't Broadcast these blocks
			acceptErr := s.AcceptBlock(block)
			if acceptErr != nil {
				// TODO: If the error is a FutureTimestampErr, need to wait before trying the
				// block again.
			}
		}
	}
	return nil
}

// SendBlocks returns a sequential set of blocks based on the 32 input block
// IDs. The most recent known ID is used as the starting point, and up to
// 'MaxCatchUpBlocks' from that BlockHeight onwards are returned. It also
// sends a boolean indicating whether more blocks are available.
func (s *State) SendBlocks(conn modules.PeerConn) error {
	// Read known blocks.
	var knownBlocks [32]types.BlockID
	err := encoding.ReadObject(conn, &knownBlocks, 32*crypto.HashSize)
	if err != nil {
		return err
	}

	// Find the most recent block from knownBlocks that is in our current path.
	id := s.mu.RLock()
	found := false
	var start types.BlockHeight
	for _, id := range knownBlocks {
		bn, exists := s.blockMap[id]
		if exists && bn.height <= s.height() && id == s.currentPath[bn.height] {
			found = true
			start = bn.height + 1 // start at child
			break
		}
	}
	s.mu.RUnlock(id)

	// If we didn't find any matching blocks, or if we're already
	// synchronized, don't send any blocks. The genesis block should be
	// included in knownBlocks, so if no matching blocks are found, the caller
	// is probably on a different blockchain altogether.
	if !found || start > s.Height() {
		// Send 0 blocks.
		err = encoding.WriteObject(conn, []types.Block{})
		if err != nil {
			return err
		}
		// Indicate that no more blocks are available.
		return encoding.WriteObject(conn, false)
	}

	// Fetch blocks to send.
	id = s.mu.RLock()
	height := s.height()
	var blocks []types.Block
	for i := start; i <= height && i < start+MaxCatchUpBlocks; i++ {
		node, exists := s.blockMap[s.currentPath[i]]
		if !exists {
			if build.DEBUG {
				panic("blockMap is missing a block whose ID is in the currentPath")
			}
			break
		}
		blocks = append(blocks, node.block)
	}
	// Indicate whether more blocks are available.
	more := start+MaxCatchUpBlocks < height
	s.mu.RUnlock(id)

	err = encoding.WriteObject(conn, blocks)
	if err != nil {
		return err
	}

	return encoding.WriteObject(conn, more)
}

// blockHistory returns up to 32 BlockIDs, starting with the 12 most recent
// BlockIDs and then doubling in step size until the genesis block is reached.
// The genesis block is always included. This array of BlockIDs is used to
// establish a shared commonality between peers during synchronization.
func (s *State) blockHistory() (blockIDs [32]types.BlockID) {
	knownBlocks := make([]types.BlockID, 0, 32)
	step := types.BlockHeight(1)
	for height := s.height(); ; height -= step {
		knownBlocks = append(knownBlocks, s.currentPath[height])

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
	knownBlocks = append(knownBlocks, s.currentPath[0])

	copy(blockIDs[:], knownBlocks)
	return
}
