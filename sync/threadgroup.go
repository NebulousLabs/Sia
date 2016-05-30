package sync

import (
	"errors"
	"io"
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
// During shutdown, it is common to close resources such as net.Conns and
// net.Listeners. Typically, this would require spawning a goroutine to wait
// on the ThreadGroup's StopChan and then close the resource. To make this
// more convenient, ThreadGroup provides a RegisterCloser method. io.Closers
// registered in this manner will be automatically closed when Stop is called.
type ThreadGroup struct {
	stopChan chan struct{}
	chanOnce sync.Once
	mu       sync.Mutex
	wg       sync.WaitGroup
	closers  []io.Closer
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

// Add increments the ThreadGroup counter.
func (tg *ThreadGroup) Add() error {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if tg.IsStopped() {
		return ErrStopped
	}
	tg.wg.Add(1)
	return nil
}

// Done decrements the ThreadGroup counter.
func (tg *ThreadGroup) Done() {
	tg.wg.Done()
}

// Stop closes the ThreadGroup's stopChan, closes members of the closer set,
// and blocks until the counter is zero.
func (tg *ThreadGroup) Stop() error {
	tg.mu.Lock()
	if tg.IsStopped() {
		tg.mu.Unlock()
		return ErrStopped
	}
	close(tg.stopChan)
	for _, c := range tg.closers {
		c.Close()
	}
	tg.mu.Unlock()
	tg.wg.Wait()
	return nil
}

// RegisterCloser adds an io.Closer to the ThreadGroups closer set. Members of
// the set will be closed when Stop is called. Note that this means the errors
// returned by Close are not recoverable.
func (tg *ThreadGroup) RegisterCloser(c io.Closer) error {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if tg.IsStopped() {
		return ErrStopped
	}
	tg.closers = append(tg.closers, c)
	return nil
}
