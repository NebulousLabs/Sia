package gateway

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

const (
	// handshakeUpgradeVersion is the version where the gateway handshake RPC
	// was altered to include adiitional information transfer.
	handshakeUpgradeVersion = "1.0.0"

	// maxLocalOutbound is currently set to 3, meaning the gateway will not
	// consider a local node to be an outbound peer if the gateway already has
	// 3 outbound peers. Three is currently needed to handle situations where
	// the gateway is at high risk of connecting to itself (such as a low
	// number of total peers, especially such as in a testing environment).
	// Once the gateway has a proper way to figure out that it's trying to
	// connect to itself, this number can be reduced.
	maxLocalOutboundPeers = 3

	// minAcceptableVersion is the version below which the gateway will refuse to
	// connect to peers and reject connection attempts.
	//
	// Reject peers < v0.4.0 as the previous version is v0.3.3 which is
	// pre-hardfork.
	minAcceptableVersion = "0.4.0"

	// saveFrequency defines how often the gateway saves its persistence.
	saveFrequency = time.Minute * 2
)

var (
	// fastNodePurgeDelay defines the amount of time that is waited between each
	// iteration of the purge loop when the gateway has enough nodes to be
	// needing to purge quickly.
	fastNodePurgeDelay = build.Select(build.Var{
		Standard: 1 * time.Minute,
		Dev:      5 * time.Second,
		Testing:  200 * time.Millisecond,
	}).(time.Duration)

	// healthyNodeListLen defines the number of nodes that the gateway must
	// have in the node list before it will stop asking peers for more nodes.
	healthyNodeListLen = build.Select(build.Var{
		Standard: int(200),
		Dev:      int(30),
		Testing:  int(15),
	}).(int)

	// maxSharedNodes defines the number of nodes that will be shared between
	// peers when they are expanding their node lists.
	maxSharedNodes = build.Select(build.Var{
		Standard: uint64(10),
		Dev:      uint64(5),
		Testing:  uint64(3),
	}).(uint64)

	// nodePurgeDelay defines the amount of time that is waited between each
	// iteration of the node purge loop.
	nodePurgeDelay = build.Select(build.Var{
		Standard: 10 * time.Minute,
		Dev:      20 * time.Second,
		Testing:  500 * time.Millisecond,
	}).(time.Duration)

	// nodeListDelay defines the amount of time that is waited between each
	// iteration of the node list loop.
	nodeListDelay = build.Select(build.Var{
		Standard: 5 * time.Second,
		Dev:      3 * time.Second,
		Testing:  500 * time.Millisecond,
	}).(time.Duration)

	// peerRPCDelay defines the amount of time waited between each RPC accepted
	// from a peer. Without this delay, a peer can force us to spin up thousands
	// of goroutines per second.
	peerRPCDelay = build.Select(build.Var{
		Standard: 3 * time.Second,
		Dev:      1 * time.Second,
		Testing:  20 * time.Millisecond,
	}).(time.Duration)

	// pruneNodeListLen defines the number of nodes that the gateway must have
	// to be pruning nodes from the node list.
	pruneNodeListLen = build.Select(build.Var{
		Standard: int(50),
		Dev:      int(15),
		Testing:  int(10),
	}).(int)

	// quickPruneListLen defines the number of nodes that the gateway must have
	// to be pruning nodes quickly from the node list.
	quickPruneListLen = build.Select(build.Var{
		Standard: int(250),
		Dev:      int(40),
		Testing:  int(20),
	}).(int)
)

var (
	// The gateway will sleep this long between incoming connections. For
	// attack reasons, the acceptInterval should be longer than the
	// nodeListDelay. Right at startup, a node is vulnerable to being flooded
	// by Sybil attackers. The node's best defense is to wait until it has
	// filled out its nodelist somewhat from the bootstrap nodes. An attacker
	// needs to completely dominate the nodelist and the peerlist to be
	// successful, so just a few honest nodes from requests to the bootstraps
	// should be enough to fend from most attacks.
	acceptInterval = build.Select(build.Var{
		Standard: 6 * time.Second,
		Dev:      3 * time.Second,
		Testing:  100 * time.Millisecond,
	}).(time.Duration)

	// acquiringPeersDelay defines the amount of time that is waited between
	// iterations of the peer acquisition loop if the gateway is actively
	// forming new connections with peers.
	acquiringPeersDelay = build.Select(build.Var{
		Standard: 5 * time.Second,
		Dev:      3 * time.Second,
		Testing:  500 * time.Millisecond,
	}).(time.Duration)

	// fullyConnectedThreshold defines the number of peers that the gateway can
	// have before it stops accepting inbound connections.
	fullyConnectedThreshold = build.Select(build.Var{
		Standard: 128,
		Dev:      20,
		Testing:  10,
	}).(int)

	// maxConcurrentOutboundPeerRequests defines the maximum number of peer
	// connections that the gateway will try to form concurrently.
	maxConcurrentOutboundPeerRequests = build.Select(build.Var{
		Standard: 3,
		Dev:      2,
		Testing:  2,
	}).(int)

	// noNodesDelay defines the amount of time that is waited between
	// iterations of the peer acquisition loop if the gateway does not have any
	// nodes in the nodelist.
	noNodesDelay = build.Select(build.Var{
		Standard: 20 * time.Second,
		Dev:      10 * time.Second,
		Testing:  3 * time.Second,
	}).(time.Duration)

	// unwawntedLocalPeerDelay defines the amount of time that is waited
	// between iterations of the permanentPeerManager if the gateway has at
	// least a few outbound peers, but is not well connected, and the recently
	// selected peer was a local peer. The wait is mostly to prevent the
	// gateway from hogging the CPU in the event that all peers are local
	// peers.
	unwantedLocalPeerDelay = build.Select(build.Var{
		Standard: 2 * time.Second,
		Dev:      1 * time.Second,
		Testing:  100 * time.Millisecond,
	}).(time.Duration)

	// wellConnectedDelay defines the amount of time that is waited between
	// iterations of the peer acquisition loop if the gateway is well
	// connected.
	wellConnectedDelay = build.Select(build.Var{
		Standard: 5 * time.Minute,
		Dev:      1 * time.Minute,
		Testing:  3 * time.Second,
	}).(time.Duration)

	// wellConnectedThreshold is the number of outbound connections at which
	// the gateway will not attempt to make new outbound connections.
	wellConnectedThreshold = build.Select(build.Var{
		Standard: 8,
		Dev:      5,
		Testing:  4,
	}).(int)

	// connectabilityCheckTimeout defines how long a connectability check's dial
	// will be allowed to block before it times out.
	connectabilityCheckTimeout = build.Select(build.Var{
		Standard: time.Minute * 2,
		Dev:      time.Minute * 5,
		Testing:  time.Second * 90,
	}).(time.Duration)
)

var (
	// connStdDeadline defines the standard deadline that should be used for
	// all temporary connections to the gateway.
	connStdDeadline = build.Select(build.Var{
		Standard: 5 * time.Minute,
		Dev:      2 * time.Minute,
		Testing:  30 * time.Second,
	}).(time.Duration)

	// the gateway will abort a connection attempt after this long
	dialTimeout = build.Select(build.Var{
		Standard: 3 * time.Minute,
		Dev:      20 * time.Second,
		Testing:  500 * time.Millisecond,
	}).(time.Duration)

	// rpcStdDeadline defines the standard deadline that should be used for all
	// incoming RPC calls.
	rpcStdDeadline = build.Select(build.Var{
		Standard: 5 * time.Minute,
		Dev:      3 * time.Minute,
		Testing:  5 * time.Second,
	}).(time.Duration)
)
