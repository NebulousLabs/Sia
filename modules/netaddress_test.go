package modules

import (
	"testing"
)

// TestRemovePort tests the RemovePort method of the NetAddress type.
func TestRemovePort(t *testing.T) {
	testSet := []struct {
		query           NetAddress
		desiredResponse string
	}{
		{"localhost", "localhost"},
		{"localhost:1234", "localhost"},
		{"127.0.0.1", "127.0.0.1"},
		{"127.0.0.1:6723", "127.0.0.1"},
		{"bbc.com", "bbc.com"},
		{"bbc.com:6434", "bbc.com"},
		{"bitcoin.ninja", "bitcoin.ninja"},
		{"bitcoin.ninja:6752", "bitcoin.ninja"},
		{"garbage:64:77", "garbage:64:77"},
		{"::1:5856", "::1:5856"},
		{"[::1]:5856", "::1"},
		{"[::1]", "[::1]"},
		{"::1", "::1"},
	}
	for _, test := range testSet {
		if test.query.RemovePort() != test.desiredResponse {
			t.Error("test failed:", test, test.query.RemovePort())
		}
	}
}

// TestIsLocal tests the IsLocal method of the NetAddress type.
func TestIsLocal(t *testing.T) {
	testSet := []struct {
		query           NetAddress
		desiredResponse bool
	}{
		// Networks such as 10.0.0.x have been omitted from testing - behavior
		// for these networks is currently undefined.

		// Localhost tests.
		{"localhost", true},
		{"localhost:1234", true},
		{"127.0.0.1", true},
		{"127.0.0.1:6723", true},
		{"::1", true},
		{"[::1]:7124", true},

		// Public name tests.
		{"hn.com", false},
		{"hn.com:8811", false},
		{"12.34.45.64", false},
		{"12.34.45.64:7777", false},
		{"::1:4646", false}, // TODO: I'm not sure if this is actually localhost. The trailing values are part of the IPAddress, not part of the port.

		// Garbage name tests.
		{"garbage", false},
		{"garbage:6432", false},
		{"garbage:6146:616", false},
		{"[::1]", false},
	}
	for _, test := range testSet {
		if test.query.IsLocal() != test.desiredResponse {
			t.Error("test failed:", test, test.query.IsLocal())
		}
	}
}

// TestIsValid checks where a netaddress matches the regex for what counts as a
// valid hostname or ip address.
func TestIsValid(t *testing.T) {
	testSet := []struct {
		query           NetAddress
		desiredResponse bool
	}{
		// Networks such as 10.0.0.x have been omitted from testing - behavior
		// for these networks is currently undefined.

		// Localhost tests.
		{"localhost", true},
		{"localhost:1234", true},
		{"127.0.0.1", true},
		{"127.0.0.1:6723", true},
		{"::1", true},
		{"[::1]:7124", true},

		// Public name tests.
		{"hn.com", true},
		{"hn.com:8811", true},
		{"12.34.45.64", true},
		{"12.34.45.64:7777", true},
		{"::1:4646", true},
		{"plain", true},
		{"plain:6432", true},

		// Garbage name tests.
		{"garbage:6146:616", false},
		{"[::1]", false},
	}
	for _, test := range testSet {
		if test.query.IsValid() != test.desiredResponse {
			t.Error("test failed:", test, test.query.IsValid())
		}
	}
}
