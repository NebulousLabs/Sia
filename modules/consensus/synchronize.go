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

	"github.com/coreos/bbolt"
)

const (
	// minNumOutbound is the minimum number of outbound peers required before ibd
	// is confident we are synced.
	minNumOutbound = 5
)

var (
	errEarlyStop         = errors.New("initial blockchain download did not complete by the time shutdown was issued")
	errNilProcBlock      = errors.New("nil processed block was fetched from the database")
	errSendBlocksStalled = errors.New("SendBlocks RPC timed and never received any blocks")

	// ibdLoopDelay is the time that threadedInitialBlockchainDownload waits
	// between attempts to synchronize with the network if the last attempt
	// failed.
	ibdLoopDelay = build.Select(build.Var{
		Standard: 10 * time.Second,
		Dev:      1 * time.Second,
		Testing:  100 * time.Millisecond,
	}).(time.Duration)

	// MaxCatchUpBlocks is the maxiumum number of blocks that can be given to
	// the consensus set in a single iteration during the initial blockchain
	// download.
	MaxCatchUpBlocks = build.Select(build.Var{
		Standard: types.BlockHeight(10),
		Dev:      types.BlockHeight(50),
		Testing:  types.BlockHeight(3),
	}).(types.BlockHeight)

	// minIBDWaitTime is the time threadedInitialBlockchainDownload waits before
	// exiting if there are >= 1 and <= minNumOutbound peers synced. This timeout
	// will primarily affect miners who have multiple nodes daisy chained off each
	// other. Those nodes will likely have to wait minIBDWaitTime on every startup
	// before IBD is done.
	minIBDWaitTime = build.Select(build.Var{
		Standard: 90 * time.Minute,
		Dev:      80 * time.Second,
		Testing:  10 * time.Second,
	}).(time.Duration)

	// relayHeaderTimeout is the timeout for the RelayHeader RPC.
	relayHeaderTimeout = build.Select(build.Var{
		Standard: 60 * time.Second,
		Dev:      20 * time.Second,
		Testing:  3 * time.Second,
	}).(time.Duration)

	// sendBlkTimeout is the timeout for the SendBlk RPC.
	sendBlkTimeout = build.Select(build.Var{
		Standard: 90 * time.Second,
		Dev:      30 * time.Second,
		Testing:  4 * time.Second,
	}).(time.Duration)

	// sendBlocksTimeout is the timeout for the SendBlocks RPC.
	sendBlocksTimeout = build.Select(build.Var{
		Standard: 180 * time.Second,
		Dev:      40 * time.Second,
		Testing:  5 * time.Second,
	}).(time.Duration)
)

// isTimeoutErr is a helper function that returns true if err was caused by a
// network timeout.
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	// COMPATv1.3.0
	return (err.Error() == "Read timeout" || err.Error() == "Write timeout")
}

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

