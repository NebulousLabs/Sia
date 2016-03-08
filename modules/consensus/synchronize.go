package consensus

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

const (
	// minNumOutbound is the minimum number of outbound peers required before ibd
	// is confident we are synced.
	minNumOutbound = 4
)

var (
	// MaxCatchUpBlocks is the maxiumum number of blocks that can be given to
	// the consensus set in a single iteration during the initial blockchain
	// download.
	MaxCatchUpBlocks = func() types.BlockHeight {
		switch build.Release {
		case "dev":
			return 10
		case "standard":
			return 10
		case "testing":
			return 3
		default:
			panic("unrecognized build.Release")
		}
	}()
	// sendBlocksTimeout is the timeout for the SendBlocks RPC.
	sendBlocksTimeout = func() time.Duration {
		switch build.Release {
		case "dev":
			return 5 * time.Minute
		case "standard":
			return 5 * time.Minute
		case "testing":
			return 5 * time.Second
		default:
			panic("unrecognized build.Release")
		}
	}()

	errSendBlocksStalled = errors.New("SendBlocks RPC timed and never received any blocks")
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
		if height <= step {
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
	// Set a deadline after which SendBlocks will timeout. During IBD, esepcially,
	// SendBlocks will timeout. This is by design so that IBD switches peers to
	// prevent any one peer from stalling IBD.
	err := conn.SetDeadline(time.Now().Add(sendBlocksTimeout))
	// Pipes do not support Set{,Read,Write}Deadline and should only be used in
	// testing.
	if build.Release != "testing" {
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "set" && opErr.Net == "pipe" {
			return err
		}
	}

	// Get blockIDs to send.
	var history [32]types.BlockID
	err = cs.db.View(func(tx *bolt.Tx) error {
		history = blockHistory(tx)
		return nil
	})
	if err != nil {
		return err
	}

	// Send the block ids.
	if err := encoding.WriteObject(conn, history); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return errSendBlocksStalled
		}
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
			// Broadcast the block to all peers <= v0.5.1 and block header to all peers > v0.5.1
			var relayBlockPeers, relayHeaderPeers []modules.Peer
			for _, p := range cs.gateway.Peers() {
				if build.VersionCmp(p.Version, "0.5.1") <= 0 {
					relayBlockPeers = append(relayBlockPeers, p)
				} else {
					relayHeaderPeers = append(relayHeaderPeers, p)
				}
			}
			go cs.gateway.Broadcast("RelayBlock", currentBlock, relayBlockPeers)
			go cs.gateway.Broadcast("RelayHeader", currentBlock.Header(), relayHeaderPeers)
		}
	}()

	// Read blocks off of the wire and add them to the consensus set until
	// there are no more blocks available.
	moreAvailable := true
	receivedBlocks := false
	for moreAvailable {
		// Read a slice of blocks from the wire.
		var newBlocks []types.Block
		if err := encoding.ReadObject(conn, &newBlocks, uint64(MaxCatchUpBlocks)*types.BlockSizeLimit); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && !receivedBlocks {
				return errSendBlocksStalled
			}
			return err
		}
		receivedBlocks = true
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

// rpcSendBlocks is the receiving end of the SendBlocks RPC. It returns a
// sequential set of blocks based on the 32 input block IDs. The most recent
// known ID is used as the starting point, and up to 'MaxCatchUpBlocks' from
// that BlockHeight onwards are returned. It also sends a boolean indicating
// whether more blocks are available.
func (cs *ConsensusSet) rpcSendBlocks(conn modules.PeerConn) error {
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
			break
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
			moreAvailable = start+MaxCatchUpBlocks <= height
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

// rpcRelayBlock is an RPC that accepts a block from a peer.
func (cs *ConsensusSet) rpcRelayBlock(conn modules.PeerConn) error {
	// Decode the block from the connection.
	var b types.Block
	err := encoding.ReadObject(conn, &b, types.BlockSizeLimit)
	if err != nil {
		return err
	}

	// Submit the block to the consensus set and broadcast it.
	err = cs.AcceptBlock(b)
	if err == errOrphan {
		// If the block is an orphan, try to find the parents. The block
		// received from the peer is discarded and will be downloaded again if
		// the parent is found.
		//
		// TODO: log error returned if non-nill.
		go cs.gateway.RPC(modules.NetAddress(conn.RemoteAddr().String()), "SendBlocks", cs.threadedReceiveBlocks)
	}
	if err != nil {
		return err
	}
	return nil
}

// rpcRelayHeader is an RPC that accepts a block header from a peer.
func (cs *ConsensusSet) rpcRelayHeader(conn modules.PeerConn) error {
	// Decode the block header from the connection.
	var h types.BlockHeader
	err := encoding.ReadObject(conn, &h, types.BlockHeaderSize)
	if err != nil {
		return err
	}

	// Start verification inside of a bolt View tx.
	cs.mu.RLock()
	err = cs.db.View(func(tx *bolt.Tx) error {
		// Do some relatively inexpensive checks to validate the header
		return cs.validateHeader(boltTxWrapper{tx}, h)
	})
	cs.mu.RUnlock()
	if err == errOrphan {
		// If the header is an orphan, try to find the parents.
		//
		// TODO: log error returned if non-nill.
		go cs.gateway.RPC(modules.NetAddress(conn.RemoteAddr().String()), "SendBlocks", cs.threadedReceiveBlocks)
		return nil
	} else if err != nil {
		return err
	}
	// If the header is valid and extends the heaviest chain, fetch, accept it,
	// and broadcast it.
	//
	// TODO: log error returned if non-nill.
	go cs.gateway.RPC(modules.NetAddress(conn.RemoteAddr().String()), "SendBlk", cs.threadedReceiveBlock(h.ID()))
	return nil
}

// rpcSendBlk is an RPC that sends the requested block to the requesting peer.
func (cs *ConsensusSet) rpcSendBlk(conn modules.PeerConn) error {
	// Decode the block id from the conneciton.
	var id types.BlockID
	err := encoding.ReadObject(conn, &id, crypto.HashSize)
	if err != nil {
		return err
	}
	// Lookup the corresponding block.
	var b types.Block
	err = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return err
		}
		b = pb.Block
		return nil
	})
	if err != nil {
		return err
	}
	// Encode and send the block to the caller.
	err = encoding.WriteObject(conn, b)
	if err != nil {
		return err
	}
	return nil
}

