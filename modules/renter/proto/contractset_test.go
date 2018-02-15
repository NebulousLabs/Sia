package proto

import (
	"sync"
	"testing"
	"time"

	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/fastrand"
)

// mustAcquire is a convenience function for acquiring contracts that are
// known to be in the set.
func (cs *ContractSet) mustAcquire(t *testing.T, id types.FileContractID) (*SafeContract, int) {
	t.Helper()
	c, ok, lockid := cs.Acquire(id)
	if !ok {
		t.Fatal("no contract with that id")
	}
	return c, lockid
}

// TestContractSet tests that the ContractSet type is safe for concurrent use.
func TestContractSet(t *testing.T) {
	// create contract set
	c1 := &SafeContract{header: contractHeader{Transaction: types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:             types.FileContractID{1},
			NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
			UnlockConditions: types.UnlockConditions{
				PublicKeys: []types.SiaPublicKey{{}, {}},
			},
		}},
	}}, mu: siasync.New(time.Minute, 100)}
	id1 := c1.header.ID()
	c2 := &SafeContract{header: contractHeader{Transaction: types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:             types.FileContractID{2},
			NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
			UnlockConditions: types.UnlockConditions{
				PublicKeys: []types.SiaPublicKey{{}, {}},
			},
		}},
	}}, mu: siasync.New(time.Minute, 100)}
	id2 := c2.header.ID()
	cs := &ContractSet{
		contracts: map[types.FileContractID]*SafeContract{
			id1: c1,
			id2: c2,
		},
	}

	// uncontested acquire/release
	c1, lockid := cs.mustAcquire(t, id1)
	cs.Return(c1, lockid)

	// 100 concurrent serialized mutations
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c1, lockid := cs.mustAcquire(t, id1)
			c1.header.Transaction.FileContractRevisions[0].NewRevisionNumber++
			time.Sleep(time.Duration(fastrand.Intn(100)))
			cs.Return(c1, lockid)
		}()
	}
	wg.Wait()
	c1, lockid = cs.mustAcquire(t, id1)
	cs.Return(c1, lockid)
	if c1.header.LastRevision().NewRevisionNumber != 100 {
		t.Fatal("expected exactly 100 increments, got", c1.header.LastRevision().NewRevisionNumber)
	}

	// a blocked acquire shouldn't prevent a return
	c1, lockid = cs.mustAcquire(t, id1)
	go func() {
		time.Sleep(time.Millisecond)
		cs.Return(c1, lockid)
	}()
	c1, lockid = cs.mustAcquire(t, id1)
	cs.Return(c1, lockid)

	// delete and reinsert id2
	c2, lockid = cs.mustAcquire(t, id2)
	cs.Delete(c2, lockid)
	cs.mu.Lock()
	cs.contracts[id2] = c2
	cs.mu.Unlock()

	// call all the methods in parallel haphazardly
	funcs := []func(){
		func() { cs.Len() },
		func() { cs.IDs() },
		func() { cs.View(id1); cs.View(id2) },
		func() { cs.ViewAll() },
		func() { cs.Return(cs.mustAcquire(t, id1)) },
		func() { cs.Return(cs.mustAcquire(t, id2)) },
		func() {
			c3 := &SafeContract{header: contractHeader{
				Transaction: types.Transaction{
					FileContractRevisions: []types.FileContractRevision{{
						ParentID:             types.FileContractID{3},
						NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
						UnlockConditions: types.UnlockConditions{
							PublicKeys: []types.SiaPublicKey{{}, {}},
						},
					}},
				},
			},
				mu: siasync.New(time.Minute, 100),
			}
			id3 := c3.header.ID()
			cs.mu.Lock()
			cs.contracts[id3] = c3
			cs.mu.Unlock()
			_, lockid := cs.mustAcquire(t, id3)
			cs.Delete(c3, lockid)
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
