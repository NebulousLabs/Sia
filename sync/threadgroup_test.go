package sync

import (
	"sync"
	"testing"
	"time"
)

// TestThreadGroup tests normal operation of a ThreadGroup.
func TestThreadGroup(t *testing.T) {
	var tg ThreadGroup
	err := tg.Add(10)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		go func() {
			defer tg.Done()
			select {
			case <-time.After(1 * time.Second):
			case <-tg.StopChan():
			}
		}()
	}
	start := time.Now()
	err = tg.Stop()
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
	var tg ThreadGroup

	// IsStopped should return false
	if tg.IsStopped() {
		t.Error("IsStopped returns true on unstopped ThreadGroup")
	}

	err := tg.Stop()
	if err != nil {
		t.Fatal(err)
	}

	// IsStopped should return true
	if !tg.IsStopped() {
		t.Error("IsStopped returns false on stopped ThreadGroup")
	}

	// Add and Stop should return errors
	err = tg.Add(1)
	if err != ErrStopped {
		t.Fatal("expected ErrStopped, got", err)
	}
	err = tg.Stop()
	if err != ErrStopped {
		t.Fatal("expected ErrStopped, got", err)
	}
}

// TestThreadGroupConcurrentAdd tests that Add can be called concurrently with Stop.
func TestThreadGroupConcurrentAdd(t *testing.T) {
	var tg ThreadGroup
	for i := 0; i < 10; i++ {
		go func() {
			err := tg.Add(1)
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
	tg.IsStopped()
	if tg.stopChan == nil {
		t.Error("stopChan should have been initialized by IsStopped")
	}

	tg = new(ThreadGroup)
	tg.Add(1)
	if tg.stopChan == nil {
		t.Error("stopChan should have been initialized by Add")
	}

	tg = new(ThreadGroup)
	tg.Stop()
	if tg.stopChan == nil {
		t.Error("stopChan should have been initialized by Stop")
	}
}

// TestThreadGroupRace tests that calling ThreadGroup methods concurrently
// does not trigger the race detector.
func TestThreadGroupRace(t *testing.T) {
	var tg ThreadGroup
	go tg.IsStopped()
	go tg.StopChan()
	go func() {
		if tg.Add(1) == nil {
			tg.Done()
		}
	}()
	err := tg.Stop()
	if err != nil {
		t.Fatal(err)
	}
}

func BenchmarkThreadGroup(b *testing.B) {
	var tg ThreadGroup
	for i := 0; i < b.N; i++ {
		tg.Add(1)
		go tg.Done()
	}
	tg.Stop()
}

func BenchmarkWaitGroup(b *testing.B) {
	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go wg.Done()
	}
	wg.Wait()
}
