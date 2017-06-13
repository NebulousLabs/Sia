package miner

import "container/heap"

// mapElements are stored in a mapHeap.
// The index refers to the location of the splitSet in the underlying slice
// used to represent the heap.
type mapElement struct {
	set   *splitSet
	id    splitSetID
	index int
}

// MapHeap is a heap of splitSets (compared by averageFee).
// The minHeap bool specifies whether it is a min-heap or max-heap.
type mapHeap struct {
	selectID map[splitSetID]*mapElement
	data     []*mapElement
	size     uint64
	minHeap  bool
}

// Len returns the number of items stored in the heap.
// It implements the sort interface.
func (mh mapHeap) Len() int {
	return len(mh.data)
}

// Less returns true if the mapElement at index i is less than the element at
// index j if the mapHeap is a min-heap. If the mapHeap is a max-heap, it
// returns true if the element at index i is greater.
// It implements the sort interface.
func (mh mapHeap) Less(i, j int) bool {
	if mh.minHeap {
		return mh.data[i].set.averageFee.Cmp(mh.data[j].set.averageFee) == -1
	}
	return mh.data[i].set.averageFee.Cmp(mh.data[j].set.averageFee) == 1
}

// Swap swaps the elements at indices i and j. It also mutates the mapElements
// in the map of a mapHeap to reflect the change of indices.
func (mh mapHeap) Swap(i, j int) {
	// Swap in slice.
	mh.data[i], mh.data[j] = mh.data[j], mh.data[i]

	// Change values in slice to correct indices.
	mh.data[i].index = i
	mh.data[j].index = j

	// Change indices in mapElement structs in map to reflect position in slice.
	mh.selectID[mh.data[i].id].index = i
	mh.selectID[mh.data[j].id].index = j
}

// Push and Pop implement the heap interface.
func (mh *mapHeap) Push(x interface{}) {
	// Get the number of items stored in the heap.
	n := len(mh.data)

	// Assert the type, since mapHeap only stores mapElements.
	elem := x.(*mapElement)

	// Add elem to the bottom of the heap, and set the index to reflect that.
	// The Go library moves it into the proper place, and changes the index.
	elem.index = n
	mh.data = append(mh.data, elem)

	// Place the mapElement into the map with the correct splitSetID.
	mh.selectID[elem.id] = elem

	// Increment the mapHeap size by the size of the mapElement.
	mh.size += elem.set.size
}

func (mh *mapHeap) Pop() interface{} {
	// Get the number of items stored in the heap.
	n := len(mh.data)

	// Get the last element.
	elem := mh.data[n-1]

	// Shrink the data slice, and delete the mapElement from the map.
	mh.data = mh.data[0 : n-1]
	delete(mh.selectID, elem.id)

	// Decrement the size of the mapHeap.
	mh.size -= elem.set.size

	return elem
}

// RemoveSetByID removes an element from the MapHeap using only the splitSetID.
func (mh *mapHeap) RemoveSetByID(s splitSetID) *mapElement {
	// Get index into data at which the element is stored.
	i := mh.selectID[s].index

	//Remove it from the heap using the Go library.
	return heap.Remove(mh, i).(*mapElement)
}
