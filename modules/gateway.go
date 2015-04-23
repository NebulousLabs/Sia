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
		"23.239.14.98:9988",
	}
)

// RPCFunc is the type signature of functions that handle incoming RPCs.
type RPCFunc func(net.Conn) error

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

type GatewayInfo struct {
	Address NetAddress
	Peers   []NetAddress
	Nodes   int
}

// A Gateway facilitates the interactions between the local node and remote
// nodes (peers). It relays incoming blocks and transactions to local modules,
// and broadcasts outgoing blocks and transactions to peers. In a broad sense,
// it is responsible for ensuring that the local consensus set is consistent
// with the "network" consensus set.
type Gateway interface {
	// Bootstrap joins the Sia network and establishes an initial peer list.
	Bootstrap(NetAddress) error

	// Connect establishes a persistent connection to a peer.
	Connect(NetAddress) error

	// Disconnect terminates a connection to a peer.
	Disconnect(NetAddress) error

	// RegisterRPC registers a function to handle incoming connections that
	// supply the given RPC ID.
	RegisterRPC(string, RPCFunc)

	// RPC calls an RPC on the given address. RPC cannot be called on an
	// address that the Gateway is not connected to.
	RPC(NetAddress, string, RPCFunc) error

	// Info reports metadata about the Gateway.
	Info() GatewayInfo

	// Close safely stops the Gateway's listener process.
	Close() error
}
