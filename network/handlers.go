package network

import (
	"net"

	"github.com/NebulousLabs/Sia/encoding"
)

// Ping returns whether an Address is reachable and responds correctly to the
// ping request -- in other words, whether it is a potential peer.
func Ping(addr Address) bool {
	var pong string
	err := addr.RPC("Ping", nil, &pong)
	return err == nil && pong == "pong"
}

func pong() (string, error) {
	return "pong", nil
}

// sendHostname replies to the sender with the sender's external IP.
func sendHostname(conn net.Conn) error {
	host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	_, err := encoding.WriteObject(conn, host)
	// write error
	encoding.WriteObject(conn, "")
	return err
}

// sharePeers replies to the sender with 10 randomly selected peers.
// Note: the set of peers may contain duplicates.
func (tcps *TCPServer) sharePeers() (addrs []Address, err error) {
	addrs = tcps.AddressBook()
	if len(addrs) > 10 {
		addrs = addrs[:10]
	}
	return
}

// addRemote adds the connecting address as a peer, using the supplied port
// number. The port number must be sent manually because it may differ from
// the conn's port number; this is due to NAT.
func (tcps *TCPServer) addRemote(conn net.Conn) (err error) {
	var addr Address
	if err = encoding.ReadObject(conn, &addr, maxMsgLen); err != nil {
		return
	}
	// check that this is the correct hostname
	connHost, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	addrHost, _, _ := net.SplitHostPort(string(addr))
	if connHost != addrHost {
		_, err = encoding.WriteObject(conn, "supplied hostname does not match connection's hostname")
		return
	}
	// check that the host is reachable on this port
	if !Ping(addr) {
		_, err = encoding.WriteObject(conn, "supplied hostname did not respond to ping")
		return
	}
	tcps.AddPeer(addr)
	// write error
	encoding.WriteObject(conn, "")
	return
}
