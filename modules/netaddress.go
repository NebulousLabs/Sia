package modules

import (
	"errors"
	"net"
	"strconv"
	"strings"

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
	if na.IsValid() != nil {
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
	if na.IsValid() != nil {
		return ""
	}
	return port
}

// isLoopback returns true for ip addresses that are on the same machine.
func (na NetAddress) isLoopback() bool {
	host, _, err := net.SplitHostPort(string(na))
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// IsValid returns an error if the NetAddress is invalid. A valid NetAddress
// is of the form "host:port", such that "host" is either a valid IPv4/IPv6
// address or a valid hostname, and "port" is an integer in the range
// [1,65535]. Furthermore, "host" may not be a loopback address (except during
// testing). Valid IPv4 addresses, IPv6 addresses, and hostnames are detailed
// in RFCs 791, 2460, and 952, respectively.
func (na NetAddress) IsValid() error {
	if build.Release != "testing" && na.isLoopback() {
		return errors.New("host is a loopback address")
	}

	host, port, err := net.SplitHostPort(string(na))
	if err != nil {
		return err
	}

	// First try to parse host as an IP address; if that fails, assume it is a
	// hostname.
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return errors.New("host is the unspecified address")
		}
	} else {
		if len(host) < 1 || len(host) > 253 {
			return errors.New("invalid hostname length")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) < 1 || len(label) > 63 {
				return errors.New("hostname contains label with invalid length")
			}
			for _, r := range strings.ToLower(label) {
				isLetter := 'a' <= r && r <= 'z'
				isNumber := '0' <= r && r <= '9'
				isHyphen := r == '-'
				if !(isLetter || isNumber || isHyphen) {
					return errors.New("host contains invalid characters")
				}
			}
		}
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return errors.New("port is not an integer")
	} else if portInt < 1 || portInt > 65535 {
		return errors.New("port is invalid")
	}

	return nil
}
