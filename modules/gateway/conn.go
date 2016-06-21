package gateway

import (
	"net"

	"github.com/NebulousLabs/Sia/modules"
)

// peerConn is a simple type that implements the modules.PeerConn interface.
type peerConn struct {
	net.Conn
	dialbackAddr modules.NetAddress
}

// RPCAddr implements the RPCAddr method of the modules.PeerConn interface. It
// is the address that identifies a peer.
func (pc peerConn) RPCAddr() modules.NetAddress {
	return pc.dialbackAddr
}
