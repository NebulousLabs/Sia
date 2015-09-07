package modules

import (
	"net"
	"regexp"

	"github.com/NebulousLabs/Sia/build"
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

// IsValid uses a regex to check whether the net address is a valid ip address
// or hostname.
func (na NetAddress) IsValid() bool {
	host := na.Host()
	// Check if the host is a valid ip address. Host will have been returned as
	// the empty string (which is not a valid ip address) if there is anything
	// structurally incorrect with the NetAddress.
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
