package modules

import (
	"net"
	"strings"
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
		"localhost:",
		"[::1]:",
		// Invalid host / port chars
		"localhost:-",
		"[::1]:-",
		"foo:{}",
		"{}:123",
		" foo:123",
		"foo :123",
		"f oo:123",
		"foo: 123",
		"foo:123 ",
		"foo:1 23",
		"\x00:123",
		"foo:\x00",
		"世界:123",
		"bar:世界",
		"世:界",
		`":"`,
		// Unspecified address
		"[::]:bar",
		"0.0.0.0:bar",
		// Invalid hostnames
		"unqualifiedhost:123",
		"Yo-Amazon.we-are-really-happy-for-you.and-we-will-let-you-finish.but-Sia-is-the-best-cloud-storage-of-all-time.of-all-time-of-all-time-of-all-time-of-all-time-of-all-time.of-all-time-of-all-time-of-all-time-of-all-time-of-all-time.of-all-time-of-all-time:123",
		strings.Repeat("a", 64) + ".com:123",                       // 64 char long label too long.
		strings.Repeat(strings.Repeat("a", 62)+".", 4) + "co:123",  // 254 char long hostname too long.
		strings.Repeat(strings.Repeat("a", 62)+".", 4) + "co.:123", // 254 char long hostname with trailing dot too long.
		"-foo.bar:123",
		"foo-.bar:123",
		"foo.-bar:123",
		"foo.bar-:123",
		"foo-bar.-baz:123",
		"foo-bar.baz-:123",
		"foo.-bar.baz:123",
		"foo.bar-.baz:123",
		".:123",
		".foo.com:123",
		"foo.com..:123",
		// invalid port numbers
		"foo:0",
		"foo:65536",
		"foo:-100",
		"foo:1000000",
		"localhost:0",
		"[::1]:0",
	}
	validAddrs = []string{
		// Loopback address (valid in testing only, can't really test this well)
		"localhost:123",
		"127.0.0.1:123",
		"[::1]:123",
		// Valid addresses.
		"foo.com:1",
		"foo.com.:1",
		"a.b.c:1",
		"a.b.c.:1",
		"foo-bar.com:123",
		"FOO.com:1",
		"1foo.com:1",
		"tld.foo.com:1",
		"hn.com:8811",
		strings.Repeat("foo.", 63) + "f:123",                     // 253 chars long
		strings.Repeat("foo.", 63) + "f.:123",                    // 254 chars long, 253 chars long without trailing dot
		strings.Repeat(strings.Repeat("a", 63)+".", 3) + "a:123", // 3x63 char length labels + 1x1 char length label without trailing dot
		strings.Repeat(strings.Repeat("a", 63)+".", 3) + ":123",  // 3x63 char length labels with trailing dot
		"[::2]:65535",
		"111.111.111.111:111",
		"12.34.45.64:7777",
	}
)

// TestHostPort tests the Host and Port methods of the NetAddress type.
func TestHostPort(t *testing.T) {
	t.Parallel()

	// Test valid addrs.
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

	// Test that Host / Port return "" when net.SplitHostPort errors
	na := NetAddress("::")
	host := na.Host()
	port := na.Port()
	if host != "" {
		t.Error("expected Host() to return blank for an un-splittable NetAddress, but it returned:", host)
	}
	if port != "" {
		t.Error("expected Port() to return blank for an un-splittable NetAddress, but it returned:", port)
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

// TestIsValid tests that IsValid only returns nil for valid addresses.
func TestIsValid(t *testing.T) {
	t.Parallel()

	for _, addr := range validAddrs {
		na := NetAddress(addr)
		if err := na.IsValid(); err != nil {
			t.Errorf("IsValid returned non-nil for valid NetAddress %q: %v", addr, err)
		}
	}
	for _, addr := range invalidAddrs {
		na := NetAddress(addr)
		if err := na.IsValid(); err == nil {
			t.Errorf("IsValid returned nil for an invalid NetAddress %q: %v", addr, err)
		}
	}
}
