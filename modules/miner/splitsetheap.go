package miner

// mapElements are stored in the map of a mapHeap.
// The index refers to the location of the split set in the underlying slice
// used to represent the heap.
type mapElement struct {
	set   *splitSet
	id    splitSetID
	index int
}

// MapMinHeap is a heap of splitSets (compared by averageFee).
// The minHeap bool specifies whether it is a min-heap or max-heap.
type mapHeap struct {
	selectID map[splitSetID]*mapElement
	rep      []*mapElement
	size     uint64
	minHeap  bool
}

// Len, Less, Swap implement the sort interface.
func (mh mapHeap) Len() int {
	return len(mh.rep)
}

func (mh mapHeap) Less(i, j int) bool {
	if mh.minHeap {
		return mh.rep[i].set.averageFee.Cmp(mh.rep[j].set.averageFee) == -1
	}
	return mh.rep[i].set.averageFee.Cmp(mh.rep[j].set.averageFee) == 1
}

func (mh mapHeap) Swap(i, j int) {
	// Swap in slice.
	mh.rep[i], mh.rep[j] = mh.rep[j], mh.rep[i]

	// Change values in slice to correct indices.
	mh.rep[i].index = i
	mh.rep[j].index = j

	// Change indices in mapElement structs in map to reflect position in slice.
	mh.selectID[mh.rep[i].id].index = i
	mh.selectID[mh.rep[j].id].index = j
}

// Push and Pop implement the heap interface.
func (mh *mapHeap) Push(x interface{}) {
	n := len(mh.rep)
	elt := x.(*mapElement)
	elt.index = n
	mh.rep = append(mh.rep, elt)
	mh.selectID[elt.id] = elt
	mh.size += elt.set.size
}

func (mh *mapHeap) Pop() interface{} {
	n := len(mh.rep)
	elt := mh.rep[n-1]
	mh.rep = mh.rep[0 : n-1]
	delete(mh.selectID, elt.id)
	mh.size -= elt.set.size
	return elt
}
