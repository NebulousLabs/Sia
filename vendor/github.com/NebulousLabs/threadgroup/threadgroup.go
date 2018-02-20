// Package threadgroup provides a utility for performing organized clean
// shutdown and quick shutdown of long running groups of threads, such as
// networking threads, background threads, or resources like databases.
//
// The OnStop and AfterStop functions are helpers which enable shutdown code to
// be inlined with resource allocation, similar to defer. The difference is that
// `OnStop` and `AfterStop` will be called following tg.Stop, instead of when
// the parent function goes out of scope.
package threadgroup

import (
	"sync"

	"github.com/NebulousLabs/errors"
)

// ErrStopped is returned by ThreadGroup methods if Stop has already been
// called.
var ErrStopped = errors.New("ThreadGroup already stopped")

// A ThreadGroup is a one-time-use object to manage the life cycle of a group
// of threads. It is a sync.WaitGroup that provides functions for coordinating
// actions and shutting down threads. After Stop() is called, the thread group
// is no longer useful.
//
// It is safe to call Add(), Done(), and Stop() concurrently.
type ThreadGroup struct {
	onStopFns    []func() error
	afterStopFns []func() error

	once     sync.Once
	stopChan chan struct{}
	bmu      sync.Mutex // Protects 'Add' and 'Wait'.
	mu       sync.Mutex // Protects the 'onStopFns' and 'afterStopFns' variable
	wg       sync.WaitGroup
}

// init creates the stop channel for the thread group.
func (tg *ThreadGroup) init() {
	tg.stopChan = make(chan struct{})
}

// isStopped will return true if Stop() has been called on the thread group.
func (tg *ThreadGroup) isStopped() bool {
	tg.once.Do(tg.init)
	select {
	case <-tg.stopChan:
		return true
	default:
		return false
	}
}

// Add increments the thread group counter.
func (tg *ThreadGroup) Add() error {
	tg.bmu.Lock()
	defer tg.bmu.Unlock()

	if tg.isStopped() {
		return ErrStopped
	}
	tg.wg.Add(1)
	return nil
}

// AfterStop ensures that a function will be called after Stop() has been called
// and after all running routines have called Done(). The functions will be
// called in reverse order to how they were added, similar to defer. If Stop()
// has already been called, the input function will be called immediately, and a
// composition of ErrStopped and the error from calling fn will be returned.
//
// The primary use of AfterStop is to allow code that opens and closes
// resources to be positioned next to each other. The purpose is similar to
// `defer`, except for resources that outlive the function which creates them.
func (tg *ThreadGroup) AfterStop(fn func() error) error {
	tg.mu.Lock()
	if tg.isStopped() {
		tg.mu.Unlock()
		return errors.Compose(ErrStopped, fn())
	}
	tg.afterStopFns = append(tg.afterStopFns, fn)
	tg.mu.Unlock()
	return nil
}

// OnStop ensures that a function will be called after Stop() has been called,
// and before blocking until all running routines have called Done(). It is safe
// to use OnStop to coordinate the closing of long-running threads. The OnStop
// functions will be called in the reverse order in which they were added,
// similar to defer. If Stop() has already been called, the input function will
// be called immediately, and a composition of ErrStopped and the error from
// calling fn will be returned.
func (tg *ThreadGroup) OnStop(fn func() error) error {
	tg.mu.Lock()
	if tg.isStopped() {
		tg.mu.Unlock()
		return errors.Compose(ErrStopped, fn())
	}
	tg.onStopFns = append(tg.onStopFns, fn)
	tg.mu.Unlock()
	return nil
}

// Done decrements the thread group counter.
func (tg *ThreadGroup) Done() {
	tg.wg.Done()
}

// Stop will close the stop channel of the thread group, then call all 'OnStop'
// functions in reverse order, then will wait until the thread group counter
// reaches zero, then will call all of the 'AfterStop' functions in reverse
// order.
//
// The errors returned by the OnStop and AfterStop functions will be composed
// into a single error.
func (tg *ThreadGroup) Stop() error {
	// Signal that the threadgroup is shutting down.
	if tg.isStopped() {
		return ErrStopped
	}
	tg.bmu.Lock()
	close(tg.stopChan)
	tg.bmu.Unlock()

	// Flush any function that made it past isStopped and might be trying to do
	// something under the mu lock. Any calls to OnStop or AfterStop after this
	// will fail, because isStopped will cut them short.
	tg.mu.Lock()
	tg.mu.Unlock()

	// Run all of the OnStop functions, in reverse order of how they were added.
	var err error
	for i := len(tg.onStopFns) - 1; i >= 0; i-- {
		err = errors.Extend(err, tg.onStopFns[i]())
	}

	// Wait for all running processes to signal completion.
	tg.wg.Wait()

	// Run all of the AfterStop functions, in reverse order of how they were
	// added.
	for i := len(tg.afterStopFns) - 1; i >= 0; i-- {
		err = errors.Extend(err, tg.afterStopFns[i]())
	}
	return err
}

// StopChan provides read-only access to the ThreadGroup's stopChan. Callers
// should select on StopChan in order to interrupt long-running reads (such as
// time.After).
func (tg *ThreadGroup) StopChan() <-chan struct{} {
	tg.once.Do(tg.init)
	return tg.stopChan
}
