package modules

import (
	"testing"
)

// TestHost tests the Host method of the NetAddress type.
func TestHost(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

		// Unspecified address tests.
		{"0.0.0.0:1234", false},
		{"[::]:1234", false},

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

// TestIsValid tests that IsValid only returns nil for valid addresses, and
// that it only returns ErrLoopbackAddr for loopback addresses.
func TestIsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr       NetAddress
		valid      bool
		isLoopback bool
	}{
		// Networks such as 10.0.0.x have been omitted from testing - behavior
		// for these networks is currently undefined.

		// Garbage addresses
		{addr: "", valid: false},
		{addr: "foo:bar:baz", valid: false},
		{addr: "garbage:6146:616", valid: false},
		// Missing host / port
		{addr: ":", valid: false},
		{addr: "111.111.111.111", valid: false},
		{addr: "12.34.45.64", valid: false},
		{addr: "[::2]", valid: false},
		{addr: "::2", valid: false},
		{addr: "foo", valid: false},
		{addr: "hn.com", valid: false},
		{addr: "世界", valid: false},
		{addr: "foo:", valid: false},
		{addr: "世界:", valid: false},
		{addr: ":foo", valid: false},
		{addr: ":世界", valid: false},
		// Invalid host / port chars
		{addr: " foo:bar", valid: false},
		{addr: "foo :bar", valid: false},
		{addr: "f oo:bar", valid: false},
		{addr: "foo: bar", valid: false},
		{addr: "foo:bar ", valid: false},
		{addr: "foo:b ar", valid: false},
		{addr: "\x00:bar", valid: false},
		{addr: "foo:\x00", valid: false},
		// Loopback address
		{addr: "localhost:bar", valid: false, isLoopback: true},
		{addr: "127.0.0.1:bar", valid: false, isLoopback: true},
		{addr: "[::1]:bar", valid: false, isLoopback: true},
		// Unspecified address
		{addr: "[::]:bar", valid: false},
		{addr: "0.0.0.0:bar", valid: false},
		// Valid addresses.
		{addr: "foo:bar", valid: true},
		{addr: "hn.com:8811", valid: true},
		{addr: "[::2]:bar", valid: true},
		{addr: "111.111.111.111:111", valid: true},
		{addr: "12.34.45.64:7777", valid: true},
		{addr: "世界:bar", valid: true},
		{addr: "bar:世界", valid: true},
		{addr: "世:界", valid: true},
	}
	for _, tt := range tests {
		err := tt.addr.IsValid()
		if (err == nil) != tt.valid || (err == ErrLoopbackAddr) != tt.isLoopback {
			t.Errorf("test failed: got err: '%v', in test: '%v'", err, tt)
		}
	}
}
