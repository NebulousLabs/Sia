package renter

import (
	"container/heap"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/errors"
)

// streamHeap is a priority queue and implements heap.Interface and holds chunkData
type streamHeap []*chunkData

// chunkData contatins the data and the timestamp for the unfinished
// download chunks
type chunkData struct {
	id         string
	data       []byte
	lastAccess time.Time
	index      int
}

// streamCache contains a streamMap for quick look up and a streamHeap for
// quick removal of old chunks
type streamCache struct {
	streamMap  map[string]*chunkData
	streamHeap streamHeap
	cacheSize  uint64
	mu         sync.Mutex
}

// Required functions for use of heap for streamHeap
func (sh streamHeap) Len() int { return len(sh) }

// Less returns the lesser of two elements
func (sh streamHeap) Less(i, j int) bool { return sh[i].lastAccess.Before(sh[j].lastAccess) }

// Swap swaps two elements from the heap
func (sh streamHeap) Swap(i, j int) {
	sh[i], sh[j] = sh[j], sh[i]
	sh[i].index = i
	sh[j].index = j
}

// Push adds an element to the heap
func (sh *streamHeap) Push(x interface{}) {
	n := len(*sh)
	chunkData := x.(*chunkData)
	chunkData.index = n
	*sh = append(*sh, chunkData)
}

// Pop removes element from the heap
func (sh *streamHeap) Pop() interface{} {
	old := *sh
	n := len(old)
	chunkData := old[n-1]
	chunkData.index = -1 // for safety
	*sh = old[0 : n-1]
	return chunkData
}

// update updates the heap and reorders
func (sh *streamHeap) update(cd *chunkData, id string, data []byte, lastAccess time.Time) {
	cd.id = id
	cd.data = data
	cd.lastAccess = lastAccess
	heap.Fix(sh, cd.index)
}

// Add adds the chunk to the cache if the download is a streaming
// endpoint download.
// TODO this won't be necessary anymore once we have partial downloads.
func (sc *streamCache) Add(cacheID string, data []byte) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Check to make sure chuck has not already been added
	if _, ok := sc.streamMap[cacheID]; ok {
		return
	}

	// pruning cache to cacheSize - 1 to make room to add the new chunk
	sc.pruneCache(sc.cacheSize - 1)

	// Add chunk to Map and Heap
	cd := &chunkData{
		id:         cacheID,
		data:       data,
		lastAccess: time.Now(),
	}
	sc.streamMap[cacheID] = cd
	heap.Push(&sc.streamHeap, cd)
	sc.streamHeap.update(cd, cd.id, cd.data, cd.lastAccess)
}

// pruneCache prunes the cache until it is the length of size
func (sc *streamCache) pruneCache(size uint64) {
	for len(sc.streamMap) > int(size) {
		// Remove from Heap
		cd := heap.Pop(&sc.streamHeap).(*chunkData)

		// Remove from Map
		if _, ok := sc.streamMap[cd.id]; !ok {
			build.Critical("Cache Data chunk not found in streamMap.")
		}
		delete(sc.streamMap, cd.id)
	}

	// Sanity check to confirm the Map and Heap where both pruned
	if len(sc.streamHeap) != len(sc.streamMap) {
		build.Critical("streamHeap and streamMap are not the same length,", len(sc.streamHeap), "and", len(sc.streamMap))
	}
}

// Retrieve tries to retrieve the chunk from the renter's cache. If
// successful it will write the data to the destination and stop the download
// if it was the last missing chunk. The function returns true if the chunk was
// in the cache.
// Using the entire unfinishedDownloadChunk as the argument as there are seven different fields
// used from unfinishedDownloadChunk and it allows using udc.fail()
//
// TODO: in the future we might need cache invalidation. At the
// moment this doesn't worry us since our files are static.
func (sc *streamCache) Retrieve(udc *unfinishedDownloadChunk) bool {
	udc.mu.Lock()
	defer udc.mu.Unlock()
	sc.mu.Lock()
	defer sc.mu.Unlock()

	cd, cached := sc.streamMap[udc.staticCacheID]
	if !cached {
		return false
	}

	// chunk exists, updating lastAccess and reinserting into map, updating heap
	cd.lastAccess = time.Now()
	sc.streamMap[udc.staticCacheID] = cd
	sc.streamHeap.update(cd, cd.id, cd.data, cd.lastAccess)

	start := udc.staticFetchOffset
	end := start + udc.staticFetchLength
	_, err := udc.destination.WriteAt(cd.data[start:end], udc.staticWriteOffset)
	if err != nil {
		udc.fail(errors.AddContext(err, "failed to write cached chunk to destination"))
		return true
	}

	// Check if the download is complete now.
	udc.download.mu.Lock()
	defer udc.download.mu.Unlock()

	udc.download.chunksRemaining--
	if udc.download.chunksRemaining == 0 {
		udc.download.endTime = time.Now()
		close(udc.download.completeChan)
		udc.download.destination.Close()
		udc.download.destination = nil
	}
	return true
}

// SetStreamingCacheSize sets the cache size.  When calling, add check
// to make sure cacheSize is greater than zero.  Otherwise it will remain
// the default value set during the initialization of the streamCache.
// It will also prune the cache to ensure the cache is always
// less than or equal to whatever the cacheSize is set to
func (sc *streamCache) SetStreamingCacheSize(cacheSize uint64) error {
	if cacheSize == 0 {
		return errors.New("cache size cannot be zero")
	}

	sc.mu.Lock()
	sc.cacheSize = cacheSize
	sc.pruneCache(sc.cacheSize)
	sc.mu.Unlock()
	return nil
}

// initStreamCache initializes the streaming cache of the renter.
func newStreamCache(cacheSize uint64) *streamCache {
	streamHeap := make(streamHeap, 0, cacheSize)
	heap.Init(&streamHeap)

	return &streamCache{
		streamMap:  make(map[string]*chunkData),
		streamHeap: streamHeap,
		cacheSize:  cacheSize,
	}
}
