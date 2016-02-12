package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	// MaxCatchUpBlocks is the maxiumum number of blocks that can be given to
	// the consensus set in a single iteration during the initial blockchain
	// download.
	MaxCatchUpBlocks = func() types.BlockHeight {
		switch build.Release {
		case "testing":
			return 3
		case "standard":
			return 10
		case "dev":
			return 10
		default:
			panic("unrecognized build.Release")
		}
	}()
)

// blockHistory returns up to 32 block ids, starting with recent blocks and
// then proving exponentially increasingly less recent blocks. The genesis
// block is always included as the last block. This block history can be used
// to find a common parent that is reasonably recent, usually the most recent
// common parent is found, but always a common parent within a factor of 2 is
// found.
func blockHistory(tx *bolt.Tx) (blockIDs [32]types.BlockID) {
	height := blockHeight(tx)
	step := types.BlockHeight(1)
	// The final step is to include the genesis block, which is why the final
	// element is skipped during iteration.
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

// threadedReceiveBlocks is the calling end of the SendBlocks RPC.
func (cs *ConsensusSet) threadedReceiveBlocks(conn modules.PeerConn) error {
	// Get blockIDs to send.
	var history [32]types.BlockID
	err := cs.db.View(func(tx *bolt.Tx) error {
		history = blockHistory(tx)
		return nil
	})
	if err != nil {
		return err
	}

	// Send the block ids.
	if err := encoding.WriteObject(conn, history); err != nil {
		return err
	}

	// Broadcast the last block accepted. This functionality is in a defer to
	// ensure that a block is always broadcast if any blocks are accepted. This
	// is to stop an attacker from preventing block broadcasts.
	chainExtended := false
	defer func() {
		if chainExtended {
			// The last block received will be the current block since
			// managedAcceptBlock only returns nil if a block extends the longest chain.
			currentBlock := cs.CurrentBlock()
			go cs.gateway.Broadcast("RelayBlock", currentBlock)
		}
	}()

	// Read blocks off of the wire and add them to the consensus set until
	// there are no more blocks available.
	moreAvailable := true
	for moreAvailable {
		// Read a slice of blocks from the wire.
		var newBlocks []types.Block
		if err := encoding.ReadObject(conn, &newBlocks, uint64(MaxCatchUpBlocks)*types.BlockSizeLimit); err != nil {
			return err
		}
		if err := encoding.ReadObject(conn, &moreAvailable, 1); err != nil {
			return err
		}

		// Integrate the blocks into the consensus set.
		for _, block := range newBlocks {
			// Call managedAcceptBlock instead of AcceptBlock so as not to broadcast
			// every block.
			acceptErr := cs.managedAcceptBlock(block)
			// Set a flag to indicate that we should broadcast the last block received.
			if acceptErr == nil {
				chainExtended = true
			}
			// ErrNonExtendingBlock must be ignored until headers-first block
			// sharing is implemented, block already in database should also be
			// ignored.
			if acceptErr == modules.ErrNonExtendingBlock || acceptErr == modules.ErrBlockKnown {
				acceptErr = nil
			}
			if acceptErr != nil {
				return acceptErr
			}
		}
	}
	return nil
}

// sendBlocks is the receiving end of the SendBlocks RPC. It returns a
// sequential set of blocks based on the 32 input block IDs. The most recent
// known ID is used as the starting point, and up to 'MaxCatchUpBlocks' from
// that BlockHeight onwards are returned. It also sends a boolean indicating
// whether more blocks are available.
func (cs *ConsensusSet) sendBlocks(conn modules.PeerConn) error {
	// Read a list of blocks known to the requester and find the most recent
	// block from the current path.
	var knownBlocks [32]types.BlockID
	err := encoding.ReadObject(conn, &knownBlocks, 32*crypto.HashSize)
	if err != nil {
		return err
	}

	// Find the most recent block from knownBlocks in the current path.
	found := false
	var start types.BlockHeight
	var csHeight types.BlockHeight
	cs.mu.RLock()
	err = cs.db.View(func(tx *bolt.Tx) error {
		csHeight = blockHeight(tx)
		for _, id := range knownBlocks {
			pb, err := getBlockMap(tx, id)
			if err != nil {
				continue
			}
			pathID, err := getPath(tx, pb.Height)
			if err != nil {
				continue
			}
			if pathID != pb.Block.ID() {
				continue
			}
			if pb.Height == csHeight {
				break
			}
			found = true
			// Start from the child of the common block.
			start = pb.Height + 1
		}
		return nil
	})
	cs.mu.RUnlock()
	if err != nil {
		return err
	}

	// If no matching blocks are found, or if the caller has all known blocks,
	// don't send any blocks.
	if !found {
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
		cs.mu.RLock()
		cs.db.View(func(tx *bolt.Tx) error {
			height := blockHeight(tx)
			for i := start; i <= height && i < start+MaxCatchUpBlocks; i++ {
				id, err := getPath(tx, i)
				if build.DEBUG && err != nil {
					panic(err)
				}
				pb, err := getBlockMap(tx, id)
				if build.DEBUG && err != nil {
					panic(err)
				}
				blocks = append(blocks, pb.Block)
			}
			moreAvailable = start+MaxCatchUpBlocks < height
			start += MaxCatchUpBlocks
			return nil
		})
		cs.mu.RUnlock()
		if err != nil {
			return err
		}

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

// relayBlock is an RPC that accepts a block from a peer.
func (cs *ConsensusSet) relayBlock(conn modules.PeerConn) error {
	// Decode the block from the connection.
	var b types.Block
	err := encoding.ReadObject(conn, &b, types.BlockSizeLimit)
	if err != nil {
		return err
	}

	// Submit the block to the consensus set.
	err = cs.AcceptBlock(b)
	if err == errOrphan {
		// If the block is an orphan, try to find the parents. The block
		// received from the peer is discarded and will be downloaded again if
		// the parent is found.
		go cs.gateway.RPC(modules.NetAddress(conn.RemoteAddr().String()), "SendBlocks", cs.threadedReceiveBlocks)
	}
	if err != nil {
		return err
	}
	return nil
}
