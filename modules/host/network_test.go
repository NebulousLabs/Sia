package host

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
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

// TestHostWorkingStatus checks that the host properly updates its working
// state
func TestHostWorkingStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	workingStatusFrequency = 5 * time.Second

	t.Parallel()

	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	if ht.host.WorkingStatus() != modules.HostWorkingStatusChecking {
		t.Fatal("expected working state to initially be modules.HostWorkingStatusChecking")
	}

	atomic.StoreUint64(&ht.host.atomicSettingsCalls, workingStatusThreshold+1)

	time.Sleep(workingStatusFrequency)
	time.Sleep(time.Second)

	if ht.host.WorkingStatus() != modules.HostWorkingStatusWorking {
		t.Fatal("expected host working status to be modules.HostWorkingStatusWorking after incrementing status calls")
	}

	time.Sleep(workingStatusFrequency)
	time.Sleep(time.Second)

	if ht.host.WorkingStatus() != modules.HostWorkingStatusNotWorking {
		t.Fatal("expected host working status to be modules.HostWorkingStatusNotWorking after waiting workingStatusFrequency with no settings calls")
	}
}

// TestHostConnectabilityStatus checks that the host properly updates its connectable
// state
func TestHostConnectabilityStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	connectabilityCheckFrequency = 5 * time.Second

	t.Parallel()

	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	if ht.host.ConnectabilityStatus() != modules.HostConnectabilityStatusChecking {
		t.Fatal("expected connectability state to initially be ConnectablityStateChecking")
	}

	time.Sleep(connectabilityCheckFrequency)
	time.Sleep(time.Second)

	if ht.host.ConnectabilityStatus() != modules.HostConnectabilityStatusConnectable {
		t.Fatal("expected connectability state to be modules.HostConnectabilityStatusConnectable")
	}
}
