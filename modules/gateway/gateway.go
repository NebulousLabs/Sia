package gateway

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
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
	port     string

	// handlers are the RPCs that the Gateway can handle.
	handlers map[rpcID]modules.RPCFunc
	// initRPCs are the RPCs that the Gateway calls upon connecting to a peer.
	initRPCs map[string]modules.RPCFunc

	// peers are the nodes we are currently connected to.
	peers map[modules.NetAddress]*peer

	// nodes is the set of all known nodes (i.e. potential peers) on the
	// network.
	nodes map[modules.NetAddress]struct{}

	// closeChan is used to shut down the Gateway's goroutines.
	closeChan chan struct{}

	persistDir string

	log *persist.Logger
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
	var errs []error

	// Unregister RPCs. Not strictly necessary for clean shutdown in this specific
	// case, as the handlers should only contain references to the gateway itself,
	// but do it anyways as an example for other modules to follow.
	g.UnregisterRPC("ShareNodes")
	g.UnregisterConnectCall("ShareNodes")
	// save the latest gateway state
	id := g.mu.RLock()
	if err := g.saveSync(); err != nil {
		errs = append(errs, fmt.Errorf("save failed: %v", err))
	}
	g.mu.RUnlock(id)
	// send close signal
	close(g.closeChan)
	// clear the port mapping (no effect if UPnP not supported)
	id = g.mu.RLock()
	g.clearPort(g.myAddr.Port())
	g.mu.RUnlock(id)
	// shut down the listener
	if err := g.listener.Close(); err != nil {
		errs = append(errs, fmt.Errorf("listener.Close failed: %v", err))
	}
	// Disconnect from peers.
	for _, p := range g.Peers() {
		if err := g.Disconnect(p.NetAddress); err != nil {
			errs = append(errs, fmt.Errorf("Disconnect failed: %v", err))
		}
	}
	// Sleep to give time for all goroutines to exit. This is necessary because
	// some goroutines write to the logger so we must give them time to exit
	// before closing the logger.
	// TODO: block until goroutines exit instead of sleeping.
	time.Sleep(100 * time.Millisecond)
	// Close the logger. The logger should be the last thing to shut down so that
	// all other objects have access to logging while closing.
	if err := g.log.Close(); err != nil {
		errs = append(errs, fmt.Errorf("log.Close failed: %v", err))
	}

	return build.JoinErrors(errs, "; ")
}

// New returns an initialized Gateway.
func New(addr string, persistDir string) (g *Gateway, err error) {
	// Create the directory if it doesn't exist.
	err = os.MkdirAll(persistDir, 0700)
	if err != nil {
		return
	}

	g = &Gateway{
		handlers:   make(map[rpcID]modules.RPCFunc),
		initRPCs:   make(map[string]modules.RPCFunc),
		peers:      make(map[modules.NetAddress]*peer),
		nodes:      make(map[modules.NetAddress]struct{}),
		closeChan:  make(chan struct{}),
		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 2),
	}

	// Create the logger.
	g.log, err = persist.NewFileLogger(filepath.Join(g.persistDir, logFile))
	if err != nil {
		return nil, err
	}

	// Register RPCs.
	g.RegisterRPC("ShareNodes", g.shareNodes)
	g.RegisterConnectCall("ShareNodes", g.requestNodes)

	// Load the old node list. If it doesn't exist, no problem, but if it does,
	// we want to know about any errors preventing us from loading it.
	if loadErr := g.load(); loadErr != nil && !os.IsNotExist(loadErr) {
		return nil, loadErr
	}

	// Add the bootstrap peers to the node list.
	if build.Release == "standard" {
		for _, addr := range modules.BootstrapPeers {
			err := g.addNode(addr)
			if err != nil && err != errNodeExists {
				g.log.Printf("WARN: failed to add the bootstrap node '%v': %v", addr, err)
			}
		}
		g.save()
	}

	// Create listener and set address.
	g.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}
	_, g.port, err = net.SplitHostPort(g.listener.Addr().String())
	if err != nil {
		return nil, err
	}
	if build.Release == "testing" {
		g.myAddr = modules.NetAddress(g.listener.Addr().String())
	}

	g.log.Println("INFO: gateway created, started logging")

	// Forward the RPC port, if possible.
	go g.forwardPort(g.port)

	// Learn our external IP.
	go g.learnHostname()

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
