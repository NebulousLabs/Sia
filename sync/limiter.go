package sync

import (
	"sync"
)

// A Limiter restricts access to a resource.
//
// Units of the resource are reserved via Request and returned via Release.
// Conventionally, a caller who reserves n units is responsible for ensuring
// that all n are eventually returned. Once the number of reserved units
// exceeds the Limiters limit, further calls to Request will block until
// sufficient units are returned via Release.
//
// This Limiter differs from others in that it allows requesting more than the
// limit. This request is only fulfilled once all other units have been
// returned. Once the request is fulfilled, calls to Request will block until
// enough units have been returned to bring the total outlay below the limit.
// This design choice prevents any call to Request from blocking forever,
// striking a balance between precise resource management and flexibility.
type Limiter struct {
	limit   int
	current int
	mu      chan struct{} // can't select on sync.Mutex
	cond    *sync.Cond
}

// Request blocks until n units are available. If n is greater than m's limit,
// Request blocks until all of m's units have been released.
//
// Request is unbiased with respect to n: calls with small n do not starve
// calls with large n.
//
// Request returns true if the request was canceled, and false otherwise.
func (l *Limiter) Request(n int, cancel <-chan struct{}) bool {
	// acquire mutex
	select {
	case <-cancel:
		return true
	case lock := <-l.mu:
		// unlock
		defer func() { l.mu <- lock }()
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
		l.cond.L.Unlock()
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
		limit: limit,
		mu:    make(chan struct{}, 1),
		cond:  sync.NewCond(new(sync.Mutex)),
	}
	l.mu <- struct{}{}
	return l
}
