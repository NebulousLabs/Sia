package sync

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// TestThreadGroupStopEarly tests that a thread group can correctly interrupt
// an ongoing process.
func TestThreadGroupStopEarly(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

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

// TestThreadGroupWait tests that a thread group will correctly wait for
// existing processes to halt.
func TestThreadGroupWait(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

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
	} else if elapsed < time.Second {
		t.Fatal("Stop did not wait for goroutines")
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
	tg.OnStop(func() {
		tg.Done()
		stopCalls = append(stopCalls, 1)
	})
	tg.OnStop(func() {
		tg.Done()
		stopCalls = append(stopCalls, 2)
	})
	tg.AfterStop(func() {
		stopCalls = append(stopCalls, 10)
	})
	tg.AfterStop(func() {
		stopCalls = append(stopCalls, 20)
	})
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
	tg.OnStop(func() {
		onStopCalled = true
	})
	if !onStopCalled {
		t.Error("OnStop function not called immediately despite the thread group being closed already.")
	}
	afterStopCalled := false
	tg.AfterStop(func() {
		afterStopCalled = true
	})
	if !afterStopCalled {
		t.Error("AfterStop function not called immediately despite the thread group being closed already.")
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
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	// create ThreadGroup and register the closer
	var tg ThreadGroup
	tg.OnStop(func() { l.Close() })

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

// TestThreadGroupSiaExample tries to use a thread group as it might be
// expected to be used by a module of Sia.
func TestThreadGroupSiaExample(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	testDir := build.TempDir("sync", "TestThreadGroupSiaExample")
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	var tg ThreadGroup

	// Open an example file. The file is expected to be used throughout the
	// lifetime of the module, and should not be closed until 'AfterStop' is
	// called.
	fileClosed := false
	file, err := os.Create(filepath.Join(testDir, "exampleFile.txt"))
	if err != nil {
		t.Fatal(err)
	}
	tg.AfterStop(func() {
		fileClosed = true
		err := file.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	// Open a listener. The listener and handler thread should be closed before
	// the file is closed.
	listenerCleanedUp := false
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	// Open a thread to accept calls from the listener.
	handlerFinishedChan := make(chan struct{})
	go func() {
		for {
			_, err := listener.Accept()
			if err != nil {
				break
			}
		}
		handlerFinishedChan <- struct{}{}
	}()
	tg.OnStop(func() {
		err := listener.Close()
		if err != nil {
			t.Fatal(err)
		}
		<-handlerFinishedChan

		if fileClosed {
			t.Error("file should be open while the listener is shutting down")
		}
		listenerCleanedUp = true
	})

	// Create a thread that does some stuff which takes time, and then closes.
	// Use Flush to clear out the process without closing the resources.
	threadFinished := false
	err = tg.Add()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(time.Second)
		threadFinished = true
		tg.Done()
	}()
	tg.Flush()
	if !threadFinished {
		t.Error("call to Flush should have allowed the working thread to finish")
	}
	if listenerCleanedUp || fileClosed {
		t.Error("call to Flush resulted in permanent resources being closed")
	}

	// Create a thread that does some stuff which takes time, and then closes.
	// Use Stop to wait for the threead to finish and then check that all
	// resources have closed.
	threadFinished2 := false
	err = tg.Add()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(time.Second)
		threadFinished2 = true
		tg.Done()
	}()
	tg.Stop()
	if !threadFinished || !listenerCleanedUp || !fileClosed {
		t.Error("stop did not block until all running resources had closed")
	}
}

// TestAddOnStop checks that you can safely call OnStop from under the
// protection of an Add call.
func TestAddOnStop(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	var tg ThreadGroup
	var data int
	addChan := make(chan struct{})
	stopChan := make(chan struct{})
	tg.OnStop(func() {
		close(stopChan)
	})
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
			tg.OnStop(func() {
				data++
			})
			tg.AfterStop(func() {
				data++
			})
		}
		tg.Done()
	}()

	// Wait for 'Add' to be called in the above thread, to guarantee that
	// OnStop and AfterStop will be called after 'Add' and 'Stop' have been
	// called together.
	<-addChan
	err := tg.Stop()
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
