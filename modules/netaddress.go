package modules

import (
	"errors"
	"net"
	"unicode"

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

// IsValid returns nil if the NetAddress is valid. Valid is defined as being of
// the form "host:port" such that "host" is not a loopback address (except in
// testing) nor an unspecified IP address, and such that neither the host nor
// port are blank, contain white space, or contain the null character.
func (na NetAddress) IsValid() error {
	host, port, err := net.SplitHostPort(string(na))
	if err != nil {
		return err
	}

	for _, runeValue := range host {
		if unicode.IsSpace(runeValue) || runeValue == 0 {
			return errors.New("host has invalid characters")
		}
	}
	if host == "" {
		return errors.New("host is blank")
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return errors.New("host is the unspecified address")
	}

	for _, runeValue := range port {
		if unicode.IsSpace(runeValue) || runeValue == 0 {
			return errors.New("port has invalid characters")
		}
	}
	if port == "" {
		return errors.New("port is blank")
	}

	if build.Release != "testing" && na.isLoopback() {
		return errors.New("host is a loopback address")
	}
	return nil
}
