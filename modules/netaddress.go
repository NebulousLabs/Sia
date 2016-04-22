package modules

import (
	"errors"
	"net"
	"unicode"
)

var (
	// ErrLoopbackAddr is returned by IsValid() to indicate a NetAddress is a
	// loopback address.
	ErrLoopbackAddr = errors.New("host is a loopback address")
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

// IsLoopback returns true for ip addresses that are on the same machine.
func (na NetAddress) IsLoopback() bool {
	if _, _, err := net.SplitHostPort(string(na)); err != nil {
		return false
	}
	host := na.Host()
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// IsValid returns nil if the NetAddress is valid. Valid is defined as being of
// the form "host:port" such that "host" is not a loopback address or localhost
// nor an unspecified IP address, and such that neither the host nor port are
// blank, contain white space, or contain the null character.
func (na NetAddress) IsValid() error {
	if _, _, err := net.SplitHostPort(string(na)); err != nil {
		return err
	}

	host := na.Host()
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

	port := na.Port()
	for _, runeValue := range port {
		if unicode.IsSpace(runeValue) || runeValue == 0 {
			return errors.New("port has invalid characters")
		}
	}
	if port == "" {
		return errors.New("port is blank")
	}

	// This check must be last, as it is common to check for ErrLoopbackAddr and
	// ignore the error during testing. If this check was not last, an invalid
	// port could be mistaken as a loopback but otherwise valid address.
	if na.IsLoopback() {
		return ErrLoopbackAddr
	}
	return nil
}
