package modules

import (
	"net"

	"github.com/NebulousLabs/Sia/build"
)

const (
	// GatewayDir is the name of the directory used to store the gateway's
	// persistent data.
	GatewayDir = "gateway"
)

var (
	// BootstrapPeers is a list of peers that can be used to find other peers -
	// when a client first connects to the network, the only options for
	// finding peers are either manual entry of peers or to use a hardcoded
	// bootstrap point. While the bootstrap point could be a central service,
	// it can also be a list of peers that are known to be stable. We have
	// chosen to hardcode known-stable peers.
	BootstrapPeers = build.Select(build.Var{
		Standard: []NetAddress{
			"23.175.0.151:9981",
			"51.15.48.114:9981",
			"137.74.1.200:9981",
			"23.239.14.98:9971",
			"23.239.14.98:9981",
			"163.172.209.161:9981",
			"147.135.133.107:9981",
			"147.135.133.105:9981",
			"66.49.218.102:9981",
			"72.24.114.182:9981",
			"108.162.149.228:9981",
			"162.211.163.188:9981",
			"24.6.98.156:9981",
			"209.182.206.161:9981",
			"62.210.93.142:9981",
			"72.229.14.109:9981",
			"45.22.40.217:9981",
			"139.162.187.240:9991",
			"66.49.218.20:9981",
			"163.172.18.134:9981",
			"94.23.54.36:9981",
			"162.211.163.189:9981",
			"67.254.219.180:9981",
			"88.202.202.221:9981",
			"66.49.218.179:9981",
			"65.30.45.202:9981",
			"147.135.133.101:9981",
			"176.28.42.24:9981",
			"147.135.133.117:9981",
			"67.207.162.194:9981",
			"94.23.7.123:9981",
			"195.154.173.18:9981",
			"71.199.19.26:9981",
			"13.93.44.96:9981",
			"74.107.124.167:9981",
			"107.181.137.139:9981",
			"185.172.165.40:9981",
			"147.135.133.100:9981",
		},
		Dev:     []NetAddress(nil),
		Testing: []NetAddress(nil),
	}).([]NetAddress)
)

type (
	// Peer contains all the info necessary to Broadcast to a peer.
	Peer struct {
		Inbound    bool       `json:"inbound"`
		Local      bool       `json:"local"`
		NetAddress NetAddress `json:"netaddress"`
		Version    string     `json:"version"`
	}

	// A PeerConn is the connection type used when communicating with peers during
	// an RPC. It is identical to a net.Conn with the additional RPCAddr method.
	// This method acts as an identifier for peers and is the address that the
	// peer can be dialed on. It is also the address that should be used when
	// calling an RPC on the peer.
	PeerConn interface {
		net.Conn
		RPCAddr() NetAddress
	}

	// RPCFunc is the type signature of functions that handle RPCs. It is used for
	// both the caller and the callee. RPCFuncs may perform locking. RPCFuncs may
	// close the connection early, and it is recommended that they do so to avoid
	// keeping the connection open after all necessary I/O has been performed.
	RPCFunc func(PeerConn) error

	// A Gateway facilitates the interactions between the local node and remote
	// nodes (peers). It relays incoming blocks and transactions to local modules,
	// and broadcasts outgoing blocks and transactions to peers. In a broad sense,
	// it is responsible for ensuring that the local consensus set is consistent
	// with the "network" consensus set.
	Gateway interface {
		// Connect establishes a persistent connection to a peer.
		Connect(NetAddress) error

		// Disconnect terminates a connection to a peer.
		Disconnect(NetAddress) error

		// Address returns the Gateway's address.
		Address() NetAddress

		// Peers returns the addresses that the Gateway is currently connected to.
		Peers() []Peer

		// RegisterRPC registers a function to handle incoming connections that
		// supply the given RPC ID.
		RegisterRPC(string, RPCFunc)

		// UnregisterRPC unregisters an RPC and removes all references to the RPCFunc
		// supplied in the corresponding RegisterRPC call. References to RPCFuncs
		// registered with RegisterConnectCall are not removed and should be removed
		// with UnregisterConnectCall. If the RPC does not exist no action is taken.
		UnregisterRPC(string)

		// RegisterConnectCall registers an RPC name and function to be called
		// upon connecting to a peer.
		RegisterConnectCall(string, RPCFunc)

		// UnregisterConnectCall unregisters an RPC and removes all references to the
		// RPCFunc supplied in the corresponding RegisterConnectCall call. References
		// to RPCFuncs registered with RegisterRPC are not removed and should be
		// removed with UnregisterRPC. If the RPC does not exist no action is taken.
		UnregisterConnectCall(string)

		// RPC calls an RPC on the given address. RPC cannot be called on an
		// address that the Gateway is not connected to.
		RPC(NetAddress, string, RPCFunc) error

		// Broadcast transmits obj, prefaced by the RPC name, to all of the
		// given peers in parallel.
		Broadcast(name string, obj interface{}, peers []Peer)

		// Online returns true if the gateway is connected to remote hosts
		Online() bool

		// Close safely stops the Gateway's listener process.
		Close() error
	}
)
