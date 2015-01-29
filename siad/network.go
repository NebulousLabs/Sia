package main

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

const (
	MaxCatchUpBlocks = 100
)

var (
	moreBlocksErr = errors.New("more blocks are available")
)

// initializeNetwork registers the rpcs and bootstraps to the network,
// downlading all of the blocks and establishing a peer list.
func (d *daemon) initializeNetwork(addr string, nobootstrap bool) (err error) {
	d.network, err = network.NewTCPServer(addr)
	if err != nil {
		return
	}

	err = d.network.RegisterRPC("AcceptBlock", d.state.AcceptBlock)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("AcceptTransaction", d.tpool.AcceptTransaction)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("SendBlocks", d.SendBlocks)
	if err != nil {
		return
	}
	/*
		d.network.RegisterRPC("NegotiateContract", d.host.NegotiateContract)
		if err != nil {
			return
		}
		d.network.RegisterRPC("RetrieveFile", d.host.RetrieveFile)
		if err != nil {
			return
		}
	*/

	// If we aren't bootstrapping, then we're done.
	// TODO: this means the CatchUp thread isn't spawned.
	// It should probably be spawned after the first peer connects.
	if nobootstrap {
		return
	}

	// Bootstrapping may take a while
	go func() {
		// Establish an initial peer list.
		if err := d.network.Bootstrap(); err != nil {
			// log error
			return
		}

		// Every 2 minutes, call CatchUp() on a random peer. This helps with
		// synchronization.
		for ; ; time.Sleep(time.Minute * 2) {
			peer, err := d.network.RandomPeer()
			if err != nil {
				continue
			}
			go d.CatchUp(peer)
		}
	}()

	return
}

// blockHistory returns up to 32 BlockIDs, starting with the 12 most recent
// BlockIDs and then doubling in step size until the genesis block is reached.
// The genesis block is always included. This array of BlockIDs is used to
// establish a shared commonality between peers during synchronization.
func (d *daemon) blockHistory() (blockIDs [32]consensus.BlockID) {
	knownBlocks := make([]consensus.BlockID, 0, 32)
	step := consensus.BlockHeight(1)
	for height := d.state.Height(); ; height -= step {
		block, exists := d.state.BlockAtHeight(height)
		if !exists {
			// faulty state; log high-priority error
			return
		}
		knownBlocks = append(knownBlocks, block.ID())

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
	genesis, _ := d.state.BlockAtHeight(0)
	knownBlocks = append(knownBlocks, genesis.ID())

	copy(blockIDs[:], knownBlocks)
	return
}

// SendBlocks takes a list of block ids as input, and sends all blocks from
func (d *daemon) SendBlocks(knownBlocks [32]consensus.BlockID) (blocks []consensus.Block, err error) {
	// Find the most recent block from knownBlocks that is in our current path.
	found := false
	var start consensus.BlockHeight
	for _, id := range knownBlocks {
		height, exists := d.state.HeightOfBlock(id)
		if !exists {
			found = true
			start = height + 1 // start at child
			break
		}
	}
	if !found {
		// The genesis block should be included in knownBlocks - if no matching
		// blocks are found the caller is probably on a different blockchain
		// altogether.
		err = errors.New("no matching block found")
		return
	}

	// Send blocks, starting with the child of the most recent known block.
	for i := start; i < start+MaxCatchUpBlocks; i++ {
		b, exists := d.state.BlockAtHeight(i)
		if !exists {
			break
		}
		blocks = append(blocks, b)
	}

	// If more blocks are available, send a benign error
	if _, exists := d.state.BlockAtHeight(start + MaxCatchUpBlocks); !exists {
		err = moreBlocksErr
	}

	return
}

// CatchUp synchronizes with a peer to acquire any missing blocks. The
// requester sends 32 blocks, starting with the 12 most recent and then
// progressing exponentially backwards to the genesis block. The receiver uses
// these blocks to find the most recent block seen by both peers, and then
// transmits blocks sequentially until the requester is fully synchronized.
func (d *daemon) CatchUp(peer network.Address) {
	var newBlocks []consensus.Block
	err := peer.RPC("SendBlocks", d.blockHistory(), &newBlocks)
	if err != nil && err.Error() != moreBlocksErr.Error() {
		// log error
		// TODO: try a different peer?
		return
	}
	for _, block := range newBlocks {
		acceptErr := d.state.AcceptBlock(block)
		if acceptErr != nil {
			// TODO: something
			//
			// TODO: If the error is a FutureBlockErr, need to wait before trying the
			// block again.
		}
	}

	// TODO: There is probably a better approach than to call CatchUp
	// recursively. Furthermore, if there is a reorg that's greater than 100
	// blocks, CatchUp is going to fail outright.
	if err != nil && err.Error() == moreBlocksErr.Error() {
		go d.CatchUp(peer)
	}
}
