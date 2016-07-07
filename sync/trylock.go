package sync

import (
	"errors"
	"sync"
)

var (
	// ErrTryLockFailed is returned if a call to TryLock is unsuccessful.
	ErrTryLockFailed = errors.New("lock could not be obtained")
)

// TryMutex provides a lock that you can 'peek' at, if the lock is potentially
// being held by another thread, the try function will return an error instead
// of blocking.
//
// TryMutex is meant to be used in scenarios where locks may be held for long
// periods of time.
type TryMutex struct {
	locked bool
	outer  sync.Mutex
	inner  sync.Mutex
}

// Lock grabs a lock on the TryMutex, blocking until the lock is obtained.
func (tm *TryMutex) Lock() {
	tm.outer.Lock()
	defer tm.outer.Unlock()

	// The inner lock needs to be grabbed before 'tm.locked' is set to true.
	tm.inner.Lock()
	if tm.locked {
		panic("bug in TryLock")
	}
	tm.locked = true
}

// TryLock grabs a lock on the TryMutex, returning an error if the mutex is
// already locked.
func (tm *TryMutex) TryLock() error {
	tm.outer.Lock()
	defer tm.outer.Unlock()

	if tm.locked {
		return ErrTryLockFailed
	}
	tm.inner.Lock()
	tm.locked = true
	return nil
}

// Unlock releases a lock on the TryMutex.
func (tm *TryMutex) Unlock() {
	tm.outer.Lock()
	defer tm.outer.Unlock()

	if !tm.locked {
		panic("bug in TryLock")
	}
	tm.locked = false
	tm.inner.Unlock()
}
