package modules

import (
	"net"
)

const (
	GatewayDir = "gateway"
)

// TODO: Move this and it's functionality into the gateway package.
var (
	BootstrapPeers = []NetAddress{
		"23.239.14.98:9981",
		"87.98.216.46:9981",
	}
)

// A PeerConn is the connection type used when communicating with peers during
// an RPC. For now it is identical to a net.Conn.
type PeerConn interface {
	net.Conn
	// AddStrike() ?
}

// RPCFunc is the type signature of functions that handle RPCs. It is used for
// both the caller and the callee. RPCFuncs may perform locking. RPCFuncs may
// close the connection early, and it is recommended that they do so to avoid
// keeping the connection open after all necessary I/O has been performed.
type RPCFunc func(PeerConn) error

// A NetAddress contains the information needed to contact a peer.
type NetAddress string

// Host returns the NetAddress' IP.
func (na NetAddress) Host() string {
	host, _, _ := net.SplitHostPort(string(na))
	return host
}

// Port returns the NetAddress' port number.
func (na NetAddress) Port() string {
	_, port, _ := net.SplitHostPort(string(na))
	return port
}

// A Gateway facilitates the interactions between the local node and remote
// nodes (peers). It relays incoming blocks and transactions to local modules,
// and broadcasts outgoing blocks and transactions to peers. In a broad sense,
// it is responsible for ensuring that the local consensus set is consistent
// with the "network" consensus set.
type Gateway interface {
	// Connect establishes a persistent connection to a peer.
	Connect(NetAddress) error

	// Disconnect terminates a connection to a peer.
	Disconnect(NetAddress) error

	// Address returns the Gateway's address.
	Address() NetAddress

	// Peers returns the addresses that the Gateway is currently connected to.
	Peers() []NetAddress

	// RegisterRPC registers a function to handle incoming connections that
	// supply the given RPC ID.
	RegisterRPC(string, RPCFunc)

	// RegisterConnectCall registers an RPC name and function to be called
	// upon connecting to a peer.
	RegisterConnectCall(string, RPCFunc)

	// RPC calls an RPC on the given address. RPC cannot be called on an
	// address that the Gateway is not connected to.
	RPC(NetAddress, string, RPCFunc) error

	// Broadcast transmits obj, prefaced by the RPC name, to all of the
	// Gateway's connected peers in parallel.
	Broadcast(name string, obj interface{})

	// Close safely stops the Gateway's listener process.
	Close() error
}
