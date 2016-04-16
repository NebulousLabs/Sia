package host

/*
import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestRPCMetrics checks that the rpc tracking is counting incoming RPC cals.
func TestRPCMetrics(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRPCMetrics")
	if err != nil {
		t.Fatal(err)
	}

	// Upload a test file to get some metrics to increment.
	_, err = ht.uploadFile("TestRPCMetrics - 1", renewDisabled)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadUint64(&ht.host.atomicReviseCalls) != 1 {
		t.Error("expected to count a revise call")
	}
	if atomic.LoadUint64(&ht.host.atomicSettingsCalls) != 1 {
		t.Error("expected to count a settings call")
	}
	if atomic.LoadUint64(&ht.host.atomicUploadCalls) != 1 {
		t.Error("expected to count an upload call")
	}

	// Restart the host and check that the counts persist.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadUint64(&rebootHost.atomicReviseCalls) != 1 {
		t.Error("expected to count a revise call")
	}
	if atomic.LoadUint64(&rebootHost.atomicSettingsCalls) != 1 {
		t.Error("expected to count a settings call")
	}
	if atomic.LoadUint64(&rebootHost.atomicUploadCalls) != 1 {
		t.Error("expected to count an upload call")
	}
}
*/
