package renter

// The download heap is a heap that contains all the chunks that we are trying
// to download, sorted by download priority. Each time there are resources
// available to kick off another download, a chunk is popped off the heap,
// prepared for downloading, and then sent off to the workers.
//
// Download jobs are added to the heap via a function call.

import (
	"errors"
	"time"
)

const (
	defaultFilePerm         = 0666
	downloadFailureCooldown = time.Minute * 30
)

var (
	errDownloadRenterClosed = errors.New("download could not be scheduled because renter is shutting down")
	errInsufficientHosts    = errors.New("insufficient hosts to recover file")
	errInsufficientPieces   = errors.New("couldn't fetch enough pieces to recover data")
	errPrevErr              = errors.New("download could not be completed due to a previous error")
)

// downloadChunkHeap is a heap that is sorted first by file priority, then by
// the start time of the download, and finally by the index of the chunk.  As
// downloads are queued, they are added to the downloadChunkHeap. As resources
// become available to execute downloads, chunks are pulled off of the heap and
// distributed to workers.
type downloadChunkHeap []*unfinishedDownloadChunk

// Implementation of heap.Interface for downloadChunkHeap.
func (dch downloadChunkHeap) Len() int { return len(dch) }
func (dch downloadChunkHeap) Less(i, j int) bool {
	// First sort by priority.
	if dch[i].priority != dch[j].priority {
		return dch[i].priority > dch[j].priority
	}
	// For equal priority, sort by start time.
	if dch[i].download.startTime != dch[j].download.startTime {
		return dch[i].download.startTime.Before(dch[j].download.startTime)
	}
	// For equal start time (typically meaning it's the same file), sort by
	// chunkIndex.
	//
	// NOTE: To prevent deadlocks when acquiring memory and using writers that
	// will streamline / order different chunks, we must make sure that we sort
	// by chunkIndex such that the earlier chunks are selected first from the
	// heap.
	return dch[i].chunkIndex < dch[j].chunkIndex
}
func (dch downloadChunkHeap) Swap(i, j int)       { dch[i], dch[j] = dch[j], dch[i] }
func (dch *downloadChunkHeap) Push(x interface{}) { *dch = append(*dch, x.(*unfinishedDownloadChunk)) }
func (dch *downloadChunkHeap) Pop() interface{} {
	old := *dch
	n := len(old)
	x := old[n-1]
	*dch = old[0 : n-1]
	return x
}

// acquireMemoryForDownloadChunk will block until memory is available for the
// chunk to be downloaded. 'false' will be returned if the renter shuts down
// before memory can be acquired.
func (r *Renter) managedAcquireMemoryForDownloadChunk(udc *unfinishedDownloadChunk) bool {
	// If the chunk does not need memory, can return immediately with a success
	// condition - all requried memory has been acquired.
	if nextChunk.memoryRequired == 0 {
		return true
	}
	// Wait to acquire the memory. 'false' will be returned if the renter shuts
	// down before memory can be acquired.
	return r.managedMemoryGet(memoryNeeded)
}

// managedAddChunkToHeap will add a chunk to the download heap in a thread-safe
// way.
func (r *Renter) managedAddChunkToHeap(udc *unfinishedDownloadChunk) {
	r.downloadHeapMu.Lock()
	r.downloadHeap.Push(udc)
	r.downloadHeapMu.Unlock()
}

// managedBlockUntilOnline will block until the renter is online. The renter
// will appropriately handle incoming download requests and stop signals while
// waiting.
func (r *Renter) managedBlockUntillOnline() bool {
	for !r.g.Online() {
		select {
		case <-r.tg.StopChan():
			return false
		case <-time.After(offlineCheckFrequency):
		}
	}
	return true
}

// managedNextDownloadChunk will fetch the next chunk from the download heap. If
// the download heap is empty, 'nil' will be returned.
func (r *Renter) managedNextDownloadChunk() *unfinishedDownloadChunk {
	r.downloadHeapMu.Lock()
	if r.downloadHeap.Len() <= 0 {
		r.downloadHeapMu.Unlock()
		return nil
	}
	nextChunk := heap.Pop(r.downloadHeap).(*unfinishedDownloadChunk)
	r.downloadHeapMu.Unlock()
	return nextChunk
}

// threadedDownloadLoop utilizes the worker pool to make progress on any queued
// downloads.
func (r *Renter) threadedDownloadLoop() {
	err := r.tg.Add()
	if err != nil {
		return
	}
	defer r.tg.Done()

	// Infinite loop to process downloads. Will return if r.tg.Stop() is called.
LOOP:
	for {
		// Wait until the renter is online.
		if !r.managedBlockUntilOnline() {
			// The renter shut down before the internet connection was restored.
			return
		}

		// Update the worker pool and fetch the current time. The loop will
		// reset after a certain amount of time has passed.
		r.managedUpdateWorkerPool()
		workerUpdateTime := time.Now()

		// Pull downloads out of the heap. Will break if the heap is empty, and
		// will reset to the top of the outer loop if a reset condition is met.
		for {
			// Check that we still have an internet connection, and also that we
			// do not need to update the worker pool yet.
			if !r.g.Online() || time.Now().After(workerUpdateTime.Add(workerPoolUpdateTimeout)) {
				// Reset to the top of the outer loop.
				continue LOOP
			}

			// Get the next chunk.
			nextChunk := r.managedNextDownloadChunk()
			if nextChunk == nil {
				// Break out of the inner loop and wait for more work.
				break
			}
			// Get the required memory to download this chunk.
			if !r.managedAcquireMemoryForDownloadChunk(nextChunk) {
				// The renter shut down before memory could be acquired.
				return
			}

			// Distribute the chunk to workers, marking the number of workers
			// that have received the work.
			r.mu.Lock()
			nextChunk.mu.Lock()
			nextChunk.workersRemaining = len(r.workerPool)
			nextChunk.mu.Unlock()
			for _, worker := range r.workerPool {
				worker.managedQueueDownloadChunk(nextChunk)
			}
			r.mu.Unlock()
		}

		// Wait for more work.
		select {
		case <-r.tg.StopChan():
			return
		case <-r.newDownloads:
		}
	}
}
