package modules

import (
	"net"

	"gitlab.com/NebulousLabs/Sia/build"
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
			"162.211.163.190:9981",
			"67.254.219.180:9981",
			"108.28.206.98:9981",
			"162.211.163.189:9981",
			"192.254.68.245:9981",
			"69.125.184.202:9981",
			"66.49.218.20:9981",
			"66.49.218.102:9981",
			"72.229.14.109:9981",
			"209.182.216.47:9981",
			"65.30.45.202:9981",
			"174.58.166.159:9981",
			"147.135.132.95:9981",
			"88.202.202.221:9981",
			"51.15.41.218:9981",
			"147.135.133.105:9981",
			"147.135.133.100:9981",
			"94.23.7.123:9981",
			"24.125.195.213:9981",
			"107.181.137.139:9981",
			"67.207.162.194:9981",
			"147.135.132.94:9981",
			"94.23.54.36:9981",
			"147.135.133.114:9981",
			"72.24.114.182:9981",
			"85.27.163.135:9981",
			"77.132.24.85:9981",
			"62.210.140.24:9981",
			"217.160.178.117:9981",
			"80.239.147.28:9981",
			"94.137.140.40:9981",
			"193.248.47.218:9981",
			"85.214.229.44:9981",
			"138.68.84.69:9981",
			"93.103.248.239:9981",
			"76.28.239.179:9981",
			"76.174.155.33:9984",
			"94.225.83.165:9981",
			"78.41.116.91:9981",
			"51.175.10.196:9981",
			"188.242.52.10:9988",
			"188.242.52.10:9986",
			"5.20.184.133:9981",
			"81.167.50.168:9981",
			"83.142.60.64:9981",
			"85.21.232.162:9981",
			"188.244.40.69:9985",
			"188.244.40.69:9981",
			"95.78.166.67:9981",
			"109.194.162.16:9981",
			"78.248.218.28:9981",
			"217.173.23.149:9981",
			"91.231.94.22:9981",
			"27.120.81.167:9981",
			"162.211.163.188:9981",
			"122.214.1.30:9981",
			"94.201.207.205:9981",
			"71.65.230.65:9332",
			"109.248.206.13:9981",
			"114.55.95.111:9981",
			"47.94.45.200:9981",
			"121.254.64.110:9981",
			"45.56.21.129:9981",
			"188.242.52.10:9981",
			"116.62.118.38:30308",
			"174.138.20.196:9981",
			"71.199.19.26:9981",
			"78.46.64.86:9981",
			"115.70.170.117:9981",
			"203.87.65.150:9981",
			"75.82.191.146:19981",
			"163.172.215.185:9981",
			"108.162.149.228:9981",
			"69.172.174.21:9981",
			"91.206.15.126:9981",
			"13.93.44.96:9981",
			"188.244.40.69:9997",
			"92.70.88.30:9981",
			"109.173.126.111:9981",
			"5.9.139.30:9981",
			"213.239.221.175:9981",
			"66.49.218.179:9981",
			"163.172.209.161:9981",
			"101.200.202.108:9981",
			"24.6.98.156:9981",
			"162.211.163.187:9981",
			"176.9.51.183:9981",
			"87.249.251.18:9981",
			"74.107.124.167:9981",
			"163.172.18.134:9981",
			"176.9.144.99:9981",
			"45.22.40.217:9981",
			"62.210.93.142:9981",
			"80.108.192.203:9981",
			"5.57.198.66:9981",
			"147.135.133.117:9981",
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

		// DiscoverAddress discovers and returns the current public IP address of the
		// gateway. Contrary to Address, DiscoverAddress is blocking and might take multiple minutes to return
		DiscoverAddress(cancel <-chan struct{}) (NetAddress, error)

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
