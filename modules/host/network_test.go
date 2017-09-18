package host

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
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
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()

	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// TODO: this causes an ndf, because it relies on the host tester starting up
	// and fully returning faster than the first check, which isnt always the
	// case.  This check is disabled for now, but can be fixed by using the
	// disrupt() pattern.
	// if ht.host.WorkingStatus() != modules.HostWorkingStatusChecking {
	// 	t.Fatal("expected working state to initially be modules.HostWorkingStatusChecking")
	// }

	for i := 0; i < 5; i++ {
		// Simulate some setting calls, and see if the host picks up on it.
		atomic.AddUint64(&ht.host.atomicSettingsCalls, workingStatusThreshold+1)

		success := false
		for start := time.Now(); time.Since(start) < 30*time.Second; time.Sleep(time.Millisecond * 10) {
			if ht.host.WorkingStatus() == modules.HostWorkingStatusWorking {
				success = true
				break
			}
		}
		if !success {
			t.Fatal("expected working state to flip to HostWorkingStatusWorking after incrementing settings calls")
		}

		// make no settings calls, host should flip back to NotWorking
		success = false
		for start := time.Now(); time.Since(start) < 30*time.Second; time.Sleep(time.Millisecond * 10) {
			if ht.host.WorkingStatus() == modules.HostWorkingStatusNotWorking {
				success = true
				break
			}
		}
		if !success {
			t.Fatal("expected working state to flip to HostStatusNotWorking if no settings calls occur")
		}
	}
}

// TestHostConnectabilityStatus checks that the host properly updates its connectable
// state
func TestHostConnectabilityStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// TODO: this causes an ndf, because it relies on the host tester starting up
	// and fully returning faster than the first check, which isnt always the
	// case.  This check is disabled for now, but can be fixed by using the
	// disrupt() pattern.
	// if ht.host.ConnectabilityStatus() != modules.HostConnectabilityStatusChecking {
	// 		t.Fatal("expected connectability state to initially be ConnectablityStateChecking")
	// }

	success := false
	for start := time.Now(); time.Since(start) < 30*time.Second; time.Sleep(time.Millisecond * 10) {
		if ht.host.ConnectabilityStatus() == modules.HostConnectabilityStatusConnectable {
			success = true
			break
		}
	}
	if !success {
		t.Fatal("expected connectability state to flip to HostConnectabilityStatusConnectable")
	}
}
