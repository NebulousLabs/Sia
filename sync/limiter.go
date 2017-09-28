package sync

import (
	"sync"
)

// A Limiter restricts access to a resource.
type Limiter struct {
	limit    int
	current  int
	requests chan struct{}
	cond     *sync.Cond
}

// Request blocks until n units are available. If n is greater than m's limit,
// Request blocks until all of m's units have been released.
//
// Request is unbiased with respect to n: calls with small n do not starve
// calls with large n.
//
// Request returns true if the request was canceled, and false otherwise.
func (l *Limiter) Request(n int, cancel <-chan struct{}) bool {
	// wait until our request is "first in line"
	select {
	case <-cancel:
		return true
	case token := <-l.requests:
		// return token when we're done
		defer func() { l.requests <- token }()
	}

	// spawn goroutine to handle cancellation
	var cancelled bool
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-cancel:
			l.cond.L.Lock()
			cancelled = true
			l.cond.L.Unlock()
			l.cond.Signal()
		case <-done:
		}
	}()

	// wait until request can be satisfied
	l.cond.L.Lock()
	for l.current+n > l.limit && l.current != 0 && !cancelled {
		l.cond.Wait()
	}
	defer l.cond.L.Unlock()
	if !cancelled {
		l.current += n
	}
	return cancelled
}

// Release returns n units to l, making them available to future callers. It
// is legal for n to be larger than l's limit, as long as n was previously
// passed to Request.
func (l *Limiter) Release(n int) {
	l.cond.L.Lock()
	l.current -= n
	if l.current < 0 {
		panic("units released exceeds units requested")
	}
	l.cond.L.Unlock()
	l.cond.Signal()
}

// SetLimit sets the limit of l. It is legal to interpose calls to SetLimit
// between Request/Release pairs.
func (l *Limiter) SetLimit(limit int) {
	l.cond.L.Lock()
	l.limit = limit
	l.cond.L.Unlock()
	l.cond.Signal()
}

// NewLimiter returns a Limiter with the supplied limit.
func NewLimiter(limit int) *Limiter {
	l := &Limiter{
		limit:    limit,
		requests: make(chan struct{}, 1),
		cond:     sync.NewCond(new(sync.Mutex)),
	}
	l.requests <- struct{}{}
	return l
}
