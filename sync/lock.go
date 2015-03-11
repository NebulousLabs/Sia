package sync

import (
	"fmt"
	"sync"
	"time"
)

// RWMutex provides locking functions, and an ability to detect and mitigate
// deadlocks.
type RWMutex struct {
	openLocks        map[int]string
	openLocksCounter int
	openLocksMutex   sync.Mutex

	maxLockTime time.Duration

	mu sync.RWMutex
}

// New takes a maxLockTime and returns a lock. The lock will never stay locked
// for more than maxLockTime, instead printing an error and unlocking after
// maxLockTime has passed.
func New(maxLockTime time.Duration) *RWMutex {
	return &RWMutex{
		openLocks:   make(map[int]string),
		maxLockTime: maxLockTime,
	}
}

// RLock will read lock the RWMutex. The id is so that if there is a problem, the
// dev can easily figure out which caller caused the problem. The return value
// is important for correctly managing unlocks.
func (rwm *RWMutex) RLock(id string) int {
	rwm.openLocksMutex.Lock()
	counter := rwm.openLocksCounter
	rwm.openLocks[counter] = id
	rwm.openLocksCounter++
	rwm.openLocksMutex.Unlock()

	rwm.mu.RLock()

	go func() {
		time.Sleep(rwm.maxLockTime)

		rwm.openLocksMutex.Lock()
		_, exists := rwm.openLocks[counter]
		if exists {
			fmt.Printf("RLock held for too long, using id %v and counter %v\n", id, counter)
			delete(rwm.openLocks, counter)
			rwm.mu.RUnlock()
		}
		rwm.openLocksMutex.Unlock()
	}()

	return counter
}

// RUnlock will read unlock the lock. The id is so devs can easily figure out
// which caller is causing problems. The counter is important for knowing which
// instance was holding the lock.
func (rwm *RWMutex) RUnlock(id string, counter int) {
	rwm.openLocksMutex.Lock()
	_, exists := rwm.openLocks[counter]
	if !exists {
		fmt.Printf("RUnlock called too late, using id %v and counter %v\n", id, counter)
	} else {
		delete(rwm.openLocks, counter)
		rwm.mu.RUnlock()
	}
	rwm.openLocksMutex.Unlock()
}

// Lock will lock the RWMutex. The id is so that if there is a problem, the dev
// can easily figure out which caller caused the problem. The return value is
// important for correctly managing unlocks.
func (rwm *RWMutex) Lock(id string) int {
	rwm.openLocksMutex.Lock()
	counter := rwm.openLocksCounter
	rwm.openLocks[counter] = id
	rwm.openLocksCounter++
	rwm.openLocksMutex.Unlock()

	rwm.mu.Lock()

	go func() {
		time.Sleep(rwm.maxLockTime)

		rwm.openLocksMutex.Lock()
		_, exists := rwm.openLocks[counter]
		if exists {
			fmt.Printf("Lock held for too long, using id %v and counter %v\n", id, counter)
			delete(rwm.openLocks, counter)
			rwm.mu.Unlock()
		}
		rwm.openLocksMutex.Unlock()
	}()

	return counter
}

// Unlock will unlock the lock. The id is so devs can easily figure out which
// caller is causing problems. The counter is important for knowing which
// instance was holding the lock.
func (rwm *RWMutex) Unlock(id string, counter int) {
	rwm.openLocksMutex.Lock()
	_, exists := rwm.openLocks[counter]
	if !exists {
		fmt.Printf("RUnlock called too late, using id %v and counter %v\n", id, counter)
	} else {
		delete(rwm.openLocks, counter)
		rwm.mu.Unlock()
	}
	rwm.openLocksMutex.Unlock()
}
