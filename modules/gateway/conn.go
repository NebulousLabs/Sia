package gateway

import (
	"net"

	"github.com/NebulousLabs/Sia/modules"
)

// peerConn is a simple type that implements the modules.PeerConn interface.
// Eventually it may monitor the connection to thwart malicious behavior.
type peerConn struct {
	net.Conn
	addr modules.NetAddress
}

func (pc *peerConn) CallbackAddr() modules.NetAddress {
	return pc.addr
}
