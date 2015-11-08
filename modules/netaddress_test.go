package modules

import (
	"testing"
)

// TestHost tests the Host method of the NetAddress type.
func TestHost(t *testing.T) {
	testSet := []struct {
		query           NetAddress
		desiredResponse string
	}{
		{"localhost", ""},
		{"localhost:1234", "localhost"},
		{"127.0.0.1", ""},
		{"127.0.0.1:6723", "127.0.0.1"},
		{"bbc.com", ""},
		{"bbc.com:6434", "bbc.com"},
		{"bitcoin.ninja", ""},
		{"bitcoin.ninja:6752", "bitcoin.ninja"},
		{"garbage:64:77", ""},
		{"::1:5856", ""},
		{"[::1]:5856", "::1"},
		{"[::1]", ""},
		{"::1", ""},
	}
	for _, test := range testSet {
		if test.query.Host() != test.desiredResponse {
			t.Error("test failed:", test, test.query.Host())
		}
	}
}

// TestIsLoopback tests the IsLoopback method of the NetAddress type.
func TestIsLoopback(t *testing.T) {
	testSet := []struct {
		query           NetAddress
		desiredResponse bool
	}{
		// Networks such as 10.0.0.x have been omitted from testing - behavior
		// for these networks is currently undefined.

		// Localhost tests.
		{"localhost", false},
		{"localhost:1234", true},
		{"127.0.0.1", false},
		{"127.0.0.1:6723", true},
		{"0.0.0.0:1234", true},
		{"[::]:1234", true},
		{"::1", false},
		{"[::1]:7124", true},

		// Public name tests.
		{"hn.com", false},
		{"hn.com:8811", false},
		{"12.34.45.64", false},
		{"12.34.45.64:7777", false},

		// Garbage name tests.
		{"", false},
		{"garbage", false},
		{"garbage:6432", false},
		{"garbage:6146:616", false},
		{"::1:4646", false},
		{"[::1]", false},
	}
	for _, test := range testSet {
		if test.query.IsLoopback() != test.desiredResponse {
			t.Error("test failed:", test, test.query.IsLoopback())
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
		{"localhost", false},
		{"localhost:1234", true},
		{"127.0.0.1", false},
		{"127.0.0.1:6723", true},
		{"::1", false},
		{"[::1]:7124", true},

		// Public name tests.
		{"hn.com", false},
		{"hn.com:8811", true},
		{"12.34.45.64", false},
		{"12.34.45.64:7777", true},
		{"::1:4646", false},
		{"plain", false},
		{"plain:6432", true},

		// Garbage name tests.
		{"", false},
		{"garbage:6146:616", false},
		{"[::1]", false},
		// {"google.com:notAPort", false}, TODO: Failed test case.
	}
	for _, test := range testSet {
		if test.query.IsValid() != test.desiredResponse {
			t.Error("test failed:", test, test.query.IsValid())
		}
	}
}
