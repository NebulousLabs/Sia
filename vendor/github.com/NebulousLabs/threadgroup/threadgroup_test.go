package threadgroup

import (
	"net"
	"sync"
	"testing"
	"time"
)

// TestThreadGroupStopEarly tests that a thread group can correctly interrupt
// an ongoing process.
func TestThreadGroupStopEarly(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

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
	} else if elapsed > 500*time.Millisecond {
		t.Fatal("Stop did not interrupt goroutines")
	}
}

// TestThreadGroupWait tests that a thread group will correctly wait for
// existing processes to halt.
func TestThreadGroupWait(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	var tg ThreadGroup
	for i := 0; i < 10; i++ {
		err := tg.Add()
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			defer tg.Done()
			time.Sleep(time.Second)
		}()
	}
	start := time.Now()
	err := tg.Stop()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	} else if elapsed < time.Millisecond*950 {
		t.Fatal("Stop did not wait for goroutines:", elapsed)
	}
}

// TestThreadGroupStop tests the behavior of a ThreadGroup after Stop has been
// called.
func TestThreadGroupStop(t *testing.T) {
	// Create a thread group and stop it.
	var tg ThreadGroup
	// Create an array to track the order of execution for OnStop and AfterStop
	// calls.
	var stopCalls []int

	// isStopped should return false
	if tg.isStopped() {
		t.Error("isStopped returns true on unstopped ThreadGroup")
	}
	// The cannel provided by StopChan should be open.
	select {
	case <-tg.StopChan():
		t.Error("stop chan appears to be closed")
	default:
	}

	// OnStop and AfterStop should queue their functions, but not call them.
	// 'Add' and 'Done' are setup around the OnStop functions, to make sure
	// that the OnStop functions are called before waiting for all calls to
	// 'Done' to come through.
	//
	// Note: the practice of calling Add outside of OnStop and Done inside of
	// OnStop is a bad one - any call to tg.Flush() will cause a deadlock
	// because the stop functions will not be called but tg.Flush will be
	// waiting for the thread group counter to reach zero.
	err := tg.Add()
	if err != nil {
		t.Fatal(err)
	}
	err = tg.Add()
	if err != nil {
		t.Fatal(err)
	}
	err = tg.OnStop(func() error {
		tg.Done()
		stopCalls = append(stopCalls, 1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = tg.OnStop(func() error {
		tg.Done()
		stopCalls = append(stopCalls, 2)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = tg.AfterStop(func() error {
		stopCalls = append(stopCalls, 10)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = tg.AfterStop(func() error {
		stopCalls = append(stopCalls, 20)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// None of the stop calls should have been called yet.
	if len(stopCalls) != 0 {
		t.Fatal("Stop calls were called too early")
	}

	// Stop the thread group.
	err = tg.Stop()
	if err != nil {
		t.Fatal(err)
	}
	// isStopped should return true.
	if !tg.isStopped() {
		t.Error("isStopped returns false on stopped ThreadGroup")
	}
	// The cannel provided by StopChan should be closed.
	select {
	case <-tg.StopChan():
	default:
		t.Error("stop chan appears to be closed")
	}
	// The OnStop calls should have been called first, in reverse order, and
	// the AfterStop calls should have been called second, in reverse order.
	if len(stopCalls) != 4 {
		t.Fatal("Stop did not call the stopping functions correctly")
	}
	if stopCalls[0] != 2 {
		t.Error("Stop called the stopping functions in the wrong order")
	}
	if stopCalls[1] != 1 {
		t.Error("Stop called the stopping functions in the wrong order")
	}
	if stopCalls[2] != 20 {
		t.Error("Stop called the stopping functions in the wrong order")
	}
	if stopCalls[3] != 10 {
		t.Error("Stop called the stopping functions in the wrong order")
	}

	// Add and Stop should return errors.
	err = tg.Add()
	if err != ErrStopped {
		t.Error("expected ErrStopped, got", err)
	}
	err = tg.Stop()
	if err != ErrStopped {
		t.Error("expected ErrStopped, got", err)
	}

	// OnStop and AfterStop should call their functions immediately now that
	// the thread group has stopped.
	onStopCalled := false
	err = tg.OnStop(func() error {
		onStopCalled = true
		return nil
	})
	if err == nil {
		t.Fatal("OnStop should return an error after being called after stop")
	}

	if !onStopCalled {
		t.Error("OnStop function not called immediately despite the thread group being closed already.")
	}
	afterStopCalled := false
	err = tg.AfterStop(func() error {
		afterStopCalled = true
		return nil
	})
	if err == nil {
		t.Fatal("AfterStop should return an error after being called after stop")
	}
	if !afterStopCalled {
		t.Error("AfterStop function not called immediately despite the thread group being closed already.")
	}
}

// TestThreadGroupConcurrentAdd tests that Add can be called concurrently with Stop.
func TestThreadGroupConcurrentAdd(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	var tg ThreadGroup
	for i := 0; i < 1000; i++ {
		go func() {
			err := tg.Add()
			if err != nil {
				return
			}
			defer tg.Done()

			select {
			case <-time.After(100 * time.Millisecond):
			case <-tg.StopChan():
			}
		}()
	}
	time.Sleep(25 * time.Millisecond)
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
	tg.isStopped()
	if tg.stopChan == nil {
		t.Error("stopChan should have been initialized by isStopped")
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

// TestThreadGroupOnStop tests that Stop calls functions registered with
// OnStop.
func TestThreadGroupOnStop(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	// create ThreadGroup and register the closer
	var tg ThreadGroup
	err = tg.OnStop(func() error { return l.Close() })
	if err != nil {
		t.Fatal(err)
	}

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

// TestThreadGroupCloseAfterStop checks that an AfterStop function is
// correctly called after the thread is stopped.
func TestThreadGroupClosedAfterStop(t *testing.T) {
	var tg ThreadGroup
	var closed bool
	err := tg.AfterStop(func() error { closed = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
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
	err = tg.AfterStop(func() error { closed = true; return nil })
	if err == nil {
		t.Fatal("AfterStop should return an error after stop")
	}
	if !closed {
		t.Fatal("close function should have been called immediately")
	}
}

// TestThreadGroupNetworkExample tries to use a thread group as it might be
// expected to be used by a networking module.
func TestThreadGroupNetworkExample(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	var tg ThreadGroup

	// Open a listener, and queue the shutdown.
	listenerCleanedUp := false
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	// Open a thread to accept calls from the listener.
	handlerFinishedChan := make(chan struct{})
	go func() {
		// Threadgroup shutdown should stall until listener closes.
		err := tg.Add()
		if err != nil {
			// Testing is non-deterministic, sometimes Stop() will be called
			// before the listener fully starts up.
			close(handlerFinishedChan)
			return
		}
		defer tg.Done()

		for {
			_, err := listener.Accept()
			if err != nil {
				break
			}
		}
		close(handlerFinishedChan)
	}()
	err = tg.OnStop(func() error {
		err := listener.Close()
		if err != nil {
			return err
		}
		<-handlerFinishedChan

		listenerCleanedUp = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a thread that does some stuff which takes time, and then closes.
	threadFinished := false
	err = tg.Add()
	if err != nil {
		t.Fatal(err)
	}
	go func() error {
		time.Sleep(time.Second)
		threadFinished = true
		tg.Done()
		return nil
	}()

	// Create a thread that does some stuff which takes time, and then closes.
	// Use Stop to wait for the threead to finish and then check that all
	// resources have closed.
	threadFinished2 := false
	err = tg.Add()
	if err != nil {
		t.Fatal(err)
	}
	go func() error {
		time.Sleep(time.Second)
		threadFinished2 = true
		tg.Done()
		return nil
	}()

	// Let the listener run for a bit.
	time.Sleep(100 * time.Millisecond)
	err = tg.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if !threadFinished2 || !listenerCleanedUp {
		t.Error("stop did not block until all running resources had closed")
	}
}

// TestNestedAdd will call Add repeatedly from the same goroutine, then call
// stop concurrently.
func TestNestedAdd(t *testing.T) {
	var tg ThreadGroup
	go func() {
		for i := 0; i < 1000; i++ {
			err := tg.Add()
			if err == nil {
				defer tg.Done()
			}
		}
	}()

	time.Sleep(10 * time.Millisecond)
	tg.Stop()
}

// TestAddOnStop checks that you can safely call OnStop from under the
// protection of an Add call.
func TestAddOnStop(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	var tg ThreadGroup
	var data int
	addChan := make(chan struct{})
	stopChan := make(chan struct{})
	err := tg.OnStop(func() error {
		close(stopChan)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		err := tg.Add()
		if err != nil {
			t.Fatal(err)
		}
		close(addChan)

		// Wait for the call to 'Stop' to be called in the parent thread, and
		// then queue a bunch of 'OnStop' and 'AfterStop' functions before
		// calling 'Done'.
		<-stopChan
		for i := 0; i < 10; i++ {
			err = tg.OnStop(func() error {
				data++
				return nil
			})
			if err == nil {
				t.Fatal("OnStop should return an error when being called after stop")
			}
			err = tg.AfterStop(func() error {
				data++
				return nil
			})
			if err == nil {
				t.Fatal("AfterStop should return an error when being called after stop")
			}
		}
		tg.Done()
	}()

	// Wait for 'Add' to be called in the above thread, to guarantee that
	// OnStop and AfterStop will be called after 'Add' and 'Stop' have been
	// called together.
	<-addChan
	err = tg.Stop()
	if err != nil {
		t.Fatal(err)
	}

	if data != 20 {
		t.Error("20 calls were made to increment data, but value is", data)
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
