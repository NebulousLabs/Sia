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
	tcps  *network.TCPServer
	state *consensus.State
	tpool modules.TransactionPool

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
// RelayBlock is called by Miners.
func (g *Gateway) RelayBlock(b consensus.Block) (err error) {
	err = g.state.AcceptBlock(b)
	if err != nil {
		return
	}
	g.broadcast("RelayBlock", b, nil)
	return
}

// RelayTransaction relays a transaction, both locally and to the network.
// RelayTransaction is called by Wallets.
func (g *Gateway) RelayTransaction(t consensus.Transaction) (err error) {
	err = g.tpool.AcceptTransaction(t)
	if err != nil {
		return
	}
	g.broadcast("RelayTransaction", t, nil)
	return
}

// SharePeers returns up to 10 randomly selected peers.
func (g *Gateway) SharePeers() (peers []network.Address, err error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for peer := range g.peers {
		if len(peers) > 10 {
			return
		}
		peers = append(peers, peer)
	}
	return
}

// AddPeer is an RPC that requests that the Gateway add a peer to its peer
// list. The supplied peer is assumed to be the peer making the RPC.
func (g *Gateway) AddMe(peer network.Address) error {
	if !network.Ping(peer) {
		return ErrUnreachable
	}
	g.AddPeer(peer)
	return nil
}

// AddPeer adds a peer to the Gateway's peer list.
func (g *Gateway) AddPeer(peer network.Address) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.addPeer(peer)
}

// RemovePeer removes a peer from the Gateway's peer list.
func (g *Gateway) RemovePeer(peer network.Address) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.removePeer(peer)
}

// Info returns metadata about the Gateway.
func (g *Gateway) Info() (info modules.GatewayInfo) {
	for peer := range g.peers {
		info.Peers = append(info.Peers, peer)
	}
	return
}

// New returns an initialized Gateway.
func New(tcps *network.TCPServer, s *consensus.State, tp modules.TransactionPool) *Gateway {
	return &Gateway{
		tcps:  tcps,
		state: s,
		tpool: tp,
		peers: make(map[network.Address]struct{}),
	}
}
