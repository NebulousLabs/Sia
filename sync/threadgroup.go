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
type ThreadGroup struct {
	stopChan chan struct{}
	chanOnce sync.Once
	wg       sync.WaitGroup
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

// IsStopped returns true if Stop has been called.
func (tg *ThreadGroup) IsStopped() bool {
	select {
	case <-tg.StopChan():
		return true
	default:
		return false
	}
}

// Add adds delta to the ThreadGroup counter.
func (tg *ThreadGroup) Add(delta int) error {
	if tg.IsStopped() {
		return ErrStopped
	}
	tg.wg.Add(delta)
	return nil
}

// Done decrements the ThreadGroup counter.
func (tg *ThreadGroup) Done() {
	tg.wg.Done()
}

// Stop closes the Threadgroup's stopChan and blocks until the counter is
// zero.
func (tg *ThreadGroup) Stop() error {
	if tg.IsStopped() {
		return ErrStopped
	}
	close(tg.stopChan)
	tg.wg.Wait()
	return nil
}
