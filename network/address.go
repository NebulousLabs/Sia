package network

import (
	"net"
)

// An Address contains the information needed to contact a peer over TCP.
type Address string

// Host returns the Address' IP.
func (a Address) Host() string {
	host, _, _ := net.SplitHostPort(string(a))
	return host
}

// Port returns the Address' port number.
func (a Address) Port() string {
	_, port, _ := net.SplitHostPort(string(a))
	return port
}