// managedReceiveBlocks is the calling end of the SendBlocks RPC, without the
// threadgroup wrapping.
func (cs *ConsensusSet) managedReceiveBlocks(conn modules.PeerConn) (returnErr error) {
	// Set a deadline after which SendBlocks will timeout. During IBD, especially,
	// SendBlocks will timeout. This is by design so that IBD switches peers to
	// prevent any one peer from stalling IBD.
	err := conn.SetDeadline(time.Now().Add(sendBlocksTimeout))
	if err != nil {
		return err
	}
	finishedChan := make(chan struct{})
	defer close(finishedChan)
	go func() {
		select {
		case <-cs.tg.StopChan():
		case <-finishedChan:
		}
		conn.Close()
	}()

	// Check whether this RPC has timed out with the remote peer at the end of
	// the fuction, and if so, return a custom error to signal that a new peer
	// needs to be chosen.
	stalled := true
	defer func() {
		if isTimeoutErr(returnErr) && stalled {
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
	var initialBlock types.BlockID
	if build.DEBUG {
		// Prepare for a sanity check on 'chainExtended' - chain extended should
		// be set to true if an ony if the result of calling dbCurrentBlockID
		// changes.
		initialBlock = cs.dbCurrentBlockID()
	}
	chainExtended := false
	defer func() {
		cs.mu.RLock()
		synced := cs.synced
		cs.mu.RUnlock()
		if synced && chainExtended {
			if build.DEBUG && initialBlock == cs.dbCurrentBlockID() {
				panic("blockchain extension reporting is incorrect")
			}
			fullBlock := cs.managedCurrentBlock() // TODO: Add cacheing, replace this line by looking at the cache.
			go cs.gateway.Broadcast("RelayHeader", fullBlock.Header(), cs.gateway.Peers())
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
		if len(newBlocks) == 0 {
			continue
		}
		stalled = false

		// Call managedAcceptBlock instead of AcceptBlock so as not to broadcast
		// every block.
		extended, acceptErr := cs.managedAcceptBlocks(newBlocks)
		if extended {
			chainExtended = true
		}
		// ErrNonExtendingBlock must be ignored until headers-first block
		// sharing is implemented, block already in database should also be
		// ignored.
		if acceptErr != nil && acceptErr != modules.ErrNonExtendingBlock && acceptErr != modules.ErrBlockKnown {
			return acceptErr
		}
	}
	return nil
}

// threadedReceiveBlocks is the calling end of the SendBlocks RPC.
func (cs *ConsensusSet) threadedReceiveBlocks(conn modules.PeerConn) error {
	err := conn.SetDeadline(time.Now().Add(sendBlocksTimeout))
	if err != nil {
		return err
	}
	finishedChan := make(chan struct{})
	defer close(finishedChan)
	go func() {
		select {
		case <-cs.tg.StopChan():
		case <-finishedChan:
		}
		conn.Close()
	}()
	err = cs.tg.Add()
	if err != nil {
		return err
	}
	defer cs.tg.Done()
	return cs.managedReceiveBlocks(conn)
}

// rpcSendBlocks is the receiving end of the SendBlocks RPC. It returns a
// sequential set of blocks based on the 32 input block IDs. The most recent
// known ID is used as the starting point, and up to 'MaxCatchUpBlocks' from
// that BlockHeight onwards are returned. It also sends a boolean indicating
// whether more blocks are available.
func (cs *ConsensusSet) rpcSendBlocks(conn modules.PeerConn) error {
	err := conn.SetDeadline(time.Now().Add(sendBlocksTimeout))
	if err != nil {
		return err
	}
	finishedChan := make(chan struct{})
	defer close(finishedChan)
	go func() {
		select {
		case <-cs.tg.StopChan():
		case <-finishedChan:
		}
		conn.Close()
	}()
	err = cs.tg.Add()
	if err != nil {
		return err
	}
	defer cs.tg.Done()

	// Read a list of blocks known to the requester and find the most recent
	// block from the current path.
	var knownBlocks [32]types.BlockID
	err = encoding.ReadObject(conn, &knownBlocks, 32*crypto.HashSize)
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
				if err != nil {
					cs.log.Critical("Unable to get path: height", height, ":: request", i)
					return err
				}
				pb, err := getBlockMap(tx, id)
				if err != nil {
					cs.log.Critical("Unable to get block from block map: height", height, ":: request", i, ":: id", id)
					return err
				}
				if pb == nil {
					cs.log.Critical("getBlockMap yielded 'nil' block:", height, ":: request", i, ":: id", id)
					return errNilProcBlock
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

// threadedRPCRelayHeader is an RPC that accepts a block header from a peer.
func (cs *ConsensusSet) threadedRPCRelayHeader(conn modules.PeerConn) error {
	err := conn.SetDeadline(time.Now().Add(relayHeaderTimeout))
	if err != nil {
		return err
	}
	finishedChan := make(chan struct{})
	defer close(finishedChan)
	go func() {
		select {
		case <-cs.tg.StopChan():
		case <-finishedChan:
		}
		conn.Close()
	}()
	err = cs.tg.Add()
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
	// WARN: orphan multithreading logic (dangerous areas, see below)
	//
	// If the header is valid and extends the heaviest chain, fetch the
	// corresponding block. Call needs to be made in a separate goroutine
	// because an exported call to the gateway is used, which is a deadlock
	// risk given that rpcRelayHeader is called from the gateway.
	//
	// NOTE: In general this is bad design. Rather than recycling other
	// calls, the whole protocol should have been kept in a single RPC.
	// Because it is not, we have to do weird threading to prevent
	// deadlocks, and we also have to be concerned every time the code in
	// managedReceiveBlock is adjusted.
	if err == errOrphan { // WARN: orphan multithreading logic case #1
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cs.gateway.RPC(conn.RPCAddr(), "SendBlocks", cs.managedReceiveBlocks)
			if err != nil {
				cs.log.Debugln("WARN: failed to get parents of orphan header:", err)
			}
		}()
		return nil
	} else if err != nil {
		return err
	}

	// WARN: orphan multithreading logic case #2
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = cs.gateway.RPC(conn.RPCAddr(), "SendBlk", cs.managedReceiveBlock(h.ID()))
		if err != nil {
			cs.log.Debugln("WARN: failed to get header's corresponding block:", err)
		}
	}()
	return nil
}

// rpcSendBlk is an RPC that sends the requested block to the requesting peer.
func (cs *ConsensusSet) rpcSendBlk(conn modules.PeerConn) error {
	err := conn.SetDeadline(time.Now().Add(sendBlkTimeout))
	if err != nil {
		return err
	}
	finishedChan := make(chan struct{})
	defer close(finishedChan)
	go func() {
		select {
		case <-cs.tg.StopChan():
		case <-finishedChan:
		}
		conn.Close()
	}()
	err = cs.tg.Add()
	if err != nil {
		return err
	}
	defer cs.tg.Done()

	// Decode the block id from the connection.
	var id types.BlockID
	err = encoding.ReadObject(conn, &id, crypto.HashSize)
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

// managedReceiveBlock takes a block id and returns an RPCFunc that requests that
// block and then calls AcceptBlock on it. The returned function should be used
// as the calling end of the SendBlk RPC.
func (cs *ConsensusSet) managedReceiveBlock(id types.BlockID) modules.RPCFunc {
	return func(conn modules.PeerConn) error {
		if err := encoding.WriteObject(conn, id); err != nil {
			return err
		}
		var block types.Block
		if err := encoding.ReadObject(conn, &block, types.BlockSizeLimit); err != nil {
			return err
		}
		chainExtended, err := cs.managedAcceptBlocks([]types.Block{block})
		if chainExtended {
			cs.managedBroadcastBlock(block)
		}
		if err != nil {
			return err
		}
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
func (cs *ConsensusSet) threadedInitialBlockchainDownload() error {
	// The consensus set will not recognize IBD as complete until it has enough
	// peers. After the deadline though, it will recognize the blockchain
	// download as complete even with only one peer. This deadline is helpful
	// to local-net setups, where a machine will frequently only have one peer
	// (and that peer will be another machine on the same local network, but
	// within the local network at least one peer is connected to the braod
	// network).
	deadline := time.Now().Add(minIBDWaitTime)
	numOutboundSynced := 0
	numOutboundNotSynced := 0
	for {
		numOutboundSynced = 0
		numOutboundNotSynced = 0
		for _, p := range cs.gateway.Peers() {
			// We only sync on outbound peers at first to make IBD less susceptible to
			// fast-mining and other attacks, as outbound peers are more difficult to
			// manipulate.
			if p.Inbound {
				continue
			}

			// Put the rest of the iteration inside of a thread group.
			err := func() error {
				err := cs.tg.Add()
				if err != nil {
					return err
				}
				defer cs.tg.Done()

				// Request blocks from the peer. The error returned will only be
				// 'nil' if there are no more blocks to receive.
				err = cs.gateway.RPC(p.NetAddress, "SendBlocks", cs.managedReceiveBlocks)
				if err == nil {
					numOutboundSynced++
					// In this case, 'return nil' is equivalent to skipping to
					// the next iteration of the loop.
					return nil
				}
				numOutboundNotSynced++
				if !isTimeoutErr(err) {
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
				return nil
			}()
			if err != nil {
				return err
			}
		}

		// The consensus set is not considered synced until a majority of
		// outbound peers say that we are synced. If less than 10 minutes have
		// passed, a minimum of 'minNumOutbound' peers must say that we are
		// synced, otherwise a 1 vs 0 majority is sufficient.
		//
		// This scheme is used to prevent malicious peers from being able to
		// barricade the sync'd status of the consensus set, and to make sure
		// that consensus sets behind a firewall with only one peer
		// (potentially a local peer) are still able to eventually conclude
		// that they have syncrhonized. Miners and hosts will often have setups
		// beind a firewall where there is a single node with many peers and
		// then the rest of the nodes only have a few peers.
		if numOutboundSynced > numOutboundNotSynced && (numOutboundSynced >= minNumOutbound || time.Now().After(deadline)) {
			break
		} else {
			// Sleep so we don't hammer the network with SendBlock requests.
			time.Sleep(ibdLoopDelay)
		}
	}

	cs.log.Printf("INFO: IBD done, synced with %v peers", numOutboundSynced)
	return nil
}

// Synced returns true if the consensus set is synced with the network.
func (cs *ConsensusSet) Synced() bool {
	err := cs.tg.Add()
	if err != nil {
		return false
	}
	defer cs.tg.Done()
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.synced
}
