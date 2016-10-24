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
	BootstrapPeers = func() []NetAddress {
		switch build.Release {
		case "dev":
			return nil
		case "standard":
			return []NetAddress{
				"101.200.214.115:9981",
				"104.223.98.174:9981",
				"109.172.42.157:9981",
				"109.206.33.225:9981",
				"109.71.42.163:9981",
				"109.71.42.164:9981",
				"113.98.98.164:9981",
				"115.187.229.102:9981",
				"120.25.198.251:9981",
				"138.201.13.159:9981",
				"139.162.152.204:9981",
				"141.105.11.33:9981",
				"142.4.209.72:9981",
				"148.251.221.163:9981",
				"158.69.120.71:9981",
				"162.210.249.170:9981",
				"162.222.23.93:9981",
				"176.9.59.110:9981",
				"176.9.72.2:9981",
				"180.167.17.236:9981",
				"18.239.0.53:9981",
				"183.86.218.232:9981",
				"188.166.61.155:9981",
				"188.166.61.157:9981",
				"188.166.61.158:9981",
				"188.166.61.159:9981",
				"188.166.61.163:9981",
				"188.61.177.92:9981",
				"190.10.8.173:9981",
				"193.198.102.34:9981",
				"194.135.90.38:9981",
				"195.154.243.233:9981",
				"202.63.55.79:9981",
				"210.14.155.90:9981",
				"213.251.158.199:9981",
				"217.65.8.75:9981",
				"222.187.224.89:9981",
				"222.187.224.93:9981",
				"23.239.14.98:9971",
				"23.239.14.98:9981",
				"23.239.14.98:9981",
				"24.91.0.62:9981",
				"31.178.227.21:9981",
				"37.139.1.78:9981",
				"37.139.28.207:9981",
				"43.227.113.131:9981",
				"45.79.132.35:9981",
				"45.79.159.167:9981",
				"46.105.118.15:9981",
				"52.205.238.6:9981",
				"62.107.201.170:9981",
				"62.210.207.79:9981",
				"62.212.77.12:9981",
				"64.31.31.106:9981",
				"68.55.10.144:9981",
				"73.73.50.191:33721",
				"76.164.234.13:9981",
				"78.119.218.13:9981",
				"79.172.204.10:9981",
				"79.51.183.132:9981",
				"80.234.37.94:9981",
				"82.196.11.170:9981",
				"82.196.5.50:9981",
				"82.220.99.82:9981",
				"83.76.19.197:10981",
				"85.255.197.69:9981",
				"87.236.27.155:12487",
				"87.98.216.46:9981",
				"88.109.8.173:9981",
				"91.121.183.171:9981",
				"95.211.203.138:9981",
				"95.85.14.54:9981",
				"95.85.15.69:9981",
				"95.85.15.71:9981",
			}
		case "testing":
			return nil
		default:
			panic("unrecognized build.Release constant in BootstrapPeers")
		}
	}()
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

		// Close safely stops the Gateway's listener process.
		Close() error
	}
)
