package gateway

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
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

	// peers are the nodes we are currently connected to.
	peers map[modules.NetAddress]*Peer

	// nodes is a list of all known nodes (i.e. potential peers) on the
	// network.
	// TODO: map to a timestamp?
	nodes map[modules.NetAddress]struct{}

	// saveDir is the path used to save/load peers.
	saveDir string

	log *log.Logger

	mu *sync.RWMutex
}

// Address returns the NetAddress of the Gateway.
func (g *Gateway) Address() modules.NetAddress {
	id := g.mu.RLock()
	defer g.mu.RUnlock(id)
	return g.myAddr
}

// Close stops the Gateway's listener process.
func (g *Gateway) Close() error {
	return g.listener.Close()
}

// Bootstrap joins the Sia network and establishes an initial peer list.
func (g *Gateway) Bootstrap(addr modules.NetAddress) error {
	g.log.Println("INFO: initiated bootstrapping to", addr)

	// contact the bootstrap peer
	bootstrap, err := g.connect(addr)
	if err != nil {
		return err
	}

	// initial peer discovery
	go g.requestNodes(bootstrap)

	// spawn synchronizer
	go g.threadedResynchronize()

	g.log.Printf("INFO: successfully bootstrapped to %v (this does not mean you are synchronized)", addr)

	return nil
}

// RelayBlock relays a block to the network.
func (g *Gateway) RelayBlock(b types.Block) {
	go g.threadedBroadcast("AcceptBlock", writerRPC(b))
}

// RelayTransaction relays a transaction to the network.
func (g *Gateway) RelayTransaction(t types.Transaction) {
	go g.threadedBroadcast("AcceptTransaction", writerRPC(t))
}

// Info returns metadata about the Gateway.
func (g *Gateway) Info() (info modules.GatewayInfo) {
	id := g.mu.RLock()
	defer g.mu.RUnlock(id)
	info.Address = g.myAddr
	for peer := range g.peers {
		info.Peers = append(info.Peers, peer)
	}
	info.Nodes = len(g.nodes)
	return
}

// New returns an initialized Gateway.
func New(addr string, s *consensus.State, saveDir string) (g *Gateway, err error) {
	if s == nil {
		err = errors.New("gateway cannot use nil state")
		return
	}

	// Create the directory if it doesn't exist.
	err = os.MkdirAll(saveDir, 0700)
	if err != nil {
		return
	}

	// Create the logger.
	logger, err := makeLogger(saveDir)
	if err != nil {
		return
	}

	g = &Gateway{
		state:      s,
		handlerMap: make(map[rpcID]modules.RPCFunc),
		peers:      make(map[modules.NetAddress]*Peer),
		nodes:      make(map[modules.NetAddress]struct{}),
		saveDir:    saveDir,
		mu:         sync.New(time.Second*1, 0),
		log:        logger,
	}

	g.RegisterRPC("ShareNodes", g.shareNodes)
	g.RegisterRPC("SendBlocks", g.sendBlocks)

	g.log.Println("INFO: gateway created, started logging")

	// Create the listener.
	g.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}
	// Set myAddr (this is necessary if addr == ":0", in which case the OS
	// will assign us a random open port).
	g.myAddr = modules.NetAddress(g.listener.Addr().String())

	// Discover external IP
	hostname, err := g.getExternalIP()
	if err != nil {
		return nil, err
	}
	g.myAddr = modules.NetAddress(net.JoinHostPort(hostname, g.myAddr.Port()))

	g.log.Println("INFO: our address is", g.myAddr)

	// Add ourselves as a node.
	g.addNode(g.myAddr)

	// Spawn the primary listener.
	go g.listen()

	// Load the old peer list. If it doesn't exist, no problem, but if it does,
	// we want to know about any errors preventing us from loading it.
	if loadErr := g.load(); err != nil && !os.IsNotExist(loadErr) {
		return nil, loadErr
	}

	return
}

// getExternalIP learns the server's hostname from a centralized service,
// myexternalip.com.
func (g *Gateway) getExternalIP() (string, error) {
	// during testing, return the loopback address
	if build.Release == "testing" {
		return "::1", nil
	}

	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	hostname := string(buf[:n-1]) // trim newline
	return hostname, nil
}
