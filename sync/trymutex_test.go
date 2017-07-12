package sync

import (
	"sync"
	"testing"
	"time"
)

// TestTryMutexBasicMutex verifies that Lock and Unlock work the same as a
// normal mutex would.
func TestTryMutexBasicMutex(t *testing.T) {
	// Check that two calls to lock will execute in the correct order.
	var tm TryMutex
	var data int
	tm.Lock()
	go func() {
		data = 15
		tm.Unlock()
	}()
	tm.Lock()
	if data != 15 {
		t.Error("Locking did not safely protect the data")
	}
	tm.Unlock()
}

// TestTryMutexConcurrentLocking checks that doing lots of concurrent locks is
// handled as expected.
func TestTryMutexConcurrentLocking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Try executing multiple additions concurrently.
	var tm TryMutex
	var data int
	var wg sync.WaitGroup
	for i := 0; i < 250; i++ {
		wg.Add(1)
		go func() {
			tm.Lock()
			data++
			tm.Unlock()
			wg.Done()
		}()
	}
	wg.Wait()
	if data != 250 {
		t.Error("Locking did not safely protect the data")
	}
}

// TestTryMutexBasicTryLock checks that a TryLock will succeed if nobody is
// holding a lock, and will fail if the lock is being held.
func TestTryMutexBasicTryLock(t *testing.T) {
	// Lock and then TryLock.
	var tm TryMutex
	tm.Lock()
	if tm.TryLock() {
		t.Error("TryLock should have failed")
	}
	tm.Unlock()

	tm.Lock()
	tm.Unlock()

	// TryLock and then TryLock.
	if !tm.TryLock() {
		t.Error("Could not get a blank lock")
	}
	if tm.TryLock() {
		t.Error("should not have been able to get the lock")
	}
	tm.Unlock()
}

// TestTryMutexConcurrentTries attempts to grab locks from many threads, giving
// the race detector a chance to detect any issues.
func TestTryMutexConncurrentTries(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Try executing multiple additions concurrently.
	var tm TryMutex
	var data int
	var wg sync.WaitGroup
	for i := 0; i < 250; i++ {
		wg.Add(1)
		go func() {
			for !tm.TryLock() {
			}

			data++
			tm.Unlock()
			wg.Done()
		}()
	}
	wg.Wait()
	if data != 250 {
		t.Error("Locking did not safely protect the data")
	}
}

// TestTryMutexTimed checks that a timed lock will correctly time out if it
// cannot grab a lock.
func TestTryMutexTimed(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	var tm TryMutex
	tm.Lock()

	startTime := time.Now()
	if tm.TryLockTimed(time.Millisecond * 500) {
		t.Error("was able to grab a locked lock")
	}

	wait := time.Now().Sub(startTime)
	if wait < time.Millisecond*450 {
		t.Error("lock did not wait the correct amount of time before timing out", wait)
	}
	if wait > time.Millisecond*900 {
		t.Error("lock waited too long before timing out", wait)
	}

	tm.Unlock()
	if !tm.TryLockTimed(time.Millisecond * 1) {
		t.Error("Unable to get an unlocked lock")
	}
	tm.Unlock()
}

// TestTryMutexTimedConcurrent checks that a timed lock will correctly time out
// if it cannot grab a lock.
func TestTryMutexTimedConcurrent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	var tm TryMutex

	// Engage a lock and launch a gothread to wait for a lock, fail, and then
	// call unlock.
	tm.Lock()
	go func() {
		startTime := time.Now()
		if tm.TryLockTimed(time.Millisecond * 500) {
			t.Error("was able to grab a locked lock")
		}

		wait := time.Now().Sub(startTime)
		if wait < time.Millisecond*450 {
			t.Error("lock did not wait the correct amount of time before timing out:", wait)
		}
		if wait > time.Millisecond*900 {
			t.Error("lock waited too long before timing out", wait)
		}

		tm.Unlock()
	}()

	// Try to get a lock, but don't wait long enough.
	if tm.TryLockTimed(time.Millisecond * 250) {
		// Lock shoud time out because the gothread responsible for releasing
		// the lock will be idle for 500 milliseconds.
		t.Error("Lock should have timed out")
	}
	if !tm.TryLockTimed(time.Millisecond * 950) {
		// Lock should be successful - the above thread should finish in under
		// 950 milliseconds.
		t.Error("Lock should have been successful")
	}
	tm.Unlock()
}
