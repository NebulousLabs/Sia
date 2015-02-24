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
	err := encoding.WriteObject(conn, host)
	// write error
	encoding.WriteObject(conn, "")
	return err
}
