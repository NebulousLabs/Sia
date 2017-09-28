package sync

import (
	"testing"
	"time"
)

func cancelAfter(d time.Duration) <-chan struct{} {
	c := make(chan struct{})
	time.AfterFunc(d, func() { close(c) })
	return c
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(10)

	// request 1
	l.Request(1, nil)
	l.Release(1)

	// request limit
	l.Request(10, nil)
	l.Release(10)

	// request more than limit
	l.Request(11, nil)
	l.Release(11)

	// request multiple
	l.Request(5, nil)
	l.Request(3, nil)
	l.Request(2, nil)
	l.Release(10)

	// release multiple
	l.Request(10, nil)
	l.Release(5)
	l.Release(3)
	l.Release(2)

	// when all units have been requested, Request should block
	l.Request(10, nil)
	cancelled := l.Request(1, cancelAfter(10*time.Millisecond))
	if !cancelled {
		t.Fatal("expected Request to be cancelled")
	}
	l.Release(10)

	// when a unit is returned, a pending call to Request should be woken up
	l.Request(10, nil)
	time.AfterFunc(10*time.Millisecond, func() { l.Release(1) })
	cancelled = l.Request(1, cancelAfter(100*time.Millisecond))
	if cancelled {
		t.Fatal("expected Request to succeed")
	}
	time.Sleep(10 * time.Millisecond)
	l.Release(10)

	// requesting more than the limit should succeed once l.current == 0
	l.Request(1, nil)
	time.AfterFunc(10*time.Millisecond, func() { l.Release(1) })
	cancelled = l.Request(12, cancelAfter(100*time.Millisecond))
	if cancelled {
		t.Fatal("expected Request to succeed")
	}
	// after more than the limit has been requested, requests should not
	// succeed until l.current falls below l.limit again
	cancelled = l.Request(2, cancelAfter(10*time.Millisecond))
	if !cancelled {
		t.Fatal("expected Request to be cancelled")
	}
	l.Release(2)
	cancelled = l.Request(2, cancelAfter(10*time.Millisecond))
	if !cancelled {
		t.Fatal("expected Request to be cancelled")
	}
	l.Release(2)
	cancelled = l.Request(2, nil)
	if cancelled {
		t.Fatal("expected Request to succeed")
	}
	l.Release(10)

	// calling SetLimit between a Request/Release should not cause problems
	l.Request(10, nil)
	l.SetLimit(5)
	l.Release(10)
	// limit should now be 5
	l.Request(1, nil)
	cancelled = l.Request(5, cancelAfter(10*time.Millisecond))
	if !cancelled {
		t.Fatal("expected Request to be cancelled")
	}
	l.Release(1)

	// setting a higher limit should wake up blocked requests
	l.Request(5, nil)
	time.AfterFunc(10*time.Millisecond, func() { l.SetLimit(10) })
	cancelled = l.Request(5, cancelAfter(100*time.Millisecond))
	if cancelled {
		t.Fatal("expected Request to succeed")
	}
}
