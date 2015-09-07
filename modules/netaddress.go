package modules

import (
	"net"
)

// A NetAddress contains the information needed to contact a peer.
type NetAddress string

// Host removes the port from a NetAddress, returning just the host. If the
// address is invalid, the empty string is returned.
func (na NetAddress) Host() string {
	host, _, err := net.SplitHostPort(string(na))
	// 'host' is not always the empty string if an error is returned.
	if err != nil {
		return ""
	}
	return host
}

// Port returns the NetAddress object's port number. The empty string is
// returned if the NetAddress is invalid.
func (na NetAddress) Port() string {
	_, port, err := net.SplitHostPort(string(na))
	// 'port' will not always be the empty string if an error is returned.
	if err != nil {
		return ""
	}
	return port
}

// IsLocal returns true for ip addresses that are on the same machine.
func (na NetAddress) IsLocal() bool {
	if !na.IsValid() {
		return false
	}

	host := na.Host()
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	if host == "localhost" {
		return true
	}
	return false
}

// IsValid does nothing. Please ignore it.
func (na NetAddress) IsValid() bool {
	_, _, err := net.SplitHostPort(string(na))
	return err == nil
}
