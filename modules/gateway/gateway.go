package gateway

import (
	"errors"
	"log"
	"net"
	"os"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
)

var (
	errNoPeers     = errors.New("no peers")
	errUnreachable = errors.New("peer did not respond to ping")
)

// Gateway implements the modules.Gateway interface.
type Gateway struct {
	listener net.Listener
	myAddr   modules.NetAddress

	// handlers are the RPCs that the Gateway can handle.
	handlers map[rpcID]modules.RPCFunc
	// initRPCs are the RPCs that the Gateway calls upon connecting to a peer.
	initRPCs map[string]modules.RPCFunc

	// peers are the nodes we are currently connected to.
	peers map[modules.NetAddress]*peer

	// nodes is the set of all known nodes (i.e. potential peers) on the
	// network.
	nodes map[modules.NetAddress]struct{}

	persistDir string

	log *log.Logger
	mu  *sync.RWMutex
}

// Address returns the NetAddress of the Gateway.
func (g *Gateway) Address() modules.NetAddress {
	id := g.mu.RLock()
	defer g.mu.RUnlock(id)
	return g.myAddr
}

// Close saves the state of the Gateway and stops its listener process.
func (g *Gateway) Close() error {
	id := g.mu.RLock()
	// save the latest gateway state
	if err := g.save(); err != nil {
		return err
	}
	g.mu.RUnlock(id)
	// clear the port mapping (no effect if UPnP not supported)
	g.clearPort(g.myAddr.Port())
	// shut down the listener
	return g.listener.Close()
}

// New returns an initialized Gateway.
func New(addr string, persistDir string) (g *Gateway, err error) {
	// Create the directory if it doesn't exist.
	err = os.MkdirAll(persistDir, 0700)
	if err != nil {
		return
	}

	// Create the logger.
	logger, err := makeLogger(persistDir)
	if err != nil {
		return
	}

	g = &Gateway{
		handlers:   make(map[rpcID]modules.RPCFunc),
		initRPCs:   make(map[string]modules.RPCFunc),
		peers:      make(map[modules.NetAddress]*peer),
		nodes:      make(map[modules.NetAddress]struct{}),
		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 2),
		log:        logger,
	}

	// Load the old peer list. If it doesn't exist, no problem, but if it does,
	// we want to know about any errors preventing us from loading it.
	if loadErr := g.load(); loadErr != nil && !os.IsNotExist(loadErr) {
		return nil, loadErr
	}

	// Create listener and set address.
	g.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}
	_, port, _ := net.SplitHostPort(g.listener.Addr().String())
	g.myAddr = modules.NetAddress(net.JoinHostPort("::1", port))

	// Register RPCs.
	g.RegisterRPC("ShareNodes", g.shareNodes)
	g.RegisterRPC("RelayNode", g.relayNode)
	g.RegisterConnectCall("ShareNodes", g.requestNodes)

	g.log.Println("INFO: gateway created, started logging")

	// Learn our external IP.
	go g.learnHostname()

	// Automatically forward the RPC port, if possible.
	go g.forwardPort(port)

	// Spawn the peer and node managers. These will attempt to keep the peer
	// and node lists healthy.
	go g.threadedPeerManager()
	go g.threadedNodeManager()

	// Spawn the primary listener.
	go g.listen()

	return
}

// enforce that Gateway satisfies the modules.Gateway interface
var _ modules.Gateway = (*Gateway)(nil)
