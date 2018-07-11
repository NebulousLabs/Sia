package proto

import (
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/fastrand"
)

// mustAcquire is a convenience function for acquiring contracts that are
// known to be in the set.
func (cs *ContractSet) mustAcquire(t *testing.T, id types.FileContractID) *SafeContract {
	t.Helper()
	c, ok := cs.Acquire(id)
	if !ok {
		t.Fatal("no contract with that id")
	}
	return c
}

// TestContractSet tests that the ContractSet type is safe for concurrent use.
func TestContractSet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create contract set
	testDir := build.TempDir(t.Name())
	cs, err := NewContractSet(testDir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}

	header1 := contractHeader{Transaction: types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:             types.FileContractID{1},
			NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
			UnlockConditions: types.UnlockConditions{
				PublicKeys: []types.SiaPublicKey{{}, {}},
			},
		}},
	}}
	header2 := contractHeader{Transaction: types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:             types.FileContractID{2},
			NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
			UnlockConditions: types.UnlockConditions{
				PublicKeys: []types.SiaPublicKey{{}, {}},
			},
		}},
	}}
	id1 := header1.ID()
	id2 := header2.ID()

	_, err = cs.managedInsertContract(header1, []crypto.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cs.managedInsertContract(header2, []crypto.Hash{})
	if err != nil {
		t.Fatal(err)
	}

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
			c1.header.Transaction.FileContractRevisions[0].NewRevisionNumber++
			time.Sleep(time.Duration(fastrand.Intn(100)))
			cs.Return(c1)
		}()
	}
	wg.Wait()
	c1 = cs.mustAcquire(t, id1)
	cs.Return(c1)
	if c1.header.LastRevision().NewRevisionNumber != 100 {
		t.Fatal("expected exactly 100 increments, got", c1.header.LastRevision().NewRevisionNumber)
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
	roots, err := c2.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal(err)
	}
	cs.managedInsertContract(c2.header, roots)

	// call all the methods in parallel haphazardly
	funcs := []func(){
		func() { cs.Len() },
		func() { cs.IDs() },
		func() { cs.View(id1); cs.View(id2) },
		func() { cs.ViewAll() },
		func() { cs.Return(cs.mustAcquire(t, id1)) },
		func() { cs.Return(cs.mustAcquire(t, id2)) },
		func() {
			header3 := contractHeader{
				Transaction: types.Transaction{
					FileContractRevisions: []types.FileContractRevision{{
						ParentID:             types.FileContractID{3},
						NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
						UnlockConditions: types.UnlockConditions{
							PublicKeys: []types.SiaPublicKey{{}, {}},
						},
					}},
				},
			}
			id3 := header3.ID()
			_, err := cs.managedInsertContract(header3, []crypto.Hash{})
			if err != nil {
				t.Fatal(err)
			}
			c3 := cs.mustAcquire(t, id3)
			cs.Delete(c3)
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
