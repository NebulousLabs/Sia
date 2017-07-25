package pool

// mapElements are stored in a mapHeap. The index refers to the location of the
// splitSet in the underlying slice used to represent the heap.
type mapElement struct {
	set   *splitSet
	id    splitSetID
	index int
}

// mapHeap is a heap of splitSets (compared by averageFee). The minHeap bool
// specifies whether it is a min-heap or max-heap.
type mapHeap struct {
	selectID map[splitSetID]*mapElement
	data     []*mapElement
	size     uint64
	minHeap  bool
}

// up maintains the heap condition by checking if the element at index j is less
// than its parent (as defined by less()). If so it swaps them, so that the
// element at index j goes 'up' the heap. It continues until the heap condition
// is satisfied again.
func (mh *mapHeap) up(j int) {
	for {
		// i is the parent of element at index j.
		i := (j - 1) / 2

		if i == j || !mh.less(j, i) {
			// Heap condition maintained.
			break
		}

		// Swap i and j, then continue.
		mh.swap(i, j)
		j = i
	}
}

// down maintains the heap condition by checking that the children of the
// element at index i are less than the element at i (as defined by less()). If
// so, it swaps them, and continues down the heap until the heap condition is
// satisfied.
func (mh *mapHeap) down(i0, n int) bool {
	i := i0
	for {
		// j1 is the left child of the element at index i
		j1 := 2*i + 1

		// Check that j1 is in the bounds of the heap (j1 < 0 after int overflow).
		if j1 >= n || j1 < 0 {
			break
		}

		//j is the left child of i.
		j := j1

		// If the right child (j2) of the element at index i (the sibling of j),
		// is within the bounds of the heap and satisfies
		if j2 := j1 + 1; j2 < n && !mh.less(j1, j2) {
			j = j2 // = 2*i + 2  // right child
		}

		// If the heap condition is true here, the method can exit.
		if !mh.less(j, i) {
			break
		}

		// Swap with the child and continue down the heap.
		mh.swap(i, j)
		i = j
	}
	return i > i0
}

// Len returns the number of items stored in the heap.
func (mh mapHeap) len() int {
	return len(mh.data)
}

// less returns true if the mapElement at index i is less than the element at
// index j if the mapHeap is a min-heap. If the mapHeap is a max-heap, it
// returns true if the element at index i is greater.
func (mh mapHeap) less(i, j int) bool {
	if mh.minHeap {
		return mh.data[i].set.averageFee.Cmp(mh.data[j].set.averageFee) == -1
	}
	return mh.data[i].set.averageFee.Cmp(mh.data[j].set.averageFee) == 1
}

// swap swaps the elements at indices i and j. It also mutates the mapElements
// in the map of a mapHeap to reflect the change of indices.
func (mh mapHeap) swap(i, j int) {
	// Swap in slice.
	mh.data[i], mh.data[j] = mh.data[j], mh.data[i]

	// Change values in slice to correct indices. Note that the same mapeElement
	// pointer is in the map also, so we only have to mutate it in one place.
	mh.data[i].index = i
	mh.data[j].index = j
}

// push puts an element onto the heap and maintains the heap condition.
func (mh *mapHeap) push(elem *mapElement) {
	// Get the number of items stored in the heap.
	n := len(mh.data)

	// Add elem to the bottom of the heap, and set the index to reflect that.
	elem.index = n
	mh.data = append(mh.data, elem)

	// Place the mapElement into the map with the correct splitSetID.
	mh.selectID[elem.id] = elem

	// Increment the mapHeap size by the size of the mapElement.
	mh.size += elem.set.size

	// Fix the heap condition by sifting up.
	mh.up(n)
}

// pop removes the top element from the heap (as defined by Less()) Pop will
// panic if called on an empty heap. Use Peek before Pop to be safe.
func (mh *mapHeap) pop() *mapElement {
	n := mh.len() - 1

	// Move the element to be popped to the end, then fix the heap condition.
	mh.swap(0, n)
	mh.down(0, n)

	// Get the last element.
	elem := mh.data[n]

	// Shrink the data slice, and delete the mapElement from the map.
	mh.data = mh.data[0:n]
	delete(mh.selectID, elem.id)

	// Decrement the size of the mapHeap.
	mh.size -= elem.set.size

	return elem
}

// removeSetByID removes an element from the MapHeap using only the splitSetID.
func (mh *mapHeap) removeSetByID(s splitSetID) *mapElement {
	// Get index into data at which the element is stored.
	i := mh.selectID[s].index

	//Remove it from the heap using the Go library.
	return mh.remove(i)
}

// Peek returns the element at the top of the heap without removing it.
func (mh *mapHeap) peek() (*mapElement, bool) {
	if len(mh.data) == 0 {
		return nil, false
	}
	return mh.data[0], true
}

// A heap must be initialized before any of the heap operations can be used.
// Init is idempotent with respect to the heap conditions and may be called
// whenever the heap conditions may have been invalidated. Its complexity is
// O(n) where n = h.Len().
func (mh *mapHeap) init() {
	// Sifts down through the heap to achieve the heap condition.
	n := mh.len()
	for i := n/2 - 1; i >= 0; i-- {
		mh.down(i, n)
	}
}

// remove removes the element at index i from the heap. The complexity is
// O(log(n)) where n = h.Len().
func (mh *mapHeap) remove(i int) *mapElement {
	n := mh.len() - 1

	// If the element to be removed is not at the top of the heap, move it. Then
	// fix the heap condition.
	if n != i {
		mh.swap(i, n)
		mh.down(i, n)
		mh.up(i)
	}

	// Get the last element.
	elem := mh.data[n]

	// Shrink the data slice, and delete the mapElement from the map.
	mh.data = mh.data[0:n]
	delete(mh.selectID, elem.id)

	// Decrement the size of the mapHeap.
	mh.size -= elem.set.size
	return elem
}

// fix re-establishes the heap ordering after the element at index i has changed
// its value. Changing the value of the element at index i and then calling Fix
// is equivalent to, but less expensive than, calling Remove(h, i) followed by a
// Push of the new value. The complexity is O(log(n)) where n = h.len().
func (mh *mapHeap) fix(i int) {
	// Check if the heap condition can be satisfied by sifting down.
	// If not, sift up too.
	if !mh.down(i, mh.len()) {
		mh.up(i)
	}
}
