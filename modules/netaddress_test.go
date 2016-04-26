package modules

import (
	"net"
	"testing"
)

var (
	// Networks such as 10.0.0.x have been omitted from testing - behavior
	// for these networks is currently undefined.

	invalidAddrs = []string{
		// Garbage addresses
		"",
		"foo:bar:baz",
		"garbage:6146:616",
		// Missing host / port
		":",
		"111.111.111.111",
		"12.34.45.64",
		"[::2]",
		"::2",
		"foo",
		"hn.com",
		"世界",
		"foo:",
		"世界:",
		":foo",
		":世界",
		// Invalid host / port chars
		" foo:bar",
		"foo :bar",
		"f oo:bar",
		"foo: bar",
		"foo:bar ",
		"foo:b ar",
		"\x00:bar",
		"foo:\x00",
		// Unspecified address
		"[::]:bar",
		"0.0.0.0:bar",
	}
	validAddrs = []string{
		// Loopback address (valid in testing only, can't really test this well)
		"localhost:bar",
		"127.0.0.1:bar",
		"[::1]:bar",
		// Valid addresses.
		"foo:bar",
		"hn.com:8811",
		"[::2]:bar",
		"111.111.111.111:111",
		"12.34.45.64:7777",
		"世界:bar",
		"bar:世界",
		"世:界",
		`":"`, // yeah, okay
	}
)

// TestHostPort tests the Host and Port methods of the NetAddress type.
func TestHostPort(t *testing.T) {
	t.Parallel()

	for _, addr := range validAddrs {
		na := NetAddress(addr)
		host := na.Host()
		port := na.Port()
		expectedHost, expectedPort, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatal(err)
		}
		if host != expectedHost {
			t.Errorf("Host() returned unexpected host for NetAddress '%v': expected '%v', got '%v'", na, expectedHost, host)
		}
		if port != expectedPort {
			t.Errorf("Port() returned unexpected port for NetAddress '%v': expected '%v', got '%v'", na, expectedPort, port)
		}
	}
	for _, addr := range invalidAddrs {
		na := NetAddress(addr)
		host := na.Host()
		port := na.Port()
		if host != "" {
			t.Errorf("Expected Host() to return blank string for invalid NetAddress '%v', but got '%v'", na, host)
		}
		if port != "" {
			t.Errorf("Expected Port() to return blank string for invalid NetAddress '%v', but got '%v'", na, port)
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
		if test.query.isLoopback() != test.desiredResponse {
			t.Error("test failed:", test, test.query.isLoopback())
		}
	}
}

// TestIsValid tests that IsValid only returns nil for valid addresses.
func TestIsValid(t *testing.T) {
	t.Parallel()

	for _, addr := range validAddrs {
		na := NetAddress(addr)
		if err := na.IsValid(); err != nil {
			t.Error("IsValid returned non-nil for a valid NetAddress:", err)
		}
	}
	for _, addr := range invalidAddrs {
		na := NetAddress(addr)
		if err := na.IsValid(); err == nil {
			t.Error("IsValid returned nil for an invalid NetAddress")
		}
	}
}
