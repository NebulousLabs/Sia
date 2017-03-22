package sync

import (
	"runtime"
	"sync"
	"testing"
)

// TestTryRWMutexBasicMutex verifies that Lock and Unlock work the same as a
// normal mutex would.
func TestTryRWMutexBasicMutex(t *testing.T) {
	// Check that two calls to lock will execute in the correct order.
	var tm TryRWMutex
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

// TestTryRWMutexConcurrentLocking checks that doing lots of concurrent locks
// is handled as expected.
func TestTryRWMutexConcurrentLocking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Try executing multiple additions concurrently.
	var tm TryRWMutex
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

// TestTryRWMutexBasicTryLock checks that a TryLock will succeed if nobody is
// holding a lock, and will fail if the lock is being held.
func TestTryRWMutexBasicTryLock(t *testing.T) {
	// Lock and then TryLock.
	var tm TryRWMutex
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

// TestTryRWMutexConcurrentTries attempts to grab locks from many threads,
// giving the race detector a chance to detect any issues.
func TestTryRWMutexConncurrentTries(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Try executing multiple additions concurrently.
	var tm TryRWMutex
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

// TestTryRWMutexReadAvailable will try to acquire a read lock on the mutex
// when it is supposed to be available.
func TestTryRWMutexReadAvailable(t *testing.T) {
	var tm TryRWMutex
	if !tm.TryRLock() {
		t.Fatal("Unable to get readlock on a fresh TryRWMutex")
	}

	// Grab the lock and increment the data in a goroutine.
	var data int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		tm.Lock()
		data++
		tm.Unlock()
	}()
	runtime.Gosched()
	go func() {
		defer wg.Done()
		tm.Lock()
		data++
		tm.Unlock()
	}()
	runtime.Gosched()

	// Read the data, readlock should be held.
	if data != 0 {
		t.Fatal("Data should not have changed while under readlock")
	}

	// Release the lock and wait for the other locks to finish their
	// modifications.
	tm.RUnlock()
	wg.Wait()

	// Try to grab another readlock. It should succeed. The data should have
	// changed.
	if !tm.TryRLock() {
		t.Fatal("Unable to get readlock on available TryRWMutex")
	}
	if data != 2 {
		t.Error("Data does not seem to have been altered correctly")
	}
	tm.RUnlock()
}

// TestTryRWMutexReadUnavailable will try to acquire a read lock on the mutex
// when it is supposed to be available.
func TestTryRWMutexReadUnavailable(t *testing.T) {
	var tm TryRWMutex
	if !tm.TryRLock() {
		t.Fatal("Unable to get readlock on a fresh TryRWMutex")
	}

	// Grab the lock and increment the data in a goroutine.
	var data int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		tm.Lock()
		data++
		tm.Unlock()
	}()
	runtime.Gosched()
	go func() {
		defer wg.Done()
		tm.Lock()
		data++
		tm.Unlock()
	}()
	runtime.Gosched()

	// Read the data, readlock should be held.
	if data != 0 {
		t.Fatal("Data should not have changed while under readlock")
	}

	// Release the lock and wait for the other locks to finish their
	// modifications.
	tm.RUnlock()

	// Try to grab another readlock. It should succeed. The data should have
	// changed.
	if tm.TryRLock() {
		t.Fatal("Able to get readlock on available TryRWMutex")
	}
	wg.Wait()
}
