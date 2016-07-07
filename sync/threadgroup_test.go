package sync

import (
	"net"
	"sync"
	"testing"
	"time"
)

// TestThreadGroup tests normal operation of a ThreadGroup.
func TestThreadGroup(t *testing.T) {
	var tg ThreadGroup
	for i := 0; i < 10; i++ {
		err := tg.Add()
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			defer tg.Done()
			select {
			case <-time.After(1 * time.Second):
			case <-tg.StopChan():
			}
		}()
	}
	start := time.Now()
	err := tg.Stop()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	} else if elapsed > 100*time.Millisecond {
		t.Fatal("Stop did not interrupt goroutines")
	}
}

// TestThreadGroupStop tests the behavior of a ThreadGroup after Stop has been
// called.
func TestThreadGroupStop(t *testing.T) {
	// Create a thread group and stop it.
	var tg ThreadGroup
	err := tg.Stop()
	if err != nil {
		t.Fatal(err)
	}

	// Add and Stop should return errors
	err = tg.Add()
	if err != ErrStopped {
		t.Error("expected ErrStopped, got", err)
	}
	err = tg.Stop()
	if err != ErrStopped {
		t.Error("expected ErrStopped, got", err)
	}
}

// TestThreadGroupConcurrentAdd tests that Add can be called concurrently with Stop.
func TestThreadGroupConcurrentAdd(t *testing.T) {
	var tg ThreadGroup
	for i := 0; i < 10; i++ {
		go func() {
			err := tg.Add()
			if err != nil {
				return
			}
			defer tg.Done()

			select {
			case <-time.After(1 * time.Second):
			case <-tg.StopChan():
			}
		}()
	}
	time.Sleep(10 * time.Millisecond) // wait for at least one Add
	err := tg.Stop()
	if err != nil {
		t.Fatal(err)
	}
}

// TestThreadGroupOnce tests that a zero-valued ThreadGroup's stopChan is
// properly initialized.
func TestThreadGroupOnce(t *testing.T) {
	tg := new(ThreadGroup)
	if tg.stopChan != nil {
		t.Error("expected nil stopChan")
	}

	// these methods should cause stopChan to be initialized
	tg.StopChan()
	if tg.stopChan == nil {
		t.Error("stopChan should have been initialized by StopChan")
	}

	tg = new(ThreadGroup)
	tg.Add()
	if tg.stopChan == nil {
		t.Error("stopChan should have been initialized by Add")
	}

	tg = new(ThreadGroup)
	tg.Stop()
	if tg.stopChan == nil {
		t.Error("stopChan should have been initialized by Stop")
	}
}

// TestThreadGroupBeforeStop tests that Stop calls functions registered with
// BeforeStop.
func TestThreadGroupBeforeStop(t *testing.T) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	// create ThreadGroup and register the closer
	var tg ThreadGroup
	tg.BeforeStop(func() { l.Close() })

	// send on channel when listener is closed
	var closed bool
	tg.Add()
	go func() {
		defer tg.Done()
		_, err := l.Accept()
		closed = err != nil
	}()

	tg.Stop()
	if !closed {
		t.Fatal("Stop did not close listener")
	}
}

// TestThreadGroupRace tests that calling ThreadGroup methods concurrently
// does not trigger the race detector.
func TestThreadGroupRace(t *testing.T) {
	var tg ThreadGroup
	go tg.StopChan()
	go func() {
		if tg.Add() == nil {
			tg.Done()
		}
	}()
	err := tg.Stop()
	if err != nil {
		t.Fatal(err)
	}
}

// TestThreadedGroupCloseAfterStop checks that an AfterStop function is
// correctly called after the thread is stopped.
func TestThreadGroupClosedAfterStop(t *testing.T) {
	var tg ThreadGroup
	var closed bool
	tg.AfterStop(func() { closed = true })
	if closed {
		t.Fatal("close function should not have been called yet")
	}
	if err := tg.Stop(); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("close function should have been called")
	}

	// Stop has already been called, so the close function should be called
	// immediately
	closed = false
	tg.AfterStop(func() { closed = true })
	if !closed {
		t.Fatal("close function should have been called immediately")
	}
}

// BenchmarkThreadGroup times how long it takes to add a ton of threads and
// trigger goroutines that call Done.
func BenchmarkThreadGroup(b *testing.B) {
	var tg ThreadGroup
	for i := 0; i < b.N; i++ {
		tg.Add()
		go tg.Done()
	}
	tg.Stop()
}

// BenchmarkWaitGroup times how long it takes to add a ton of threads to a wait
// group and trigger goroutines that call Done.
func BenchmarkWaitGroup(b *testing.B) {
	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go wg.Done()
	}
	wg.Wait()
}
