package host

import (
	"testing"
	"time"
)

// blockingPortForward is a dependency set that causes the host port forward
// call at startup to block for 10 seconds, simulating the amount of blocking
// that can occur in production.
//
// blockingPortForward will also cause managedClearPort to always return an
// error.
type blockingPortForward struct {
	productionDependencies
}

// disrupt will cause the port forward call to block for 10 seconds, but still
// complete normally. disrupt will also cause managedClearPort to return an
// error.
func (blockingPortForward) disrupt(s string) bool {
	// Return an error when clearing the port.
	if s == "managedClearPort return error" {
		return true
	}

	// Block during port forwarding.
	if s == "managedForwardPort" {
		time.Sleep(time.Second * 3)
	}
	return false
}

// TestPortFowardBlocking checks that the host does not accidentally call a
// write on a closed logger due to a long-running port forward call.
func TestPortForwardBlocking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newMockHostTester(blockingPortForward{}, "TestPortForwardBlocking")
	if err != nil {
		t.Fatal(err)
	}

	// The close operation would previously fail here because of improper
	// thread control regarding upnp and shutdown.
	err = ht.Close()
	if err != nil {
		t.Fatal(err)
	}

	// The trailing sleep is needed to catch the previously existing error
	// where the host was not shutting down correctly. Currently, the extra
	// sleep does nothing, but in the regression a logging panic would occur.
	time.Sleep(time.Second * 4)
}

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
	defer ht.Close()

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
