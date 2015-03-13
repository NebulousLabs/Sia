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
	openLocks        map[int]lockInfo
	openLocksCounter int
	openLocksMutex   sync.Mutex

	callDepth   int
	maxLockTime time.Duration

	mu sync.RWMutex
}

// lockInfo contains information about when and how a lock call was made.
type lockInfo struct {
	// When the lock was called.
	lockTime time.Time

	// Call stack of the caller.
	callingFiles []string
	callingLines []int
}

// New takes a maxLockTime and returns a lock. The lock will never stay locked
// for more than maxLockTime, instead printing an error and unlocking after
// maxLockTime has passed.
func New(maxLockTime time.Duration, callDepth int) *RWMutex {
	rwm := &RWMutex{
		openLocks:   make(map[int]lockInfo),
		maxLockTime: maxLockTime,
		callDepth:   callDepth,
	}

	go rwm.threadedDeadlockFinder()

	return rwm
}

// threadedDeadlockFinder occasionally freezes the mutexes and scans all open mutexes,
// reporting any that have exceeded their time limit.
func (rwm *RWMutex) threadedDeadlockFinder() {
	for {
		rwm.openLocksMutex.Lock()
		for id, info := range rwm.openLocks {
			// Check if the lock has been held for longer than 'maxLockTime'.
			if time.Now().Sub(info.lockTime) > rwm.maxLockTime {
				fmt.Printf("A lock was held for too long, id '%v'. Call stack:\n", id)
				for i := 0; i <= rwm.callDepth; i++ {
					fmt.Printf("\tFile: '%v:%v'\n", info.callingFiles[i], info.callingLines[i])
				}
			}
		}
		rwm.openLocksMutex.Unlock()

		time.Sleep(rwm.maxLockTime)
	}
}

// safeLock is the generic function for doing safe locking. If the read flag is
// set, then a readlock will be used, otherwise a lock will be used.
func (rwm *RWMutex) safeLock(read bool) int {
	// Get the call stack.
	var li lockInfo
	li.lockTime = time.Now()
	li.callingFiles = make([]string, rwm.callDepth+1)
	li.callingLines = make([]int, rwm.callDepth+1)
	for i := 0; i <= rwm.callDepth; i++ {
		_, li.callingFiles[i], li.callingLines[i], _ = runtime.Caller(2 + i)
	}

	// Safely register that a lock has been triggered.
	rwm.openLocksMutex.Lock()
	counter := rwm.openLocksCounter
	rwm.openLocks[counter] = li
	rwm.openLocksCounter++
	rwm.openLocksMutex.Unlock()

	// Lock the mutex.
	if read {
		rwm.mu.RLock()
	} else {
		rwm.mu.Lock()
	}

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
		fmt.Printf("A lock was held until deadlock, subsequent call to unlock failed. id '%v'. Call stack:\n", counter)
		for i := 0; i <= rwm.callDepth; i++ {
			fmt.Printf("\tFile: '%v:%v'\n", callingFiles[i], callingLines[i])
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
