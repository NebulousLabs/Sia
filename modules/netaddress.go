package modules

import (
	"net"
	"regexp"

	"github.com/NebulousLabs/Sia/build"
)

// A NetAddress contains the information needed to contact a peer.
type NetAddress string

// Host removes the port from a NetAddress, returning just the host.
func (na NetAddress) Host() string {
	host, _, err := net.SplitHostPort(string(na))
	if err != nil {
		return string(na)
	}
	return host
}

// Port returns the NetAddress' port number.
//
// TODO: Unchecked error.
func (na NetAddress) Port() string {
	_, port, _ := net.SplitHostPort(string(na))
	return port
}

// IsLocal returns true for ip addresses that are on the same machine.
func (na NetAddress) IsLocal() bool {
	if !na.IsValid() {
		return false
	}

	host := na.RemovePort()
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	if host == "localhost" {
		return true
	}
	return false
}

// IsValid uses a regex to check whether the net address is a valid ip address
// or hostname.
func (na NetAddress) IsValid() bool {
	host := na.RemovePort()
	// Check if the host is a valid ip address.
	if net.ParseIP(host) != nil {
		return true
	}

	// A regex pulled from
	// http://stackoverflow.com/questions/106179/regular-expression-to-match-dns-hostname-or-ip-address
	// to check for a valid hostname.
	regHostname, err := regexp.Compile("^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]{0,61}[a-zA-Z0-9])(\\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]{0,61}[a-zA-Z0-9]))*$")
	if build.DEBUG && err != nil {
		panic(err)
	}
	if err != nil {
		return false
	}
	if regHostname.MatchString(host) {
		return true
	}
	return false
}
