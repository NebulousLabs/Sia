package gateway

import (
	"net"
	"time"

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

// dial will dial the input address and return a connection. dial appropriately
// handles things like clean shutdown, fast shutdown, and chooses the correct
// communication protocol.
func (g *Gateway) dial(addr modules.NetAddress) (net.Conn, error) {
	dialer := &net.Dialer{
		Cancel:  g.threads.StopChan(),
		Timeout: dialTimeout,
		LocalAddr: &net.TCPAddr{
			IP: g.listener.Addr().(*net.TCPAddr).IP,
		},
	}
	conn, err := dialer.Dial("tcp", string(addr))
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(connStdDeadline))
	return conn, nil
}
