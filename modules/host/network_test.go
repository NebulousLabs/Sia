package host

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestRPCTracking checks that the rpc tracking is counting incoming RPC cals.
func TestRPCTracking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRPCTracking")
	if err != nil {
		t.Fatal(err)
	}

	// Upload a test file to get some metrics to increment.
	err = ht.uploadFile("TestRPCTracking - 1")
	if err != nil {
		t.Fatal(err)
	}
	if ht.host.reviseCalls != 1 {
		t.Error("expected to count a revise call")
	}
	if ht.host.settingsCalls != 1 {
		t.Error("expected to count a settings call")
	}
	if ht.host.uploadCalls != 1 {
		t.Error("expected to count an upload call")
	}

	// Restart the host and check that the counts persist.
	ht.host.Close()
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	if rebootHost.reviseCalls != 1 {
		t.Error("expected to count a revise call")
	}
	if rebootHost.settingsCalls != 1 {
		t.Error("expected to count a settings call")
	}
	if rebootHost.uploadCalls != 1 {
		t.Error("expected to count an upload call")
	}
}
