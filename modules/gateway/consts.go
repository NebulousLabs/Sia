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
)

var (
	// healthyNodeListLen defines the number of nodes that the gateway must
	// have in the node list before it will stop asking peers for more nodes.
	healthyNodeListLen = func() int {
		switch build.Release {
		case "dev":
			return 30
		case "standard":
			return 200
		case "testing":
			return 15
		default:
			panic("unrecognized build.Release in healthyNodeListLen")
		}
	}()

	// maxSharedNodes defines the number of nodes that will be shared between
	// peers when they are expanding their node lists.
	maxSharedNodes = func() uint64 {
		switch build.Release {
		case "dev":
			return 5
		case "standard":
			return 10
		case "testing":
			return 3
		default:
			panic("unrecognized build.Release in healthyNodeListLen")
		}
	}()

	// nodePurgeDelay defines the amount of time that is waited between each
	// iteration of the node purge loop.
	nodePurgeDelay = func() time.Duration {
		switch build.Release {
		case "dev":
			return 20 * time.Second
		case "standard":
			return 10 * time.Minute
		case "testing":
			return 500 * time.Millisecond
		default:
			panic("unrecognized build.Release in nodePurgeDelay")
		}
	}()

	// nodeListDelay defines the amount of time that is waited between each
	// iteration of the node list loop.
	nodeListDelay = func() time.Duration {
		switch build.Release {
		case "dev":
			return 3 * time.Second
		case "standard":
			return 5 * time.Second
		case "testing":
			return 500 * time.Millisecond
		default:
			panic("unrecognized build.Release in nodePurgeDelay")
		}
	}()

	// pruneNodeListLen defines the number of nodes that the gateway must have
	// to be pruning nodes from the node list.
	pruneNodeListLen = func() int {
		switch build.Release {
		case "dev":
			return 15
		case "standard":
			return 50
		case "testing":
			return 10
		default:
			panic("unrecognized build.Release in pruneNodeListLen")
		}
	}()
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
	acceptInterval = func() time.Duration {
		switch build.Release {
		case "dev":
			return 3 * time.Second
		case "standard":
			return 6 * time.Second
		case "testing":
			return 100 * time.Millisecond
		default:
			panic("unrecognized build.Release in acceptInterval")
		}
	}()

	// acquiringPeersDelay defines the amount of time that is waited between
	// iterations of the peer acquisition loop if the gateway is actively
	// forming new connections with peers.
	acquiringPeersDelay = func() time.Duration {
		switch build.Release {
		case "dev":
			return 3 * time.Second
		case "standard":
			return 5 * time.Second
		case "testing":
			return 500 * time.Millisecond
		default:
			panic("unrecognized build.Release in wellConnectedDelay")
		}
	}()

	// fullyConnectedThreshold defines the number of peers that the gateway can
	// have before it stops accepting inbound connections.
	fullyConnectedThreshold = func() int {
		switch build.Release {
		case "dev":
			return 20
		case "standard":
			return 128
		case "testing":
			return 10
		default:
			panic("unrecognized build.Release in fullyConnectedThreshold")
		}
	}()

	// maxConcurrentOutboundPeerRequests defines the maximum number of peer
	// connections that the gateway will try to form concurrently.
	maxConcurrentOutboundPeerRequests = func() int {
		switch build.Release {
		case "dev":
			return 2
		case "standard":
			return 3
		case "testing":
			return 2
		default:
			panic("unrecognized build.Release in maxConcurrentOutboundPeerRequests")
		}
	}()

	// noNodesDelay defines the amount of time that is waited between
	// iterations of the peer acquisition loop if the gateway does not have any
	// nodes in the nodelist.
	noNodesDelay = func() time.Duration {
		switch build.Release {
		case "dev":
			return 10 * time.Second
		case "standard":
			return 20 * time.Second
		case "testing":
			return 3 * time.Second
		default:
			panic("unrecognized build.Release in noNodesDelay")
		}
	}()

	// unwawntedLocalPeerDelay defines the amount of time that is waited
	// between iterations of the permanentPeerManager if the gateway has at
	// least a few outbound peers, but is not well connected, and the recently
	// selected peer was a local peer. The wait is mostly to prevent the
	// gateway from hogging the CPU in the event that all peers are local
	// peers.
	unwantedLocalPeerDelay = func() time.Duration {
		switch build.Release {
		case "dev":
			return 1 * time.Second
		case "standard":
			return 2 * time.Second
		case "testing":
			return 100 * time.Millisecond
		default:
			panic("unrecognized build.Release in unwawntedLocalPeerDelay")
		}
	}()

	// wellConnectedDelay defines the amount of time that is waited between
	// iterations of the peer acquisition loop if the gateway is well
	// connected.
	wellConnectedDelay = func() time.Duration {
		switch build.Release {
		case "dev":
			return 1 * time.Minute
		case "standard":
			return 5 * time.Minute
		case "testing":
			return 3 * time.Second
		default:
			panic("unrecognized build.Release in wellConnectedDelay")
		}
	}()

	// wellConnectedThreshold is the number of outbound connections at which
	// the gateway will not attempt to make new outbound connections.
	wellConnectedThreshold = func() int {
		switch build.Release {
		case "dev":
			return 5
		case "standard":
			return 8
		case "testing":
			return 4
		default:
			panic("unrecognized build.Release in wellConnectedThreshold")
		}
	}()
)

var (
	// connStdDeadline defines the standard deadline that should be used for
	// all temporary connections to the gateway.
	connStdDeadline = func() time.Duration {
		switch build.Release {
		case "dev":
			return 2 * time.Minute
		case "standard":
			return 5 * time.Minute
		case "testing":
			return 30 * time.Second
		default:
			panic("unrecognized build.Release in connStdDeadline")
		}
	}()

	// the gateway will abort a connection attempt after this long
	dialTimeout = func() time.Duration {
		switch build.Release {
		case "dev":
			return 20 * time.Second
		case "standard":
			return 2 * time.Minute
		case "testing":
			return 500 * time.Millisecond
		default:
			panic("unrecognized build.Release in dialTimeout")
		}
	}()
)
