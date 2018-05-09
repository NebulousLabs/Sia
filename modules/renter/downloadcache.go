package renter

// TODO expose the downloadCacheSize as a variable and allow users to set it
// via the API.

import (
	"container/heap"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/errors"
)

// chunkCacheHeap is a priority queue and implements heap.Interface and holds chunkData
type chunkCacheHeap []*chunkData

// chunkData contatins the data and the timestamp for the unfinished
// download chunks
type chunkData struct {
	id         string
	data       []byte
	lastAccess time.Time
	index      int
}

// downloadChunkCache contains a chunkCacheMap for quick look up and a chunkCacheHeap for
// quick removal of old chunks
type downloadChunkCache struct {
	chunkCacheMap  map[string]*chunkData
	chunkCacheHeap chunkCacheHeap
	cacheSize      int
	mu             sync.Mutex
}

// Required functions for use of heap for chunkCacheHeap
func (cch chunkCacheHeap) Len() int { return len(cch) }

// Less returns the lessor of two elements
func (cch chunkCacheHeap) Less(i, j int) bool { return cch[i].lastAccess.Before(cch[j].lastAccess) }

// Swap swaps two elements from the heap
func (cch chunkCacheHeap) Swap(i, j int) {
	cch[i], cch[j] = cch[j], cch[i]
	cch[i].index = i
	cch[j].index = j
}

// Push adds an element to the heap
func (cch *chunkCacheHeap) Push(x interface{}) {
	n := len(*cch)
	chunkData := x.(*chunkData)
	chunkData.index = n
	*cch = append(*cch, chunkData)
}

// Pop removes element from the heap
func (cch *chunkCacheHeap) Pop() interface{} {
	old := *cch
	n := len(old)
	chunkData := old[n-1]
	chunkData.index = -1 // for safety
	*cch = old[0 : n-1]
	return chunkData
}

// update, updates the heap and reorders
func (cch *chunkCacheHeap) update(cd *chunkData, id string, data []byte, lastAccess time.Time) {
	cd.id = id
	cd.data = data
	cd.lastAccess = lastAccess
	heap.Fix(cch, cd.index)
}

// init initializes the downloadChunkCache
func (dcc *downloadChunkCache) init(cacheSize int) {
	dcc.chunkCacheMap = make(map[string]*chunkData)
	dcc.chunkCacheHeap = make(chunkCacheHeap, 0, cacheSize)
	cacheSize = cacheSize
	heap.Init(&dcc.chunkCacheHeap)
}

// add adds the chunk to the cache if the download is a streaming
// endpoint download.
// TODO this won't be necessary anymore once we have partial downloads.
func (dcc *downloadChunkCache) add(data []byte, udc *unfinishedDownloadChunk) {
	if udc.download.staticDestinationType != destinationTypeSeekStream {
		// We only cache streaming chunks since browsers and media players tend
		// to only request a few kib at once when streaming data. That way we can
		// prevent scheduling the same chunk for download over and over.
		return
	}
	dcc.mu.Lock()
	defer dcc.mu.Unlock()

	// Prune cache if necessary.
	for len(dcc.chunkCacheMap) >= dcc.cacheSize {
		// Remove from Heap
		cd := heap.Pop(&dcc.chunkCacheHeap).(*chunkData)

		// Remove from Map
		if _, ok := dcc.chunkCacheMap[cd.id]; !ok {
			build.Critical("Cache Data chunk not found in chunkCacheMap.")
		}
		delete(dcc.chunkCacheMap, cd.id)
	}

	// Add chunk to Map and Heap
	cd := &chunkData{
		id:         udc.staticCacheID,
		data:       data,
		lastAccess: time.Now(),
	}
	dcc.chunkCacheMap[udc.staticCacheID] = cd
	heap.Push(&dcc.chunkCacheHeap, cd)
	dcc.chunkCacheHeap.update(cd, cd.id, cd.data, cd.lastAccess)
}

// retreive tries to retrieve the chunk from the renter's cache. If
// successful it will write the data to the destination and stop the download
// if it was the last missing chunk. The function returns true if the chunk was
// in the cache.
// TODO in the future we might need cache invalidation. At the
// moment this doesn't worry us since our files are static.
func (dcc *downloadChunkCache) retreive(udc *unfinishedDownloadChunk) bool {
	udc.mu.Lock()
	defer udc.mu.Unlock()
	dcc.mu.Lock()
	defer dcc.mu.Unlock()

	cd, cached := dcc.chunkCacheMap[udc.staticCacheID]
	if !cached {
		return false
	}

	// chunk exists, updating lastAccess and reinserting into map, updating heap
	cd.lastAccess = time.Now()
	dcc.chunkCacheMap[udc.staticCacheID] = cd
	dcc.chunkCacheHeap.update(cd, cd.id, cd.data, cd.lastAccess)

	start := udc.staticFetchOffset
	end := start + udc.staticFetchLength
	_, err := udc.destination.WriteAt(cd.data[start:end], udc.staticWriteOffset)
	if err != nil {
		// r.log.Println("WARN: failed to write cached chunk to destination:", err)
		udc.fail(errors.AddContext(err, "failed to write cached chunk to destination"))
		return true
	}

	// Check if the download is complete now.
	udc.download.mu.Lock()
	udc.download.chunksRemaining--
	if udc.download.chunksRemaining == 0 {
		udc.download.endTime = time.Now()
		close(udc.download.completeChan)
		udc.download.destination.Close()
		udc.download.destination = nil
	}
	udc.download.mu.Unlock()
	return true
}
