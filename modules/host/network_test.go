package host

import (
	"sync/atomic"
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

// TestHostWorkingState checks that the host properly updates its working
// state
func TestHostWorkingState(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	workingStateFrequency = 5 * time.Second

	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	if ht.host.WorkingState() != WorkingStateChecking {
		t.Fatal("expected working state to initially be WorkingStateChecking")
	}

	atomic.StoreUint64(&ht.host.atomicSettingsCalls, workingStateThreshold+1)

	time.Sleep(workingStateFrequency)
	time.Sleep(time.Second)

	if ht.host.WorkingState() != WorkingStateWorking {
		t.Fatal("expected host working status to be WorkingStateWorking after incrementing status calls")
	}

	time.Sleep(workingStateFrequency)
	time.Sleep(time.Second)

	if ht.host.WorkingState() != WorkingStateNotWorking {
		t.Fatal("expected host working status to be WorkingStateNotWorking after waiting workingStateFrequency with no settings calls")
	}
}

// TestHostConnectabilityState checks that the host properly updates its connectable
// state
func TestHostConnectabilityState(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	connectabilityCheckFrequency = 5 * time.Second

	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	if ht.host.ConnectabilityState() != ConnectabilityStateChecking {
		t.Fatal("expected connectability state to initially be ConnectablityStateChecking")
	}

	time.Sleep(connectabilityCheckFrequency)
	time.Sleep(time.Second)

	if ht.host.ConnectabilityState() != ConnectabilityStateConnectable {
		t.Fatal("expected connectability state to be ConnectabilityStateConnectable")
	}
}
