package renter

// TODO expose the downloadCacheSize as a variable and allow users to set it
// via the API.

import (
	"container/heap"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/errors"
)

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

// addChunkToCache adds the chunk to the cache if the download is a streaming
// endpoint download.
// TODO this won't be necessary anymore once we have partial downloads.
func (udc *unfinishedDownloadChunk) addChunkToCache(data []byte) {
	if udc.download.staticDestinationType != destinationTypeSeekStream {
		// We only cache streaming chunks since browsers and media players tend
		// to only request a few kib at once when streaming data. That way we can
		// prevent scheduling the same chunk for download over and over.
		return
	}
	udc.cacheMu.Lock()
	defer udc.cacheMu.Unlock()

	// Prune cache if necessary.
	for len(udc.downloadChunkCache.chunkCacheMap) >= downloadCacheSize {
		// Remove from Heap
		cd := heap.Pop(&udc.downloadChunkCache.chunkCacheHeap).(*chunkData)

		// Remove from Map
		if _, ok := udc.downloadChunkCache.chunkCacheMap[cd.id]; !ok {
			build.Critical("Cache Data chunk not found in chunkCacheMap.")
		}
		delete(udc.downloadChunkCache.chunkCacheMap, cd.id)
	}

	// Add chunk to Map and Heap
	cd := &chunkData{
		id:         udc.staticCacheID,
		data:       data,
		lastAccess: time.Now(),
	}
	udc.downloadChunkCache.chunkCacheMap[udc.staticCacheID] = cd
	heap.Push(&udc.downloadChunkCache.chunkCacheHeap, cd)
	udc.downloadChunkCache.chunkCacheHeap.update(cd, cd.id, cd.data, cd.lastAccess)
}

// managedTryCache tries to retrieve the chunk from the renter's cache. If
// successful it will write the data to the destination and stop the download
// if it was the last missing chunk. The function returns true if the chunk was
// in the cache.
// TODO in the future we might need cache invalidation. At the
// moment this doesn't worry us since our files are static.
func (r *Renter) managedTryCache(udc *unfinishedDownloadChunk) bool {
	udc.mu.Lock()
	defer udc.mu.Unlock()
	r.cmu.Lock()
	cd, cached := r.downloadChunkCache.chunkCacheMap[udc.staticCacheID]
	r.cmu.Unlock()
	if !cached {
		return false
	}

	// chunk exists, updating lastAccess and reinserting into map, updating heap
	cd.lastAccess = time.Now()
	r.downloadChunkCache.chunkCacheMap[udc.staticCacheID] = cd
	udc.downloadChunkCache.chunkCacheHeap.update(cd, cd.id, cd.data, cd.lastAccess)

	start := udc.staticFetchOffset
	end := start + udc.staticFetchLength
	_, err := udc.destination.WriteAt(cd.data[start:end], udc.staticWriteOffset)
	if err != nil {
		r.log.Println("WARN: failed to write cached chunk to destination:", err)
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
