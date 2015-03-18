package gateway

import (
	"errors"
	"net"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
)

const (
	// maxStrikes is the number of "strikes" that can be incurred by a peer
	// before it will be removed.
	// TODO: need a way to whitelist peers (e.g. hosts)
	maxStrikes = 5
)

var (
	errNoPeers     = errors.New("no peers")
	errUnreachable = errors.New("peer did not respond to ping")
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

	// saveDir is the path used to save/load peers.
	saveDir string

	mu *sync.RWMutex
}

// Address returns the NetAddress of the Gateway.
func (g *Gateway) Address() modules.NetAddress {
	counter := g.mu.RLock()
	defer g.mu.RUnlock(counter)
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
		return errUnreachable
	}
	counter := g.mu.Lock()
	g.addPeer(bootstrapPeer)
	g.mu.Unlock(counter)

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

	// request peers from the bootstrap
	g.requestPeers(bootstrapPeer)
	// save new peers
	g.save()

	// announce ourselves to the new peers
	go g.threadedBroadcast("AddMe", writerRPC(g.myAddr))

	// spawn synchronizer
	go g.threadedResynchronize()

	return
}

// RelayBlock relays a block to the network.
func (g *Gateway) RelayBlock(b consensus.Block) {
	go g.threadedBroadcast("AcceptBlock", writerRPC(b))
}

// RelayTransaction relays a transaction to the network.
func (g *Gateway) RelayTransaction(t consensus.Transaction) {
	go g.threadedBroadcast("AcceptTransaction", writerRPC(t))
}

// Info returns metadata about the Gateway.
func (g *Gateway) Info() (info modules.GatewayInfo) {
	counter := g.mu.RLock()
	defer g.mu.RUnlock(counter)
	info.Address = g.myAddr
	for peer := range g.peers {
		info.Peers = append(info.Peers, peer)
	}
	return
}

// New returns an initialized Gateway.
func New(addr string, s *consensus.State, saveDir string) (g *Gateway, err error) {
	if s == nil {
		err = errors.New("gateway cannot use nil state")
		return
	}

	g = &Gateway{
		state:      s,
		myAddr:     modules.NetAddress(addr),
		handlerMap: make(map[rpcID]modules.RPCFunc),
		peers:      make(map[modules.NetAddress]int),
		saveDir:    saveDir,
		mu:         sync.New(time.Second*1, 0),
	}

	// Create the directory if it doesn't exist.
	err = os.MkdirAll(saveDir, 0700)
	if err != nil {
		return
	}

	g.RegisterRPC("Ping", writerRPC(pong))
	g.RegisterRPC("SendHostname", sendHostname)
	g.RegisterRPC("AddMe", g.addMe)
	g.RegisterRPC("SharePeers", g.sharePeers)
	g.RegisterRPC("SendBlocks", g.sendBlocks)

	// spawn RPC handler
	err = g.startListener(addr)
	if err != nil {
		return
	}

	return
}
