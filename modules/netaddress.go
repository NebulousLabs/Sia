package modules

import (
	"bytes"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/NebulousLabs/Sia/build"
)

// MaxEncodedNetAddressLength is the maximum length of a NetAddress encoded
// with the encode package. 266 was chosen because the maximum length for the
// hostname is 254 + 1 for the separating colon + 5 for the port + 8 byte
// string length prefix.
const MaxEncodedNetAddressLength = 266

// A NetAddress contains the information needed to contact a peer.
type NetAddress string

// Host removes the port from a NetAddress, returning just the host. If the
// address is not of the form "host:port" the empty string is returned. The
// port will still be returned for invalid NetAddresses (e.g. "unqualified:0"
// will return "unqualified"), but in general you should only call Host on
// valid addresses.
func (na NetAddress) Host() string {
	host, _, err := net.SplitHostPort(string(na))
	// 'host' is not always the empty string if an error is returned.
	if err != nil {
		return ""
	}
	return host
}

// Port returns the NetAddress object's port number. If the address is not of
// the form "host:port" the empty string is returned. The port will still be
// returned for invalid NetAddresses (e.g. "localhost:0" will return "0"), but
// in general you should only call Port on valid addresses.
func (na NetAddress) Port() string {
	_, port, err := net.SplitHostPort(string(na))
	// 'port' will not always be the empty string if an error is returned.
	if err != nil {
		return ""
	}
	return port
}

// IsLoopback returns true for IP addresses that are on the same machine.
func (na NetAddress) IsLoopback() bool {
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

// IsPrivate returns true if the input IP address belongs to a private address
// range (non-loopback) such as 192.168.x.x
func (na NetAddress) IsPrivate() bool {
	// If it's loopback, it doesn't count. Checking this first allows us to
	// check for a non-IP address hostname without hitting any edge cases.
	if na.IsLoopback() {
		return false
	}

	// Grab the IP address of the net address. If there is an error parsing,
	// return false, as it's not a private ip address range.
	ip := net.ParseIP(na.Host())
	if ip == nil {
		return false
	}
	ip16 := ip.To16()

	// Get the ranges of the private IP addresses.
	range1Low := net.ParseIP("10.0.0.0").To16()
	range1High := net.ParseIP("10.255.255.255").To16()
	range2Low := net.ParseIP("172.16.0.0").To16()
	range2High := net.ParseIP("172.31.255.255").To16()
	range3Low := net.ParseIP("192.168.0.0").To16()
	range3High := net.ParseIP("192.168.255.255").To16()
	range4Low := net.ParseIP("fd00:0000:0000:0000:0000:0000:0000:0000")
	range4High := net.ParseIP("fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")

	// Sanity check - all values should be non-nil.
	if ip16 == nil ||
		range1Low == nil || range1High == nil ||
		range2Low == nil || range2High == nil ||
		range3Low == nil || range3High == nil ||
		range4Low == nil || range4High == nil {
		panic("invalid range")
	}

	// Return true if ip16 falls between any of the above defined ranges.
	if bytes.Compare(range1Low, ip16) <= 0 && bytes.Compare(range1High, ip16) <= 0 {
		return true
	}
	if bytes.Compare(range2Low, ip16) <= 0 && bytes.Compare(range2High, ip16) <= 0 {
		return true
	}
	if bytes.Compare(range3Low, ip16) <= 0 && bytes.Compare(range3High, ip16) <= 0 {
		return true
	}
	if bytes.Compare(range4Low, ip16) <= 0 && bytes.Compare(range4High, ip16) <= 0 {
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
	host, port, err := net.SplitHostPort(string(na))
	if err != nil {
		return err
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return errors.New("port is not an integer")
	} else if portInt < 1 || portInt > 65535 {
		return errors.New("port is invalid")
	}

	// This check must come after the valid port check so that a host such as
	// "localhost:badport" will fail.
	if na.IsLoopback() {
		if build.Release == "testing" {
			return nil
		}
		return errors.New("host is a loopback address")
	}

	// First try to parse host as an IP address; if that fails, assume it is a
	// hostname.
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return errors.New("host is the unspecified address")
		}
	} else {
		// Hostnames can have a trailing dot (which indicates that the hostname is
		// fully qualified), but we ignore it for validation purposes.
		if strings.HasSuffix(host, ".") {
			host = host[:len(host)-1]
		}
		if len(host) < 1 || len(host) > 253 {
			return errors.New("invalid hostname length")
		}
		labels := strings.Split(host, ".")
		if len(labels) == 1 {
			return errors.New("unqualified hostname")
		}
		for _, label := range labels {
			if len(label) < 1 || len(label) > 63 {
				return errors.New("hostname contains label with invalid length")
			}
			if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return errors.New("hostname contains label that starts or ends with a hyphen")
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

	return nil
}
