package demotemutex

import (
	"testing"
)

// TestDemotion checks that a lock can be correctly demoted.
func TestDemotion(t *testing.T) {
	value := 0
	var dm DemoteMutex
	c := make(chan int)

	// Lock the mutex and update the value.
	dm.Lock()
	value++ // should set value to 1.

	// Spawn a thread that will lock the value and update it.
	go func() {
		dm.Lock()
		value++ // should set value to 2.
		c <- value
		dm.Unlock()
	}()

	// Spawn a thread that will read the value and send it down the channel.
	//
	// Ideally there's a way to guarantee that this thread runs after the other
	// goroutine, to guarantee that the writelock is trying to acquire the
	// mutex before the call to 'dm.RLock' is made. I could not find a way to
	// get that guarantee.
	go func() {
		dm.RLock()
		c <- value
		dm.RUnlock()
	}()

	// Demote the lock, which will allow all threads attempting to aquire a
	// readlock access to the lock without permitting any threads blocking for
	// access to a writelock access to the lock.
	//
	// The thread blocking for a readlock should be able to aquire the lock and
	// send the value '1' down the channel. It should not be blocked by the
	// thread waiting for a writelock.
	dm.Demote()
	v := <-c
	if v != 1 {
		t.Fatal("demoting lock failed - value is unexpected")
	}
	// Allow threads blocking for a writelock access to the lock.
	dm.DemotedUnlock()

	// Pull a second value out of 'c' - it should be the value provided from
	// the thread that writelocks and then increments to 2.
	v = <-c
	if v != 2 {
		t.Fatal("value was not incremented to 2")
	}
}
