package gateway

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/NebulousLabs/Sia/build"
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
	listener net.Listener
	myAddr   modules.NetAddress

	// handlers are the RPCs that the Gateway can handle.
	handlers map[rpcID]modules.RPCFunc
	// initRPCs are the RPCs that the Gateway calls upon connecting to a peer.
	initRPCs map[string]modules.RPCFunc

	// peers are the nodes we are currently connected to.
	peers map[modules.NetAddress]*peer

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

// New returns an initialized Gateway.
func New(addr string, saveDir string) (g *Gateway, err error) {
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
		handlers: make(map[rpcID]modules.RPCFunc),
		initRPCs: make(map[string]modules.RPCFunc),
		peers:    make(map[modules.NetAddress]*peer),
		nodes:    make(map[modules.NetAddress]struct{}),
		saveDir:  saveDir,
		mu:       sync.New(modules.SafeMutexDelay, 2),
		log:      logger,
	}

	// Register RPCs
	g.RegisterRPC("ShareNodes", g.shareNodes)
	g.RegisterRPC("RelayNode", g.relayNode)
	g.RegisterConnectCall("ShareNodes", g.requestNodes)
	g.RegisterConnectCall("RelayNode", g.sendAddress)

	g.log.Println("INFO: gateway created, started logging")

	// Create the listener.
	g.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}
	// Set myAddr (this is necessary if addr == ":0", in which case the OS
	// will assign us a random open port).
	g.myAddr = modules.NetAddress(g.listener.Addr().String())

	// Discover our external IP. (During testing, return the loopback address.)
	var hostname string
	if build.Release == "testing" {
		hostname = "::1"
	} else {
		hostname, err = getExternalIP()
		if err != nil {
			return nil, err
		}
	}
	g.myAddr = modules.NetAddress(net.JoinHostPort(hostname, g.myAddr.Port()))

	g.log.Println("INFO: our address is", g.myAddr)

	// Add ourselves as a node.
	g.addNode(g.myAddr)

	// Spawn the primary listener.
	go g.listen()

	// Load the old peer list. If it doesn't exist, no problem, but if it does,
	// we want to know about any errors preventing us from loading it.
	if loadErr := g.load(); loadErr != nil && !os.IsNotExist(loadErr) {
		return nil, loadErr
	}

	// Spawn the connector loop. This will continually attempt to add nodes as
	// peers to ensure we stay well-connected.
	go g.makeOutboundConnections()

	return
}

// getExternalIP learns the server's hostname from a centralized service,
// myexternalip.com.
func getExternalIP() (string, error) {
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

// enforce that Gateway satisfies the modules.Gateway interface
var _ modules.Gateway = (*Gateway)(nil)
