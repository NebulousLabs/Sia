package gateway

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	MaxCatchUpBlocks = 50
)

var (
	moreBlocksErr = errors.New("more blocks are available")
)

// Sychronize to synchronize the local consensus set (i.e. the blockchain) with
// the network consensus set.
//
// TODO: don't run two Synchronize threads at the same time
func (g *Gateway) Synchronize() (err error) {
	peer, err := g.randomPeer()
	if err != nil {
		return
	}
	go g.synchronize(peer)
	return
}

// synchronize asks a peer for new blocks. The requester sends 32 block IDs,
// starting with the 12 most recent and then progressing exponentially
// backwards to the genesis block. The receiver uses these blocks to find the
// most recent block seen by both peers. From this starting height, it
// transmits blocks sequentially. Multiple such transmissions may be required
// to fully synchronize.
func (g *Gateway) synchronize(peer modules.NetAddress) {
	var newBlocks []consensus.Block
	newBlocks, err := g.requestBlocks(peer)
	if err != nil && err.Error() != moreBlocksErr.Error() {
		// TODO: try a different peer?
		return
	}
	for _, block := range newBlocks {
		acceptErr := g.state.AcceptBlock(block)
		if acceptErr != nil {
			// TODO: If the error is a FutureTimestampErr, need to wait before trying the
			// block again.
		}
	}

	if err != nil && err.Error() == moreBlocksErr.Error() {
		//fmt.Println("getting more blocks")
		go g.synchronize(peer)
		return
	}
}

// SendBlocks returns a sequential set of blocks based on the 32 input block
// IDs. The most recent known ID is used as the starting point, and up to
// 'MaxCatchUpBlocks' from that BlockHeight onwards are returned. If more
// blocks could be returned, a 'moreBlocksErr' will be returned as well.
func (g *Gateway) SendBlocks(conn modules.NetConn) (err error) {
	// read known blocks
	var knownBlocks [32]consensus.BlockID
	err = conn.ReadObject(&knownBlocks, 32*crypto.HashSize)
	if err != nil {
		return
	}

	// Find the most recent block from knownBlocks that is in our current path.
	found := false
	var start consensus.BlockHeight
	for _, id := range knownBlocks {
		if height, exists := g.state.HeightOfBlock(id); exists {
			found = true
			start = height + 1 // start at child
			break
		}
	}
	if !found {
		// The genesis block should be included in knownBlocks - if no matching
		// blocks are found, the caller is probably on a different blockchain
		// altogether.
		err = errors.New("no matching block found")
		return
	}

	// Send blocks, starting with the child of the most recent known block.
	//
	// TODO: use BlocksSince instead?
	var blocks []consensus.Block
	for i := start; i < start+MaxCatchUpBlocks; i++ {
		b, exists := g.state.BlockAtHeight(i)
		if !exists {
			break
		}
		blocks = append(blocks, b)
	}

	// If more blocks are available, send a benign error
	if _, exists := g.state.BlockAtHeight(start + MaxCatchUpBlocks); exists {
		err = moreBlocksErr
	}

	return
}

func (g *Gateway) requestBlocks(peer modules.NetAddress) (newBlocks []consensus.Block, err error) {
	history := g.blockHistory()
	err = g.RPC(peer, "SendBlocks", func(conn modules.NetConn) error {
		err := conn.WriteObject(history)
		if err != nil {
			return err
		}
		// TODO: read error
		return conn.ReadObject(newBlocks, MaxCatchUpBlocks*consensus.BlockSizeLimit)
	})
	return
}

// blockHistory returns up to 32 BlockIDs, starting with the 12 most recent
// BlockIDs and then doubling in step size until the genesis block is reached.
// The genesis block is always included. This array of BlockIDs is used to
// establish a shared commonality between peers during synchronization.
func (g *Gateway) blockHistory() (blockIDs [32]consensus.BlockID) {
	knownBlocks := make([]consensus.BlockID, 0, 32)
	step := consensus.BlockHeight(1)
	for height := g.state.Height(); ; height -= step {
		block, exists := g.state.BlockAtHeight(height)
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
	genesis, exists := g.state.BlockAtHeight(0)
	if !exists {
		// this should never happen
		return
	}
	knownBlocks = append(knownBlocks, genesis.ID())

	copy(blockIDs[:], knownBlocks)
	return
}