// threadedReceiveBlock takes a block id and returns an RPCFunc that requests
// that block and then calls AcceptBlock on it. The returned function should be
// used as the calling end of the SendBlk RPC. Note that although the function
// itself does not do any locking, it is still prefixed with "threaded" because
// the function it returns calls the exported method AcceptBlock.
func (cs *ConsensusSet) threadedReceiveBlock(id types.BlockID) modules.RPCFunc {
	managedFN := func(conn modules.PeerConn) error {
		if err := encoding.WriteObject(conn, id); err != nil {
			return err
		}
		var block types.Block
		if err := encoding.ReadObject(conn, &block, types.BlockSizeLimit); err != nil {
			return err
		}
		if err := cs.AcceptBlock(block); err != nil {
			return err
		}
		return nil
	}
	return managedFN
}

// threadedInitialBlockchainDownload performs the IBD on outbound peers. Blocks
// are downloaded from one peer at a time in 5 minute intervals, so as to
// prevent any one peer from significantly slowing down IBD.
//
// NOTE: IBD will succeed right now when each peer has a different blockchain.
// The height and the block id of the remote peers' current blocks are not
// checked to be the same. This can cause issues if you are connected to
// outbound peers <= v0.5.1 that are stalled in IBD.
func (cs *ConsensusSet) threadedInitialBlockchainDownload() {
	for {
		numOutbound := 0
		numOutboundSynced := 0
		// Cache gateway.Peers() so we can compare peers to gateway.Peers() after
		// this for loop, to see if any additional outbound peers were added while we
		// were syncing.
		peers := cs.gateway.Peers()
		for _, p := range peers {
			// We only sync on outbound peers at first to make IBD less susceptible to
			// fast-mining and other attacks, as outbound peers are more difficult to
			// manipulate.
			if p.Inbound {
				continue
			}
			numOutbound++

			err := cs.gateway.RPC(p.NetAddress, "SendBlocks", cs.threadedReceiveBlocks)
			if err == nil {
				numOutboundSynced++
				continue
			}
			if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
				// TODO: log the error returned by RPC.

				// Disconnect if there is an unexpected error (not a timeout). This
				// includes errSendBlocksStalled.
				//
				// We disconnect so that these peers are removed from gateway.Peers() and
				// do not prevent us from marking ourselves as fully synced.
				cs.gateway.Disconnect(p.NetAddress)
				// TODO: log error returned by Disconnect
			}
		}

		// Count the number of outbound peers. If we connected to more outbound peers
		// during the previous SendBlocks, we want to sync with the new peers too.
		newNumOutbound := 0
		for _, p := range cs.gateway.Peers() {
			if !p.Inbound {
				newNumOutbound++
			}
		}

		if numOutbound >= minNumOutbound && // Need a minimum number of outbound peers to call ourselves synced.
			numOutbound >= newNumOutbound && // If we got more outbound peers while we were syncing, sync with them too.
			((numOutbound == modules.WellConnectedThreshold && numOutboundSynced > (2/3*modules.WellConnectedThreshold)) ||
				(numOutbound != modules.WellConnectedThreshold && numOutboundSynced == numOutbound)) { // Super majority.
			break
		}
	}

	// TODO: log IBD done.
}
