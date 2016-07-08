package sync

import (
	"sync"
	"testing"
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
