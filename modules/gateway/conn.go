package gateway

import (
	"net"

	"github.com/NebulousLabs/Sia/modules"
)

// peerConn is a simple type that implements the modules.PeerConn interface.
type peerConn struct {
	net.Conn
	addr modules.NetAddress
}

// CallbackAddr returns the NetAddress that the peer is listening on.
func (pc *peerConn) CallbackAddr() modules.NetAddress {
	return pc.addr
}
