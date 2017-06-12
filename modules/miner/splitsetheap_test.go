package miner

import (
	"container/heap"
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

/*
   Write tests for:

   code coverage (of heap library too)

   check that sizes adjust properly

   push pop push should give same thing back (?)
*/

func TestMapHeapPushPopSimple(t *testing.T) {
	testElements := make([]*mapElement, 0)
	for i := 0; i < 100; i++ {
		e := &mapElement{
			set: &splitSet{
				averageFee:   types.NewCurrency(big.NewInt(int64(i))),
				size:         uint64(i),
				transactions: make([]types.Transaction, 10),
			},

			id:    splitSetID(i),
			index: 0,
		}
		testElements = append(testElements, e)
	}

	max := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		rep:      make([]*mapElement, 0),
		size:     0,
		minHeap:  false,
	}

	min := &mapHeap{
		selectID: make(map[splitSetID]*mapElement),
		rep:      make([]*mapElement, 0),
		size:     0,
		minHeap:  true,
	}

	heap.Init(max)
	heap.Init(min)

	for _, v := range testElements {
		heap.Push(max, v)
		heap.Push(min, v)
	}

	for i := 0; i < 100; i++ {
		maxPop := heap.Pop(max).(*mapElement)
		minPop := heap.Pop(min).(*mapElement)

		if int(maxPop.id) != 99-i {
			t.Error("Unexpected splitSetID in result from max-heap pop.")
		}

		if int(minPop.id) != i {
			t.Error("Unexpected splitSetID in result from min-heap pop.")
		}

		if maxPop.set.averageFee.Cmp(types.NewCurrency(big.NewInt(int64(99-i)))) != 0 {
			t.Error("Unexpected currency value in result from max-heap pop.")
		}

		if minPop.set.averageFee.Cmp(types.NewCurrency(big.NewInt(int64(i)))) != 0 {
			t.Error("Unexpected currency value in result from min-heap pop.")
		}
	}
}

func TestMapHeapPopPushPop(t *testing.T) {

}
