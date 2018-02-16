package renter

// TODO: Move the memory manager to its own package.

// TODO: Add functions that allow a caller to increase or decrease the base
// memory for the memory manager.

import (
	"sync"

	"github.com/NebulousLabs/Sia/build"
)

// memoryManager can handle requests for memory and returns of memory. The
// memory manager is initialized with a base amount of memory and it will allow
// up to that much memory to be requested simultaneously. Beyond that, it will
// block on calls to 'managedGetMemory' until enough memory has been returned to
// allow the request.
//
// If a request is made that exceeds the base memory, the memory manager will
// block until all memory is available, and then grant the request, blocking all
// future requests for memory until the memory is returned. This allows large
// requests to go through even if there is not enough base memory.
type memoryManager struct {
	available    uint64
	base         uint64
	fifo         []*memoryRequest
	priorityFifo []*memoryRequest
	mu           sync.Mutex
	stop         <-chan struct{}
	underflow    uint64
}

// memoryRequest is a single thread that is blocked while waiting for memory.
type memoryRequest struct {
	amount uint64
	done   chan struct{}
}

// try will try to get the amount of memory requested from the manger, returning
// true if the attempt is successful, and false if the attempt is not.  In the
// event that the attempt is successful, the internal state of the memory
// manager will be updated to reflect the granted request.
func (mm *memoryManager) try(amount uint64) bool {
	if mm.available >= amount {
		// There is enough memory, decrement the memory and return.
		mm.available -= amount
		return true
	} else if mm.available == mm.base {
		// The amount of memory being requested is greater than the amount of
		// memory available, but no memory is currently in use. Set the amount
		// of memory available to zero and return.
		//
		// The effect is that all of the memory is allocated to this one
		// request, allowing the request to succeed even though there is
		// technically not enough total memory available for the request.
		mm.available = 0
		mm.underflow = amount - mm.base
		return true
	}
	return false
}

// Request is a blocking request for memory. The request will return when the
// memory has been acquired. If 'false' is returned, it means that the renter
// shut down before the memory could be allocated.
func (mm *memoryManager) Request(amount uint64, priority bool) bool {
	// Try to request the memory.
	mm.mu.Lock()
	if len(mm.fifo) == 0 && mm.try(amount) {
		mm.mu.Unlock()
		return true
	}

	// There is not enough memory available for this request, join the fifo.
	myRequest := &memoryRequest{
		amount: amount,
		done:   make(chan struct{}),
	}
	if priority {
		mm.priorityFifo = append(mm.priorityFifo, myRequest)
	} else {
		mm.fifo = append(mm.fifo, myRequest)
	}
	mm.mu.Unlock()

	// Block until memory is available or until shutdown. The thread that closes
	// the 'available' channel will also handle updating the memoryManager
	// variables.
	select {
	case <-myRequest.done:
		return true
	case <-mm.stop:
		return false
	}
}

// Return will return memory to the manager, waking any blocking threads which
// now have enough memory to proceed.
func (mm *memoryManager) Return(amount uint64) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Add the remaining memory to the pool of available memory, clearing out
	// the underflow if needed.
	if mm.underflow > 0 && amount <= mm.underflow {
		// Not even enough memory has been returned to clear the underflow.
		// Reduce the underflow amount and return.
		mm.underflow -= amount
		return
	} else if mm.underflow > 0 && amount > mm.underflow {
		amount -= mm.underflow
		mm.underflow = 0
	}
	mm.available += amount

	// Sanity check - the amount of memory available should not exceed the base
	// unless the memory manager is being used incorrectly.
	if mm.available > mm.base {
		build.Critical("renter memory manager being used incorrectly, too much memory returned")
		mm.available = mm.base
	}

	// Release as many of the priority threads blocking in the fifo as possible.
	for len(mm.priorityFifo) > 0 {
		if !mm.try(mm.priorityFifo[0].amount) {
			// There is not enough memory to grant the next request, meaning no
			// future requests should be checked either.
			return
		}
		// There is enough memory to grant the next request. Unblock that
		// request and continue checking the next requests.
		close(mm.priorityFifo[0].done)
		mm.priorityFifo = mm.priorityFifo[1:]
	}

	// Release as many of the threads blocking in the fifo as possible.
	for len(mm.fifo) > 0 {
		if !mm.try(mm.fifo[0].amount) {
			// There is not enough memory to grant the next request, meaning no
			// future requests should be checked either.
			return
		}
		// There is enough memory to grant the next request. Unblock that
		// request and continue checking the next requests.
		close(mm.fifo[0].done)
		mm.fifo = mm.fifo[1:]
	}
}

// newMemoryManager will create a memoryManager and return it.
func newMemoryManager(baseMemory uint64, stopChan <-chan struct{}) *memoryManager {
	return &memoryManager{
		available: baseMemory,
		base:      baseMemory,
		stop:      stopChan,
	}
}
