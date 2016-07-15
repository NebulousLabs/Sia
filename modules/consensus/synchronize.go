package consensus

import (
	"errors"
	"net"
	"sync"
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
	minNumOutbound = 5
)

var (
	// MaxCatchUpBlocks is the maxiumum number of blocks that can be given to
	// the consensus set in a single iteration during the initial blockchain
	// download.
	MaxCatchUpBlocks = func() types.BlockHeight {
		switch build.Release {
		case "dev":
			return 50
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
			return 40 * time.Second
		case "standard":
			return 5 * time.Minute
		case "testing":
			return 5 * time.Second
		default:
			panic("unrecognized build.Release")
		}
	}()
	// minIBDWaitTime is the time threadedInitialBlockchainDownload waits before
	// exiting if there are >= 1 and <= minNumOutbound peers synced. This timeout
	// will primarily affect miners who have multiple nodes daisy chained off each
	// other. Those nodes will likely have to wait minIBDWaitTime on every startup
	// before IBD is done.
	minIBDWaitTime = func() time.Duration {
		switch build.Release {
		case "dev":
			return 80 * time.Second
		case "standard":
			return 90 * time.Minute
		case "testing":
			return 10 * time.Second
		default:
			panic("unrecognized build.Release")
		}
	}()
	// ibdLoopDelay is the time that threadedInitialBlockchainDownload waits
	// between attempts to synchronize with the network if the last attempt
	// failed.
	ibdLoopDelay = func() time.Duration {
		switch build.Release {
		case "dev":
			return 1 * time.Second
		case "standard":
			return 10 * time.Second
		case "testing":
			return 100 * time.Millisecond
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
func (cs *ConsensusSet) threadedReceiveBlocks(conn modules.PeerConn) (returnErr error) {
	// Set a deadline after which SendBlocks will timeout. During IBD, esepcially,
	// SendBlocks will timeout. This is by design so that IBD switches peers to
	// prevent any one peer from stalling IBD.
	err := conn.SetDeadline(time.Now().Add(sendBlocksTimeout))
	// Ignore errors returned by SetDeadline if the conn is a pipe in testing.
	// Pipes do not support Set{,Read,Write}Deadline and should only be used in
	// testing.
	if opErr, ok := err.(*net.OpError); ok && opErr.Op == "set" && opErr.Net == "pipe" && build.Release == "testing" {
		err = nil
	}
	if err != nil {
		return err
	}
	stalled := true
	defer func() {
		// TODO: Timeout errors returned by muxado do not conform to the net.Error
		// interface and therefore we cannot check if the error is a timeout using
		// the Timeout() method. Once muxado issue #14 is resolved change the below
		// condition to:
		//     if netErr, ok := returnErr.(net.Error); ok && netErr.Timeout() && stalled { ... }
		if stalled && returnErr != nil && (returnErr.Error() == "Read timeout" || returnErr.Error() == "Write timeout") {
			returnErr = errSendBlocksStalled
		}
	}()

	// Get blockIDs to send.
	var history [32]types.BlockID
	cs.mu.RLock()
	err = cs.db.View(func(tx *bolt.Tx) error {
		history = blockHistory(tx)
		return nil
	})
	cs.mu.RUnlock()
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
		if chainExtended && cs.Synced() {
			// The last block received will be the current block since
			// managedAcceptBlock only returns nil if a block extends the longest chain.
			currentBlock := cs.CurrentBlock()
			// COMPATv0.5.1 - broadcast the block to all peers <= v0.5.1 and block header to all peers > v0.5.1
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
			stalled = false
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
		err = cs.db.View(func(tx *bolt.Tx) error {
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
// COMPATv0.5.1
func (cs *ConsensusSet) rpcRelayBlock(conn modules.PeerConn) error {
	// Decode the block from the connection.
	var b types.Block
	err := encoding.ReadObject(conn, &b, types.BlockSizeLimit)
	if err != nil {
		return err
	}

	// Submit the block to the consensus set and broadcast it.
	err = cs.managedAcceptBlock(b)
	if err == errOrphan {
		// If the block is an orphan, try to find the parents. The block
		// received from the peer is discarded and will be downloaded again if
		// the parent is found.
		go func() {
			err := cs.gateway.RPC(conn.RPCAddr(), "SendBlocks", cs.threadedReceiveBlocks)
			if err != nil {
				cs.log.Debugln("WARN: failed to get parents of orphan block:", err)
			}
		}()
	}
	if err != nil {
		return err
	}
	cs.managedBroadcastBlock(b)
	return nil
}

// threadedRPCRelayHeader is an RPC that accepts a block header from a peer.
func (cs *ConsensusSet) threadedRPCRelayHeader(conn modules.PeerConn) error {
	err := cs.tg.Add()
	if err != nil {
		return err
	}
	wg := new(sync.WaitGroup)
	defer func() {
		go func() {
			wg.Wait()
			cs.tg.Done()
		}()
	}()

	// Decode the block header from the connection.
	var h types.BlockHeader
	err = encoding.ReadObject(conn, &h, types.BlockHeaderSize)
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
		go func() {
			err := cs.gateway.RPC(conn.RPCAddr(), "SendBlocks", cs.threadedReceiveBlocks)
			if err != nil {
				cs.log.Debugln("WARN: failed to get parents of orphan header:", err)
			}
		}()
		return nil
	} else if err != nil {
		return err
	}

	// If the header is valid and extends the heaviest chain, fetch the
	// corresponding block. Call needs to be made in a separate goroutine
	// because an exported call to the gateway is used, which is a deadlock
	// risk given that rpcRelayHeader is called from the gateway.
	wg.Add(1)
	go func() {
		err = cs.gateway.RPC(conn.RPCAddr(), "SendBlk", cs.threadedReceiveBlock(h.ID()))
		if err != nil {
			cs.log.Debugln("WARN: failed to get header's corresponding block:", err)
		}
		wg.Done()
	}()
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
	cs.mu.RLock()
	err = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return err
		}
		b = pb.Block
		return nil
	})
	cs.mu.RUnlock()
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

// threadedReceiveBlock takes a block id and returns an RPCFunc that requests that
// block and then calls AcceptBlock on it. The returned function should be used
// as the calling end of the SendBlk RPC. Note that although the function
// itself does not do any locking, it is still prefixed with "threaded" because
// the function it returns calls the exported method AcceptBlock.
func (cs *ConsensusSet) threadedReceiveBlock(id types.BlockID) modules.RPCFunc {
	return func(conn modules.PeerConn) error {
		if err := encoding.WriteObject(conn, id); err != nil {
			return err
		}
		var block types.Block
		if err := encoding.ReadObject(conn, &block, types.BlockSizeLimit); err != nil {
			return err
		}
		if err := cs.managedAcceptBlock(block); err != nil {
			return err
		}
		cs.managedBroadcastBlock(block)
		return nil
	}
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
	// Set the deadline 10 minutes in the future. After this deadline, we will say
	// IBD is done as long as there is at least one outbound peer synced.
	deadline := time.Now().Add(minIBDWaitTime)
	numOutboundSynced := 0
	for {
		numOutboundSynced = 0
		for _, p := range cs.gateway.Peers() {
			// We only sync on outbound peers at first to make IBD less susceptible to
			// fast-mining and other attacks, as outbound peers are more difficult to
			// manipulate.
			if p.Inbound {
				continue
			}

			err := cs.gateway.RPC(p.NetAddress, "SendBlocks", cs.threadedReceiveBlocks)
			if err == nil {
				numOutboundSynced++
				continue
			}
			// TODO: Timeout errors returned by muxado do not conform to the net.Error
			// interface and therefore we cannot check if the error is a timeout using
			// the Timeout() method. Once muxado issue #14 is resolved change the below
			// condition to:
			//     if netErr, ok := returnErr.(net.Error); !ok || !netErr.Timeout() { ... }
			if err.Error() != "Read timeout" && err.Error() != "Write timeout" {
				cs.log.Printf("WARN: disconnecting from peer %v because IBD failed: %v", p.NetAddress, err)
				// Disconnect if there is an unexpected error (not a timeout). This
				// includes errSendBlocksStalled.
				//
				// We disconnect so that these peers are removed from gateway.Peers() and
				// do not prevent us from marking ourselves as fully synced.
				err := cs.gateway.Disconnect(p.NetAddress)
				if err != nil {
					cs.log.Printf("WARN: disconnecting from peer %v failed: %v", p.NetAddress, err)
				}
			}
		}

		// If we have minNumOutbound peers synced, we are done. Otherwise, don't say
		// we are synced until we've been doing ibd for 10 minutes and we are synced
		// with at least one peer.
		if numOutboundSynced >= minNumOutbound || (numOutboundSynced > 0 && time.Now().After(deadline)) {
			break
		} else {
			// Sleep so we don't hammer the network with SendBlock requests.
			time.Sleep(ibdLoopDelay)
		}
	}

	cs.log.Printf("INFO: IBD done, synced with %v peers", numOutboundSynced)
}

// Synced returns true if the consensus set is synced with the network.
func (cs *ConsensusSet) Synced() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.synced
}
