package gateway

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

var (
	ErrNoPeers     = errors.New("no peers")
	ErrUnreachable = errors.New("peer did not respond to ping")
)

// Gateway implements the modules.Gateway interface.
type Gateway struct {
	tcps        *network.TCPServer
	state       *consensus.State
	latestBlock consensus.BlockID

	peers map[network.Address]struct{}

	mu sync.RWMutex
}

// Bootstrap joins the Sia network and establishes an initial peer list.
func (g *Gateway) Bootstrap(bootstrapPeer network.Address) (err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// contact the bootstrap peer
	if !network.Ping(bootstrapPeer) {
		return ErrUnreachable
	}
	g.addPeer(bootstrapPeer)

	// request peers
	// TODO: maybe iterate until we have enough new peers?
	var newPeers []network.Address
	err = bootstrapPeer.RPC("SharePeers", nil, &newPeers)
	if err != nil {
		return
	}
	for _, peer := range newPeers {
		if peer != g.tcps.Address() && network.Ping(peer) {
			g.addPeer(peer)
		}
	}

	// announce ourselves to new peers
	g.broadcast("AddMe", g.tcps.Address(), nil)

	return
}

// RelayBlock relays a block, both locally and to the network.
func (g *Gateway) RelayBlock(b consensus.Block) (err error) {
	err = g.state.AcceptBlock(b)
	if err != nil {
		return
	}

	// Check if b is in the current path.
	height, exists := g.state.HeightOfBlock(b.ID())
	if !exists {
		if consensus.DEBUG {
			panic("could not get the height of a block that did not return an error when being accepted into the state.")
		}
		return errors.New("state malfunction")
	}
	currentPathBlock, exists := g.state.BlockAtHeight(height)
	if !exists || b.ID() != currentPathBlock.ID() {
		return errors.New("block added, but it does not extend the state height.")
	}

	g.broadcast("RelayBlock", b, nil)
	return
}

// RelayTransaction relays a transaction, both locally and to the network.
func (g *Gateway) RelayTransaction(t consensus.Transaction) (err error) {
	g.broadcast("AcceptTransaction", t, nil)
	return
}

// Info returns metadata about the Gateway.
func (g *Gateway) Info() (info modules.GatewayInfo) {
	for peer := range g.peers {
		info.Peers = append(info.Peers, peer)
	}
	return
}

// New returns an initialized Gateway.
func New(tcps *network.TCPServer, s *consensus.State) (g *Gateway, err error) {
	if tcps == nil {
		err = errors.New("gateway cannot use nil tcp server")
		return
	}
	if s == nil {
		err = errors.New("gateway cannot use nil state")
		return
	}

	g = &Gateway{
		tcps:  tcps,
		state: s,
		peers: make(map[network.Address]struct{}),
	}
	block, exists := g.state.BlockAtHeight(0)
	if !exists {
		err = errors.New("gateway state is missing the genesis block")
		return
	}
	g.latestBlock = block.ID()

	return
}
