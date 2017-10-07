package proto

import (
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/fastrand"
)

// mustAcquire is a convenience function for acquiring contracts that are
// known to be in the set.
func (cs *ContractSet) mustAcquire(t *testing.T, id types.FileContractID) modules.RenterContract {
	t.Helper()
	c, ok := cs.Acquire(id)
	if !ok {
		t.Fatal("no contract with that id")
	}
	return c
}

// TestContractSet tests that the ContractSet type is safe for concurrent use.
func TestContractSet(t *testing.T) {
	// create contract set
	id1 := types.FileContractID{1}
	id2 := types.FileContractID{2}
	cs := NewContractSet([]modules.RenterContract{
		{ID: id1},
		{ID: id2},
	})

	// uncontested acquire/release
	c1 := cs.mustAcquire(t, id1)
	cs.Return(c1)

	// 100 concurrent serialized mutations
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c1 := cs.mustAcquire(t, id1)
			c1.LastRevision.NewRevisionNumber++
			time.Sleep(time.Duration(fastrand.Intn(100)))
			cs.Return(c1)
		}()
	}
	wg.Wait()
	c1 = cs.mustAcquire(t, id1)
	cs.Return(c1)
	if c1.LastRevision.NewRevisionNumber != 100 {
		t.Fatal("expected exactly 100 increments, got", c1.LastRevision.NewRevisionNumber)
	}

	// a blocked acquire shouldn't prevent a return
	c1 = cs.mustAcquire(t, id1)
	go func() {
		time.Sleep(time.Millisecond)
		cs.Return(c1)
	}()
	c1 = cs.mustAcquire(t, id1)
	cs.Return(c1)

	// delete and reinsert id2
	c2 := cs.mustAcquire(t, id2)
	cs.Delete(c2)
	cs.Insert(c2)

	// call all the methods in parallel haphazardly
	funcs := []func(){
		func() { cs.Len() },
		func() { cs.IDs() },
		func() { cs.Contracts() },
		func() { cs.Return(cs.mustAcquire(t, id1)) },
		func() { cs.Return(cs.mustAcquire(t, id2)) },
		func() {
			id3 := types.FileContractID{3}
			cs.Insert(modules.RenterContract{ID: id3})
			cs.Delete(cs.mustAcquire(t, id3))
		},
	}
	wg = sync.WaitGroup{}
	for _, fn := range funcs {
		wg.Add(1)
		go func(fn func()) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				time.Sleep(time.Duration(fastrand.Intn(100)))
				fn()
			}
		}(fn)
	}
	wg.Wait()
}
