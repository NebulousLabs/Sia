package sync

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// RWMutex provides locking functions, and an ability to detect and remove
// deadlocks.
type RWMutex struct {
	openLocks        map[int]struct{}
	openLocksCounter int
	openLocksMutex   sync.Mutex

	callDepth   int
	maxLockTime time.Duration

	mu sync.RWMutex
}

// New takes a maxLockTime and returns a lock. The lock will never stay locked
// for more than maxLockTime, instead printing an error and unlocking after
// maxLockTime has passed.
func New(maxLockTime time.Duration, callDepth int) RWMutex {
	return RWMutex{
		openLocks:   make(map[int]struct{}),
		maxLockTime: maxLockTime,
		callDepth:   callDepth,
	}
}

// safeLock is the generic function for doing safe locking. If the read flag is
// set, then a readlock will be used, otherwise a lock will be used.
func (rwm *RWMutex) safeLock(read bool) int {
	// Get the call stack.
	callingFiles := make([]string, rwm.callDepth+1)
	callingLines := make([]int, rwm.callDepth+1)
	for i := 0; i <= rwm.callDepth; i++ {
		_, callingFiles[i], callingLines[i], _ = runtime.Caller(2 + i)
	}

	// Safely register that a lock has been triggered.
	rwm.openLocksMutex.Lock()
	counter := rwm.openLocksCounter
	rwm.openLocks[counter] = struct{}{}
	rwm.openLocksCounter++
	rwm.openLocksMutex.Unlock()

	// Lock the mutex.
	if read {
		rwm.mu.RLock()
	} else {
		rwm.mu.Lock()
	}

	// Create the function that will wait for 'maxLockTime' and then check that
	// the lock has been disabled.

	go func() {
		time.Sleep(rwm.maxLockTime)

		rwm.openLocksMutex.Lock()
		defer rwm.openLocksMutex.Unlock()

		// Check that the lock has been removed and if it hasn't, remove it.
		_, exists := rwm.openLocks[counter]
		if exists {
			delete(rwm.openLocks, counter)
			if read {
				rwm.mu.RUnlock()
			} else {
				rwm.mu.Unlock()
			}

			var lockType string
			if read {
				lockType = "read lock"
			} else {
				lockType = "lock"
			}
			fmt.Printf("A %v was held for too long, id '%v'. Call stack:\n", lockType, counter)
			for i := 0; i <= rwm.callDepth; i++ {
				fmt.Printf("\tFile '%v', Line '%v'\n", callingFiles[i], callingLines[i])
			}
		}
	}()

	return counter
}

// safeUnlock is the generic function for doing safe unlocking. If the lock had
// to be removed because a deadlock was detected, an error is printed.
func (rwm *RWMutex) safeUnlock(read bool, counter int) {
	// Get the call stack.
	callingFiles := make([]string, rwm.callDepth+1)
	callingLines := make([]int, rwm.callDepth+1)
	for i := 0; i <= rwm.callDepth; i++ {
		_, callingFiles[i], callingLines[i], _ = runtime.Caller(2 + i)
	}

	rwm.openLocksMutex.Lock()
	defer rwm.openLocksMutex.Unlock()

	// Check if a deadlock has been detected and fixed manually.
	_, exists := rwm.openLocks[counter]
	if !exists {
		var lockType string
		if read {
			lockType = "read "
		} else {
			lockType = ""
		}
		fmt.Printf("A%v lock was held until deadlock, subsequent call to%v unlock failed. id '%v'. Call stack:\n", lockType, lockType, counter)
		for i := 0; i <= rwm.callDepth; i++ {
			fmt.Printf("\tFile '%v', Line '%v'\n", callingFiles[i], callingLines[i])
		}
		return
	}

	// Remove the lock.
	delete(rwm.openLocks, counter)
	if read {
		rwm.mu.RUnlock()
	} else {
		rwm.mu.Unlock()
	}
}

// RLock will read lock the RWMutex. The return value must be used as input
// when calling RUnlock.
func (rwm *RWMutex) RLock() int {
	return rwm.safeLock(true)
}

// RUnlock will read unlock the RWMutex. The return value of calling RLock must
// be used as input.
func (rwm *RWMutex) RUnlock(counter int) {
	rwm.safeUnlock(true, counter)
}

// Lock will lock the RWMutex. The return value must be used as input when
// calling RUnlock.
func (rwm *RWMutex) Lock() int {
	return rwm.safeLock(false)
}

// Unlock will unlock the RWMutex. The return value of calling Lock must be
// used as input.
func (rwm *RWMutex) Unlock(counter int) {
	rwm.safeUnlock(false, counter)
}
