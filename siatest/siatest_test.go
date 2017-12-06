package siatest

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

// TestNewTestNode checks that the NewTestNode method is properly build and can
// use the actual 'New' calls by the modules to create test objects.
func TestNewTestNode(t *testing.T) {
	testDir := build.TempDir("TempDir", "TestNewTestNode")
	tn, err := NewTestNode(NewTestNodeParams{
		Dir: testDir,

		NewGatewayFunc: gateway.New,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tn.Gateway == nil {
		t.Error("gateway not set correctly")
	}
}
