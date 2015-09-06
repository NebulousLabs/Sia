package modules

import (
	"testing"
)

// TestRemovePort tests the RemovePort method of the NetAddress.
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
	}
	for _, test := range testSet {
		if test.query.RemovePort() != test.desiredResponse {
			t.Error("test failed:", test, test.query.RemovePort())
		}
	}
}
