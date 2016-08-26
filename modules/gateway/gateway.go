package gateway

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
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
	//
	// initRPCs are the RPCs that the Gateway calls upon connecting to a peer.
	handlers map[rpcID]modules.RPCFunc
	initRPCs map[string]modules.RPCFunc

	// nodes is the set of all known nodes (i.e. potential peers).
	//
	// peers are the nodes that the gateway is currently connected to.
	//
	// peerTG is a special thread group for tracking peer connections, and will
	// block shutdown until all peer connections have been closed out. The peer
	// connections are put in a separate TG because of their unique
	// requirements - they have the potential to live for the lifetime of the
	// program, but also the potential to close early. Calling threads.OnStop
	// for each peer could create a huge backlog of functions that do nothing
	// (because most of the peers disconnected prior to shutdown). And they
	// can't call threads.Add because they are potentially very long running
	// and would block any threads.Flush() calls. So a second threadgroup is
	// added which handles clean-shutdown for the peers, without blocking
	// threads.Flush() calls.
	nodes  map[modules.NetAddress]struct{}
	peers  map[modules.NetAddress]*peer
	peerTG siasync.ThreadGroup

	// Utilities.
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	threads    siasync.ThreadGroup
}

// Address returns the NetAddress of the Gateway.
func (g *Gateway) Address() modules.NetAddress {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.myAddr
}

// Close saves the state of the Gateway and stops its listener process.
func (g *Gateway) Close() error {
	if err := g.threads.Stop(); err != nil {
		return err
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.saveSync()
}

// New returns an initialized Gateway.
func New(addr string, bootstrap bool, persistDir string) (*Gateway, error) {
	// Create the directory if it doesn't exist.
	err := os.MkdirAll(persistDir, 0700)
	if err != nil {
		return nil, err
	}

	g := &Gateway{
		handlers: make(map[rpcID]modules.RPCFunc),
		initRPCs: make(map[string]modules.RPCFunc),

		peers: make(map[modules.NetAddress]*peer),
		nodes: make(map[modules.NetAddress]struct{}),

		persistDir: persistDir,
	}

	// Create the logger.
	g.log, err = persist.NewFileLogger(filepath.Join(g.persistDir, logFile))
	if err != nil {
		return nil, err
	}
	// Establish the closing of the logger.
	g.threads.AfterStop(func() {
		if err := g.log.Close(); err != nil {
			// The logger may or may not be working here, so use a println
			// instead.
			fmt.Println("Failed to close the gateway logger:", err)
		}
	})

	// Establish that the peerTG must complete shutdown before the primary
	// thread group completes shutdown.
	g.threads.OnStop(func() {
		err = g.peerTG.Stop()
		if err != nil {
			g.log.Println("ERROR: peerTG experienced errors while shutting down:", err)
		}
	})

	// Register RPCs.
	g.RegisterRPC("ShareNodes", g.shareNodes)
	g.RegisterConnectCall("ShareNodes", g.requestNodes)
	// Establish the de-registration of the RPCs.
	g.threads.OnStop(func() {
		g.UnregisterRPC("ShareNodes")
		g.UnregisterConnectCall("ShareNodes")
	})

	// Load the old node list. If it doesn't exist, no problem, but if it does,
	// we want to know about any errors preventing us from loading it.
	if loadErr := g.load(); loadErr != nil && !os.IsNotExist(loadErr) {
		return nil, loadErr
	}

	// Add the bootstrap peers to the node list.
	if bootstrap {
		for _, addr := range modules.BootstrapPeers {
			err := g.addNode(addr)
			if err != nil && err != errNodeExists {
				g.log.Printf("WARN: failed to add the bootstrap node '%v': %v", addr, err)
			}
		}
	}

	// Create the listener which will listen for new connections from peers.
	permanentListenClosedChan := make(chan struct{})
	g.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	// Automatically close the listener when g.threads.Stop() is called.
	g.threads.OnStop(func() {
		err := g.listener.Close()
		if err != nil {
			g.log.Println("WARN: closing the listener failed:", err)
		}
		<-permanentListenClosedChan
	})
	// Set the address and port of the gateway.
	_, g.port, err = net.SplitHostPort(g.listener.Addr().String())
	if err != nil {
		return nil, err
	}
	// Set myAddr equal to the address returned by the listener. It will be
	// overwritten by threadedLearnHostname later on.
	g.myAddr = modules.NetAddress(g.listener.Addr().String())

	// Spawn the peer connection listener.
	go g.permanentListen(permanentListenClosedChan)

	// Spawn the peer manager and provide tools for ensuring clean shutdown.
	peerManagerClosedChan := make(chan struct{})
	g.threads.OnStop(func() {
		<-peerManagerClosedChan
	})
	go g.permanentPeerManager(peerManagerClosedChan)

	// Spawn the node manager and provide tools for ensuring clean shudown.
	nodeManagerClosedChan := make(chan struct{})
	g.threads.OnStop(func() {
		<-nodeManagerClosedChan
	})
	go g.permanentNodeManager(nodeManagerClosedChan)

	// Spawn the node purger and provide tools for ensuring clean shutdown.
	nodePurgerClosedChan := make(chan struct{})
	g.threads.OnStop(func() {
		<-nodePurgerClosedChan
	})
	go g.permanentNodePurger(nodePurgerClosedChan)

	// Spawn threads to take care of port forwarding and hostname discovery.
	go g.threadedForwardPort(g.port)
	go g.threadedLearnHostname()

	g.log.Println("INFO: gateway created, started logging")
	return g, nil
}

// enforce that Gateway satisfies the modules.Gateway interface
var _ modules.Gateway = (*Gateway)(nil)
