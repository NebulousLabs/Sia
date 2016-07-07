package sync

import (
	"errors"
	"sync"
)

// ErrStopped is returned by ThreadGroup methods if Stop has already been
// called.
var ErrStopped = errors.New("ThreadGroup already stopped")

// ThreadGroup is a sync.WaitGroup with additional functionality for
// facilitating clean shutdown. Namely, it provides a StopChan method for
// notifying callers when shutdown occurrs. Another key difference is that a
// ThreadGroup is only intended be used once; as such, its Add and Stop
// methods return errors if Stop has already been called.
//
// During shutdown, it is common to close resources such as net.Listeners.
// Typically, this would require spawning a goroutine to wait on the
// ThreadGroup's StopChan and then close the resource. To make this more
// convenient, ThreadGroup provides an OnStop method. Functions passed to
// OnStop will be called automatically when Stop is called.
type ThreadGroup struct {
	beforeStopFns []func()
	stopFns       []func()

	chanOnce sync.Once
	mu       sync.Mutex
	stopChan chan struct{}
	wg       sync.WaitGroup
	wgPerm   sync.WaitGroup
}

// isStopped will return true if the stopChan has been closed, indicating that
// Stop() has been called.
func (tg *ThreadGroup) isStopped() bool {
	select {
	case <-tg.StopChan():
		return true
	default:
		return false
	}
}

// Add increments the ThreadGroup counter.
func (tg *ThreadGroup) Add() error {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if tg.isStopped() {
		return ErrStopped
	}
	tg.wg.Add(1)
	return nil
}

// AddPermanent increments the ThreadGroup counter. Unlike Add, the thread
// group will not wait for a call to 'Done' from this thread group during a
// pause.
func (tg *ThreadGroup) AddPermanent() error {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if tg.isStopped() {
		return ErrStopped
	}
	tg.wgPerm.Add(1)
	return nil
}

// AfterStop adds a function to the ThreadGroup's stopFns set. Members of the
// set will be called when Stop is called, in reverse order. If the ThreadGroup
// is already stopped, the function will be called immediately.
func (tg *ThreadGroup) AfterStop(fn func()) {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if tg.isStopped() {
		fn()
		return
	}
	tg.stopFns = append(tg.stopFns, fn)
}

// BeforeStop will call a function during the 'Stop' call, but before waiting
// for all other threads to complete.
func (tg *ThreadGroup) BeforeStop(fn func()) {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if tg.isStopped() {
		fn()
		return
	}
	tg.beforeStopFns = append(tg.beforeStopFns, fn)
}

// Done decrements the ThreadGroup counter.
func (tg *ThreadGroup) Done() {
	tg.wg.Done()
}

// DonePermanent decrements the ThreadGroup permanent counter.
func (tg *ThreadGroup) DonePermanent() {
	tg.wgPerm.Done()
}

// Pause will cause all operations to block until 'Resume' has been called.
func (tg *ThreadGroup) Pause() error {
	tg.mu.Lock()
	if tg.isStopped() {
		tg.mu.Unlock()
		return ErrStopped
	}

	// Block until all currently open processes have released control.
	tg.wg.Wait()
	return nil
}

// Resume will allow operations to resume following a call to 'Pause'.
func (tg *ThreadGroup) Resume() error {
	if tg.isStopped() {
		return ErrStopped
	}
	tg.mu.Unlock()
	return nil
}

// Stop closes the ThreadGroup's stopChan, closes members of the closer set,
// and blocks until the counter is zero.
func (tg *ThreadGroup) Stop() error {
	tg.mu.Lock()
	if tg.isStopped() {
		tg.mu.Unlock()
		return ErrStopped
	}
	close(tg.stopChan)
	for i := len(tg.beforeStopFns) - 1; i >= 0; i-- {
		tg.beforeStopFns[i]()
	}

	tg.wg.Wait()
	tg.wgPerm.Wait()

	// After waiting for all resources to release the thread group, iterate
	// through the stop functions and call them in reverse oreder.
	for i := len(tg.stopFns) - 1; i >= 0; i-- {
		tg.stopFns[i]()
	}
	tg.stopFns = nil
	tg.mu.Unlock()
	return nil
}

// StopChan provides read-only access to the ThreadGroup's stopChan. Callers
// should select on StopChan in order to interrupt long-running reads (such as
// time.After).
func (tg *ThreadGroup) StopChan() <-chan struct{} {
	// Initialize tg.stopChan if it is nil; this makes an unitialized
	// ThreadGroup valid. (Otherwise, a NewThreadGroup function would be
	// necessary.)
	tg.chanOnce.Do(func() { tg.stopChan = make(chan struct{}) })
	return tg.stopChan
}
