// Package gateway connects a Sia node to the Sia flood network. The flood
// network is used to propagate blocks and transactions. The gateway is the
// primary avenue that a node uses to hear about transactions and blocks, and
// is the primary avenue used to tell the network about blocks that you have
// mined or about transactions that you have created.
package gateway

import (
	"time"
)

// For the user to be securely connected to the network, the user must be
// connected to at least one node which will send them all of the blocks. An
// attacker can trick the user into thinking that a different blockchain is the
// full blockchain if the user is not connected to any nodes who are seeing +
// broadcasting the real chain (and instead is connected only to attacker nodes
// or to nodes that are not broadcasting). This situation is called an eclipse
// attack.
//
// Connecting to a large number of nodes increases the resiliancy of the
// network, but also puts a networking burden on the nodes and can slow down
// block propagation or increase orphan rates. The gateway's job is to keep the
// network efficient while also protecting the user against attacks.
//
// The gateway keeps a list of nodes that it knows about. It uses this list to
// form connections with other nodes, and then uses those connections to
// participate in the flood network. The primary vector for an attacker to
// achieve an eclipse attack is node list domination. If a gateway's nodelist
// is heavily dominated by attacking nodes, then when the gateway chooses to
// make random connections the gateway is at risk of selecting only attacker
// nodes.
//
// The gateway defends itself from these attacks by minimizing the amount of
// control that an attacker has over the node list and peer list. The first
// major defense is that the gateway maintains 8 'outbound' relationships,
// which means that the gateway created those relationships instead of an
// attacker. If a node forms a connection to you, that node is called
// 'inbound', and because it may be an attacker node, it is not trusted.
// Outbound nodes can also be attacker nodes, but they are less likely to be
// attacker nodes because you chose them, instead of them choosing you.
//
// If the gateway forms too many connections, the gateway will allow incoming
// connections by kicking an existing peer. But, to limit the amount of control
// that an attacker may have, only inbound peers are selected to be kicked.
// Furthermore, to increase the difficulty of attack, if a new inbound
// connection shares the same IP address as an existing connection, the shared
// connection is the connection that gets dropped (unless that connection is a
// local or outbound connection).
//
// Nodes are added to a peerlist in two methods. The first method is that a
// gateway will ask its outbound peers for a list of nodes. If the node list is
// below a certain size (see consts.go), the gateway will repeatedly ask
// outbound peers to expand the list. Nodes are also added to the nodelist
// after they successfully form a connection with the gateway. To limit the
// attacker's ability to add nodes to the nodelist, connections are
// ratelimited. An attacker with lots of IP addresses still has the ability to
// fill up the nodelist, however getting 90% dominance of the nodelist requires
// forming thousands of connections, which will take hours or days. By that
// time, the attacked node should already have its set of outbound peers,
// limiting the amount of damage that the attacker can do.
//
// To limit DNS-based tomfoolry, nodes are only added to the nodelist if their
// connection information takes the form of an IP address.
//
// Some research has been done on Bitcoin's flood networks. The more relevant
// research has been listed below. The papers listed first are more relevant.
//     Eclipse Attacks on Bitcoin's Peer-to-Peer Network (Heilman, Kendler, Zohar, Goldberg)
//     Stubborn Mining: Generalizing Selfish Mining and Combining with an Eclipse Attack (Nayak, Kumar, Miller, Shi)
//     An Overview of BGP Hijacking (https://www.bishopfox.com/blog/2015/08/an-overview-of-bgp-hijacking/)

// TODO: Currently the gateway does not do much in terms of bucketing. The
// gateway should make sure that it has outbound peers from a wide range of IP
// addresses, and when kicking inbound peers it shouldn't just favor kicking
// peers of the same IP address, it should favor kicking peers of the same ip
// address range.
//
// TODO: When peers connect to eachother, and when they add nodes to the node
// list, there is no verification that the peers are running on the same Sia
// network, something that will be problematic if we set up a large testnet.
// It's already problematic, and currently only solved by using a different set
// of bootstrap nodes. If there is any cross-polination (which an attacker
// could do pretty easily), the gateways will not clean up over time, which
// will degrade the quality of the flood network as the two networks will
// continuously flood eachother with irrelevant information. Additionally, there
// is no public key exhcange, so communications cannot be effectively encrypted
// or authenticated. The nodes must have some way to share keys.
//
// TODO: Gateway hostname discovery currently has significant centralization,
// namely the fallback is a single third-party website that can easily form any
// response it wants. Instead, multiple TLS-protected third party websites
// should be used, and the plurality answer should be accepted as the true
// hostname.
//
// TODO: The gateway currently does hostname discovery in a non-blocking way,
// which means that the first few peers that it connects to may not get the
// correct hostname. This means that you may give the remote peer the wrong
// hostname, which means they will not be able to dial you back, which means
// they will not add you to their node list.
//
// TODO: The gateway should encrypt and authenticate all communications. Though
// the gateway participates in a flood network, practical attacks have been
// demonstrated which have been able to confuse nodes by manipulating messages
// from their peers. Encryption + authentication would have made the attack
// more difficult.
//
// TODO: The gateway does an unofficial ping in two places within nodes.go.
// These unofficial pings can be found by searching for the string "0.0.0". To
// perform the unofficial ping, the gateway connects to a peer and writes its
// version as 0.0.0. When the connection completes and the handshake passes,
// the gateway writes its version as '0.0.0', causing the other node to reject
// the connection and disconnect. The gateway then can tell that the node on
// the other end of the line is a Sia gateway, without having to add that node
// as a peer. A better solution to this hack is to just add an RPC for the ping
// function. Rollout will of course have to be gradual, as new nodes and old
// nodes need to be able to successfully ping eachother until a sufficient
// portion of the network has upgraded to the new code.
//
// TODO: The gateway, when connecting to a peer, currently has no way of
// recognizing whether that peer is itself or not. Because of this, the gateway
// does occasionally try to connect to itself, especially if it has a really
// small node list. This can cause problems, especially during testing, if the
// gateway is intentionally limiting the number of local outbount peers that it
// has. For that reason, the 'permanentPeerManager' will allow the gateway to
// add local peers as outbound peers all the way up to 3 total connections. The
// gateway needs a proper way to recognize if it is trying to connect itself
// (sending a nonce when performing the version handshake, for example). Once
// that has been implemented, maxLocalOutobundPeers can be reduced to 2 or 1
// (probably 2).

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
	nodes  map[modules.NetAddress]*node
	peers  map[modules.NetAddress]*peer
	peerTG siasync.ThreadGroup

	// Utilities.
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	threads    siasync.ThreadGroup
}

// managedSleep will sleep for the given period of time. If the full time
// elapses, 'false' is returned. If the sleep is interrupted for shutdown,
// 'true' is returned.
func (g *Gateway) managedSleep(t time.Duration) (completed bool) {
	select {
	case <-time.After(t):
		return true
	case <-g.threads.StopChan():
		return false
	}
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
		nodes: make(map[modules.NetAddress]*node),

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
	g.log.Println("INFO: gateway created, started logging")

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
		const inbound = true
		for _, addr := range modules.BootstrapPeers {
			err := g.addNode(addr, inbound)
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

	return g, nil
}

// enforce that Gateway satisfies the modules.Gateway interface
var _ modules.Gateway = (*Gateway)(nil)
