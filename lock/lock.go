package lock

import (
	"fmt"
	"sync"
	"time"
)

// Lock provides locking functions, and an ability to detect and mitigate
// deadlocks.
type Lock struct {
	openLocks        map[int]string
	openLocksCounter int
	openLocksMutex   sync.Mutex

	maxLockTime time.Duration

	mu sync.RWMutex
}

// New takes a maxLockTime and returns a lock. The lock will never stay locked
// for more than maxLockTime, instead printing an error and unlocking after
// maxLockTime has passed.
func New(maxLockTime time.Duration) *Lock {
	return &Lock{
		openLocks:   make(map[int]string),
		maxLockTime: maxLockTime,
	}
}

// RLock will read lock the Lock. The id is so that if there is a problem, the
// dev can easily figure out which caller caused the problem. The return value
// is important for correctly managing unlocks.
func (l *Lock) RLock(id string) int {
	l.openLocksMutex.Lock()
	counter := l.openLocksCounter
	l.openLocks[counter] = id
	l.openLocksCounter++
	l.openLocksMutex.Unlock()

	l.mu.RLock()

	go func() {
		time.Sleep(l.maxLockTime)

		l.openLocksMutex.Lock()
		_, exists := l.openLocks[counter]
		if exists {
			fmt.Printf("RLock held for too long, using id %v and counter %v\n", id, counter)
			delete(l.openLocks, counter)
			l.mu.RUnlock()
		}
		l.openLocksMutex.Unlock()
	}()

	return counter
}

// RUnlock will read unlock the lock. The id is so devs can easily figure out
// which caller is causing problems. The counter is important for knowing which
// instance was holding the lock.
func (l *Lock) RUnlock(id string, counter int) {
	l.openLocksMutex.Lock()
	_, exists := l.openLocks[counter]
	if !exists {
		fmt.Printf("RUnlock called too late, using id %v and counter %v\n", id, counter)
	} else {
		delete(l.openLocks, counter)
		l.mu.RUnlock()
	}
	l.openLocksMutex.Unlock()
}

// Lock will lock the Lock. The id is so that if there is a problem, the dev
// can easily figure out which caller caused the problem. The return value is
// important for correctly managing unlocks.
func (l *Lock) Lock(id string) int {
	l.openLocksMutex.Lock()
	counter := l.openLocksCounter
	l.openLocks[counter] = id
	l.openLocksCounter++
	l.openLocksMutex.Unlock()

	l.mu.Lock()

	go func() {
		time.Sleep(l.maxLockTime)

		l.openLocksMutex.Lock()
		_, exists := l.openLocks[counter]
		if exists {
			fmt.Printf("Lock held for too long, using id %v and counter %v\n", id, counter)
			delete(l.openLocks, counter)
			l.mu.Unlock()
		}
		l.openLocksMutex.Unlock()
	}()

	return counter
}

// Unlock will unlock the lock. The id is so devs can easily figure out which
// caller is causing problems. The counter is important for knowing which
// instance was holding the lock.
func (l *Lock) Unlock(id string, counter int) {
	l.openLocksMutex.Lock()
	_, exists := l.openLocks[counter]
	if !exists {
		fmt.Printf("RUnlock called too late, using id %v and counter %v\n", id, counter)
	} else {
		delete(l.openLocks, counter)
		l.mu.Unlock()
	}
	l.openLocksMutex.Unlock()
}
