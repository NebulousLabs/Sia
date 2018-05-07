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
func (cdpq chunkCacheHeap) Len() int { return len(cdpq) }

// Less returns the lessor of two elements
func (cdpq chunkCacheHeap) Less(i, j int) bool { return cdpq[i].lastAccess < cdpq[j].lastAccess }

// Swap swaps two elements from the heap
func (cdpq chunkCacheHeap) Swap(i, j int) {
	cdpq[i], cdpq[j] = cdpq[j], cdpq[i]
	cdpq[i].index = i
	cdpq[j].index = j
}

// Push adds an element to the heap
func (cdpq *chunkCacheHeap) Push(x interface{}) {
	n := len(*cdpq)
	cacheData := x.(*cacheData)
	cacheData.index = n
	*cdpq = append(*cdpq, cacheData)
}

// Pop removes element from the heap
func (cdpq *chunkCacheHeap) Pop() interface{} {
	old := *cdpq
	n := len(old)
	cacheData := old[n-1]
	cacheData.index = -1 // for safety
	*cdpq = old[0 : n-1]
	return cacheData
}

// PopNoReturn will pop the element off the heap and is for when you do need the value
func (cdpq *chunkCacheHeap) PopNoReturn() {
	old := *cdpq
	n := len(old)
	cacheData := old[n-1]
	cacheData.index = -1 // for safety
	*cdpq = old[0 : n-1]
}

// update, updates the heap and reorders
func (cdpq *chunkCacheHeap) update(cd *cacheData, data []byte, lastAccess int64) {
	cd.data = data
	cd.lastAccess = lastAccess
	heap.Fix(cdpq, cd.index)
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
		var oldestKey string
		oldestTime := time.Now().Unix()

		// TODO: turn this from a structure where you loop over every element
		// (O(n) per access) to a min heap (O(log n) per access).
		// currently not a issue due to cache size remaining small (<20)
		for id, chunk := range udc.downloadChunkCache.chunkCacheMap {
			if chunk.lastAccess.Before(oldestTime) {
				oldestTime = chunk.lastAccess
				oldestKey = id
			}
		}
		delete(udc.downloadChunkCache.chunkCacheMap, oldestKey)

		build.Critical("Cache Data chunk not found in chunkCacheMap.")
	}

	udc.downloadChunkCache.chunkCacheMap[udc.staticCacheID] = &cacheData{
		data:       data,
		lastAccess: time.Now().Unix(),
	}
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

	// chunk exists, updating lastAccess and reinserting into map
	cd.lastAccess = time.Now().Unix()
	r.downloadChunkCache.chunkCacheMap[udc.staticCacheID] = cd

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
