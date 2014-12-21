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

func pong(pong *string) error {
	*pong = "pong"
	return nil
}

// sendHostname replies to the sender with the sender's external IP.
func sendHostname(conn net.Conn) error {
	host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	_, err := encoding.WriteObject(conn, host)
	return err
}

// sharePeers replies to the sender with 10 randomly selected peers.
// Note: the set of peers may contain duplicates.
func (tcps *TCPServer) sharePeers(addrs *[]Address) error {
	*addrs = tcps.AddressBook()
	if len(*addrs) > 10 {
		*addrs = (*addrs)[:10]
	}
	return nil
}

// addRemote adds the connecting address as a peer.
func (tcps *TCPServer) addRemote(conn net.Conn) (err error) {
	addr, err := encoding.ReadPrefix(conn, maxMsgLen)
	if err != nil {
		return err
	}
	// check that this is the correct hostname
	connHost, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	addrHost, _, _ := net.SplitHostPort(string(addr))
	if connHost != addrHost {
		return
	}
	// make sure the host is reachable on this port
	if Ping(Address(addr)) {
		tcps.AddPeer(Address(addr))
	}
	return
}
