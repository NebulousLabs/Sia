package gateway

import (
	"errors"
	"net"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	// maxStrikes is the number of "strikes" that can be incurred by a peer
	// before it will be removed.
	// TODO: need a way to whitelist peers (e.g. hosts)
	maxStrikes = 5
)

var (
	// TODO: unexport these, or move them to modules
	ErrNoPeers     = errors.New("no peers")
	ErrUnreachable = errors.New("peer did not respond to ping")
)

// Gateway implements the modules.Gateway interface.
type Gateway struct {
	state *consensus.State

	listener net.Listener
	myAddr   modules.NetAddress

	// Each incoming connection begins with a string of 8 bytes, indicating
	// which function should handle the connection.
	handlerMap map[rpcID]modules.RPCFunc

	// Peers are stored in a map to guarantee uniqueness. They are paired with
	// the number of "strikes" against them; peers with too many strikes are
	// removed.
	peers map[modules.NetAddress]int

	mu sync.RWMutex
}

// Address returns the NetAddress of the Gateway.
func (g *Gateway) Address() modules.NetAddress {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.myAddr
}

// Close stops the Gateway's listener process.
func (g *Gateway) Close() error {
	return g.listener.Close()
}

// Bootstrap joins the Sia network and establishes an initial peer list.
//
// Bootstrap handles mutexes manually to avoid having a lock during network
// communication.
func (g *Gateway) Bootstrap(bootstrapPeer modules.NetAddress) (err error) {
	// contact the bootstrap peer
	if !g.Ping(bootstrapPeer) {
		return ErrUnreachable
	}
	g.mu.Lock()
	g.addPeer(bootstrapPeer)
	g.mu.Unlock()

	// ask the bootstrap peer for our hostname
	err = g.learnHostname(bootstrapPeer)
	if err != nil {
		err = g.getExternalIP()
		if err != nil {
			return
		}
	}
	if !g.Ping(g.myAddr) {
		return errors.New("couldn't learn hostname")
	}

	go func() {
		// request peers from the bootstrap
		g.requestPeers(bootstrapPeer)
		// announce ourselves to the new peers
		g.threadedBroadcast("AddMe", modules.WriterRPC(g.myAddr))
		// synchronize to a random peer
		g.Synchronize()
	}()

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
			panic("could not get the height of a block that did not return an error when being accepted into the state")
		}
		return errors.New("state malfunction")
	}
	currentPathBlock, exists := g.state.BlockAtHeight(height)
	if !exists || b.ID() != currentPathBlock.ID() {
		return errors.New("block added, but it does not extend the state height")
	}

	go g.threadedBroadcast("RelayBlock", modules.WriterRPC(b))
	return
}

// RelayTransaction relays a transaction, both locally and to the network.
func (g *Gateway) RelayTransaction(t consensus.Transaction) (err error) {
	// no locking necessary
	go g.threadedBroadcast("AcceptTransaction", modules.WriterRPC(t))
	return
}

// Info returns metadata about the Gateway.
func (g *Gateway) Info() (info modules.GatewayInfo) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	info.Address = g.myAddr
	for peer := range g.peers {
		info.Peers = append(info.Peers, peer)
	}
	return
}

// New returns an initialized Gateway.
func New(addr string, s *consensus.State) (g *Gateway, err error) {
	if s == nil {
		err = errors.New("gateway cannot use nil state")
		return
	}

	// create listener
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}

	g = &Gateway{
		state:      s,
		listener:   l,
		myAddr:     modules.NetAddress(addr),
		handlerMap: make(map[rpcID]modules.RPCFunc),
		peers:      make(map[modules.NetAddress]int),
	}

	g.RegisterRPC("Ping", modules.WriterRPC(pong))
	g.RegisterRPC("SendHostname", sendHostname)
	g.RegisterRPC("AddMe", g.addMe)
	g.RegisterRPC("SharePeers", g.sharePeers)
	g.RegisterRPC("SendBlocks", g.sendBlocks)

	// spawn RPC handler
	go g.listen()

	return
}
