package sync

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestLowThreadLocking checks that locks are functional in the safelock
// mechanism, only 2 threads are used to try and trigger a race condition.
func TestLowThreadLocking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a value and lock a mutex to protect the value.
	value := 0
	safeLock := New(time.Second, 1)
	outerID := safeLock.Lock()
	go func() {
		// Lock a mutex and read the value. Value should be 1, since the old
		// mutex was not released
		innerID := safeLock.Lock()
		defer safeLock.Unlock(innerID)
		if value != 1 {
			t.Fatal("Lock was grabbed incorrectly")
		}
	}()

	// After spawning the other thread, increment value.
	value = 1
	safeLock.Unlock(outerID)
}

// TestHighThreadLocking tries to trigger race conditions while using lots of
// threads and sleep tactics.
func TestHighThreadLocking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Try to trigger a race condition by using lots of threads.
	for i := 0; i < 50; i++ {
		go func() {
			// Create a value and lock a mutex to protect the value.
			value := 0
			safeLock := New(time.Second, 1)
			outerID := safeLock.Lock()
			go func() {
				// Lock a mutex and read the value. Value should be 1, since
				// the old mutex was not released
				innerID := safeLock.Lock()
				defer safeLock.Unlock(innerID)
				if value != 1 {
					t.Fatal("Lock was grabbed incorrectly")
				}
			}()

			// Some sleeps and a call to gosched to try and give the thread
			// control to the spawned thread.
			time.Sleep(time.Millisecond * 25)
			runtime.Gosched()
			time.Sleep(time.Millisecond * 25)
			value = 1
			safeLock.Unlock(outerID)
		}()
	}
}

// TestReadLocking checks that the readlocks can overlap without interference
// from a writelock.
func TestReadLocking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	startTime := time.Now().Unix()
	value := 0
	safeLock := New(time.Second, 1)
	writeID := safeLock.Lock()

	readThreads := 100
	var wg sync.WaitGroup
	wg.Add(readThreads)
	for i := 0; i < readThreads; i++ {
		go func() {
			readID := safeLock.RLock()
			defer safeLock.RUnlock(readID)

			if value != 1 {
				t.Error("reading is not happening correctly")
			}

			// Sleep 250 milliseconds after grabbing the readlock. Because
			// there are a bunch of threads, if the readlocks are not grabbing
			// the lock in parallel the test will take a long time.
			time.Sleep(time.Millisecond * 250)
			wg.Done()
		}()
	}
	value = 1

	// A combination of sleep and gosched to give priority to the other
	// threads.
	time.Sleep(time.Millisecond * 100)
	runtime.Gosched()
	time.Sleep(time.Millisecond * 100)
	safeLock.Unlock(writeID)

	// Wait for all of the threads to finish sleeping.
	wg.Wait()
	// Check that the whole test took under 3 seconds. If the readlocks were
	// efficiently being grabbed in parallel, the test should be subtantially
	// less than 3 seconds.
	if time.Now().Unix()-startTime > 3 {
		t.Error("test took too long to complete")
	}
}

// TestLockSafety checks that a safelock correctly unwinds a deadlock.
func TestLockSafety(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	startTime := time.Now().Unix()
	safeLock := New(time.Millisecond*25, 1)

	// Trigger a deadlock by writelocking twice. The deadlock detector should
	// resolve the issue.
	outerWrite := safeLock.Lock()
	innerWrite := safeLock.Lock()
	safeLock.Unlock(outerWrite)
	safeLock.Unlock(innerWrite)

	// Trigger a deadlock by readlocking and then writelocking. The deadlock
	// detector should resolve the issue.
	readID := safeLock.RLock()
	writeID := safeLock.Lock()
	safeLock.RUnlock(readID)
	safeLock.Unlock(writeID)

	// Check that the whole test took under 3 seconds. If the deadlock detector
	// is working, the time elapsed should be much less than 3 seconds.
	if time.Now().Unix()-startTime > 2 {
		t.Error("test took too long to complete")
	}
}
