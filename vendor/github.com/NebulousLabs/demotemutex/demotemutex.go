// Package demotemutex provides an extention to sync.Mutex that allows a
// writelock to be demoted to a readlock without releasing control to other
// writelocks.
package demotemutex

import (
	"sync"
)

// DemoteMutex is a DemoteMutex that allows 'demotion' - a writelock will demote
// such that readlocks are able to acquite the mutex, but other writelocks are
// still blocked.
type DemoteMutex struct {
	// Readlocks need only acquire the inner mutex. Writelocks must first
	// acquire the outer mutex and then the inner mutex. This means that only
	// one writelock has access to the inner mutex at a time. Upon being
	// demoted, a writelock releases the inner mutex, which allows readlocks to
	// acquire the mutex. The outer lock remains locked, preventing other lock
	// calls from blocking access to the inner mutex.
	outer sync.Mutex
	inner sync.RWMutex
}

// Demote will modify a writelocked DemoteMutex such that readlocks can acquire
// the mutex but writelocks are still blocked. After being demoted, the
// DemoteMutex must be unlocked by calling 'DemotedUnlock'.
func (dm *DemoteMutex) Demote() {
	// The inner mutex can be unlocked. Because writelocks must acquire the
	// outer lock (which has not been released), the inner lock does not need
	// to be readlocked to maintain safety.
	dm.inner.Unlock()
}

// DemotedUnlock will fully unlock a lock that has been demoted.
func (dm *DemoteMutex) DemotedUnlock() {
	dm.outer.Unlock()
}

// RLock will grab a readlock on the DemoteMutex. RLock can share the mutex with
// demoted locks.
func (dm *DemoteMutex) RLock() {
	dm.inner.RLock()
}

// RUnlock will release a readlock on the DemoteMutex.
func (dm *DemoteMutex) RUnlock() {
	dm.inner.RUnlock()
}

// Lock will grab a writelock on a DemoteMutex. A writelocked DemoteMutex can be
// demoted such that readlocks can aquire the mutex but other writelocks are
// still blocked.
func (dm *DemoteMutex) Lock() {
	// The outer lock must be acquired first, and then the inner lock acquired
	// second. This ensures that only one writelock has access to the inner
	// lock at a time.
	dm.outer.Lock()
	dm.inner.Lock()
}

// Unlock will fully unlock a DemoteMutex.
func (dm *DemoteMutex) Unlock() {
	dm.inner.Unlock()
	dm.outer.Unlock()
}
