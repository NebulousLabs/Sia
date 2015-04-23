package gateway

import (
	"net"
	"time"

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
func (g *Gateway) threadedResynchronize() {
	for {
		go func() {
			// max 10 attempts
			for i := 0; i < 10; i++ {
				peer, err := g.randomPeer()
				if err != nil {
					g.log.Println("ERR: no peers are available for synchronization")
					return
				}
				// keep looping until a successful Synchronize
				if g.Synchronize(peer) == nil {
					return
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
func (g *Gateway) Synchronize(peer modules.NetAddress) error {
	g.log.Println("INFO: synchronizing to", peer)
	for {
		var newBlocks []types.Block
		newBlocks, moreAvailable, err := g.requestBlocks(peer)
		if err != nil {
			g.log.Printf("ERR: synchronization to %v failed: %v\n", peer, err)
			return err
		}
		g.log.Printf("INFO: %v sent us %v blocks\n", peer, len(newBlocks))
		for _, block := range newBlocks {
			acceptErr := g.state.AcceptBlock(block)
			if acceptErr != nil {
				// TODO: If the error is a FutureTimestampErr, need to wait before trying the
				// block again.
				g.log.Printf("WARN: state rejected a block from %v: %v\n", peer, acceptErr)
			}
		}

		// loop until there are no more blocks available
		if !moreAvailable {
			break
		}
	}
	g.log.Printf("INFO: synchronization to %v complete\n", peer)
	return nil
}

// sendBlocks returns a sequential set of blocks based on the 32 input block
// IDs. The most recent known ID is used as the starting point, and up to
// 'MaxCatchUpBlocks' from that BlockHeight onwards are returned. It also
// sends a boolean indicating whether more blocks are available.
func (g *Gateway) sendBlocks(conn net.Conn) (err error) {
	// Read known blocks.
	var knownBlocks [32]types.BlockID
	err = encoding.ReadObject(conn, &knownBlocks, 32*crypto.HashSize)
	if err != nil {
		return
	}

	// Find the most recent block from knownBlocks that is in our current path.
	found := false
	var start types.BlockHeight
	for _, id := range knownBlocks {
		if height, exists := g.state.HeightOfBlock(id); exists {
			found = true
			start = height + 1 // start at child
			break
		}
	}
	// If we didn't find any matching blocks, or if we're already
	// synchronized, don't send any blocks. The genesis block should be
	// included in knownBlocks, so if no matching blocks are found, the caller
	// is probably on a different blockchain altogether.
	if !found || start > g.state.Height() {
		// Send 0 blocks.
		err = encoding.WriteObject(conn, []types.Block{})
		if err != nil {
			return
		}
		// Indicate that no more blocks are available.
		return encoding.WriteObject(conn, false)
	}

	// Determine range of blocks to send.
	stop := start + MaxCatchUpBlocks
	if stop > g.state.Height() {
		stop = g.state.Height()
	}
	blocks, err := g.state.BlockRange(start, stop)
	if err != nil {
		return
	}
	g.log.Printf("INFO: %v is at height %v (-%v); sending them %v blocks\n", conn.RemoteAddr(), start, g.state.Height()-start, len(blocks))
	err = encoding.WriteObject(conn, blocks)
	if err != nil {
		return
	}

	// Indicate whether more blocks are available.
	more := g.state.Height() > stop
	return encoding.WriteObject(conn, more)
}

func (g *Gateway) requestBlocks(peer modules.NetAddress) (newBlocks []types.Block, moreAvailable bool, err error) {
	history := g.blockHistory()
	err = g.RPC(peer, "SendBlocks", func(conn net.Conn) error {
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
	return
}

// blockHistory returns up to 32 BlockIDs, starting with the 12 most recent
// BlockIDs and then doubling in step size until the genesis block is reached.
// The genesis block is always included. This array of BlockIDs is used to
// establish a shared commonality between peers during synchronization.
func (g *Gateway) blockHistory() (blockIDs [32]types.BlockID) {
	knownBlocks := make([]types.BlockID, 0, 32)
	step := types.BlockHeight(1)
	for height := g.state.Height(); ; height -= step {
		block, exists := g.state.BlockAtHeight(height)
		if !exists {
			// faulty state; log high-priority error
			g.log.Println("ERR: state is missing a block at height", height)
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
		g.log.Println("ERR: state is missing a genesis block")
		return
	}
	knownBlocks = append(knownBlocks, genesis.ID())

	copy(blockIDs[:], knownBlocks)
	return
}
