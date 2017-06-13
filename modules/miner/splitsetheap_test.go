package miner

import (
	"math/rand"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestMapHeapSimple test max-heap and min-heap versions of the MapHeap on the
// same sequence of pushes and pops. The pushes are done in increasing value of
// averageFee (the value by which elements are compared).
func TestMapHeapSimple(t *testing.T) {
	max := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  false,
	}

	min := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  true,
	}

	max.Init()
	min.Init()

	for i := 0; i < 1000; i++ {
		e1 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		e2 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		max.Push(e1)
		min.Push(e2)
	}

	for i := 0; i < 1000; i++ {
		maxPop := max.Pop()
		minPop := min.Pop()

		if int(maxPop.id) != 999-i {
			t.Log(maxPop.set.averageFee)
			t.Log(maxPop.id)
			t.Log(999 - i)
			t.Error("Unexpected splitSetID in result from max-heap pop.")
		}

		if int(minPop.id) != i {
			t.Error("Unexpected splitSetID in result from min-heap pop.")
		}

		if maxPop.set.averageFee.Cmp(types.SiacoinPrecision.Mul64(uint64(999-i))) != 0 {
			t.Error("Unexpected currency value in result from max-heap pop.")
		}

		if minPop.set.averageFee.Cmp(types.SiacoinPrecision.Mul64(uint64(i))) != 0 {
			t.Error("Unexpected currency value in result from min-heap pop.")
		}
	}
}

// TestMapHeapSimpleDiffOrder performs the same test as TestMapHeapSimple but
// pushes the half of elements with larger averageFee values first, and the
// smaller sized fee elements last. Then the sequence of pops is checked.
func TestMapHeapSimpleDiffOrder(t *testing.T) {
	max := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  false,
	}

	min := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  true,
	}

	max.Init()
	min.Init()

	for i := 50; i < 100; i++ {
		e1 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		e2 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		max.Push(e1)
		min.Push(e2)
	}

	for i := 0; i < 50; i++ {
		e1 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		e2 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		max.Push(e1)
		min.Push(e2)
	}

	for i := 0; i < 100; i++ {
		maxPop := max.Pop()
		minPop := min.Pop()

		if int(maxPop.id) != 99-i {
			t.Error("Unexpected splitSetID in result from max-heap pop.")
		}

		if int(minPop.id) != i {
			t.Error("Unexpected splitSetID in result from min-heap pop.")
		}

		if maxPop.set.averageFee.Cmp(types.SiacoinPrecision.Mul64(uint64(99-i))) != 0 {
			t.Error("Unexpected currency value in result from max-heap pop.")
		}

		if minPop.set.averageFee.Cmp(types.SiacoinPrecision.Mul64(uint64(i))) != 0 {
			t.Error("Unexpected currency value in result from min-heap pop.")
		}
	}
}

// TestMapHeapSize tests that the size of MapHeaps changes accordingly with the
// sizes of elements added to it, and with those elements removed from it.
// Tests a max-heap and min-heap on the same sequence of pushes and pops.
func TestMapHeapSize(t *testing.T) {
	max := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  false,
	}

	min := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  true,
	}

	max.Init()
	min.Init()

	var expectedSize uint64

	for i := 0; i < 1000; i++ {
		e1 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(100 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		e2 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(100 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		max.Push(e1)
		min.Push(e2)
		expectedSize += e1.set.size
	}

	if max.size != expectedSize {
		t.Error("Max-heap size different than expected size.")
	}
	if min.size != expectedSize {
		t.Error("Min-heap size different than expected size.")
	}

	for i := 0; i < 1000; i++ {
		maxPop := max.Pop()
		minPop := min.Pop()

		if maxPop.set.size != uint64(100*(999-i)) {
			t.Log(i)
			t.Error("Unexpected set size in result from max-heap pop.")
		}

		if minPop.set.size != uint64(100*i) {
			t.Log(i)
			t.Error("Unexpected set size in result from min-heap pop.")
		}

	}
}

// TestMapHeapRemoveBySetID pushes a sequence of elements onto
// a max-heap and min-heap. Then it removes a random element using
// its splitSetID, and checks that it has been removed.
func TestMapHeapRemoveBySetID(t *testing.T) {
	max := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  false,
	}

	min := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		data:     make([]*mapElement, 0),
		size:     0,
		minHeap:  true,
	}

	max.Init()
	min.Init()

	for i := 0; i < 5000; i++ {
		e1 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		e2 := &mapElement{
			set: &splitSet{
				averageFee:   types.SiacoinPrecision.Mul64(uint64(i)),
				size:         uint64(10 * i),
				transactions: make([]types.Transaction, 0),
			},

			id:    splitSetID(i),
			index: 0,
		}
		max.Push(e1)
		min.Push(e2)
	}

	randID := splitSetID(rand.Intn(5000))
	firstToBeRemoved := max.selectID[randID]

	// Iterate over data in min heap and max heap to confirm the element to be
	// removed is actually there.
	inMaxHeap := false
	inMinHeap := false
	for _, v := range max.data {
		if v.id == firstToBeRemoved.id {
			t.Log(v)
			inMaxHeap = true
			break
		}
	}
	for _, v := range min.data {
		if v.id == firstToBeRemoved.id {
			t.Log(v)
			inMinHeap = true
			break
		}
	}

	if !inMinHeap || !inMaxHeap {
		t.Error("Element not found in heap(s) before being removed by splitSetID.")
	}

	if max.selectID[randID] == nil || min.selectID[randID] == nil {
		t.Error("Element not found in map(s) before being removed by splitSetID")
	}

	max.RemoveSetByID(randID)
	min.RemoveSetByID(randID)

	// Iterate over data in min heap and max heap to confirm the element to be
	// removed was actually removed
	removedFromMax := true
	removedFromMin := true
	for _, v := range max.data {
		if v.id == firstToBeRemoved.id {
			removedFromMax = false
			break
		}
	}
	for _, v := range min.data {
		if v.id == firstToBeRemoved.id {
			removedFromMin = false
			break
		}
	}

	if !removedFromMin {
		t.Error("Element found in  min heap(s) after being removed by splitSetID.")
	}

	if !removedFromMax {
		t.Error("Element found in  max heap(s) after being removed by splitSetID.")
	}

	_, inMinMap := min.selectID[randID]
	_, inMaxMap := max.selectID[randID]
	if inMinMap {
		t.Error("Element found in min map(s) after being removed by splitSetID")
	}
	if inMaxMap {
		t.Error("Element found in max map(s) after being removed by splitSetID")
	}

}
