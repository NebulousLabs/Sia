package renter

import (
	"container/heap"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestAddChunkToCache tests that the oldest chunk is removed
func TestAddChunkToCache(t *testing.T) {
	// Initializing minimum variables
	udc := &unfinishedDownloadChunk{
		download: &download{
			staticDestinationType: destinationTypeSeekStream,
		},
		downloadChunkCache: new(downloadChunkCache),
		cacheMu:            new(sync.Mutex),
	}
	udc.downloadChunkCache.init()

	// Testing Push to Heap
	old := len(udc.downloadChunkCache.chunkCacheHeap)
	heap.Push(&udc.downloadChunkCache.chunkCacheHeap, &chunkData{
		id:         "Push",
		data:       []byte{},
		lastAccess: time.Now(),
	})
	if len(udc.downloadChunkCache.chunkCacheHeap) <= old {
		t.Error("chunkData was not pushed onto Heap.")
	}

	// Popping chunkData back off Heap to work with empty Heap
	cd := heap.Pop(&udc.downloadChunkCache.chunkCacheHeap).(*chunkData)

	// Fill Cache
	for i := 0; i < downloadCacheSize; i++ {
		udc.staticCacheID = strconv.Itoa(i)
		udc.addChunkToCache([]byte{})
		time.Sleep(1 * time.Millisecond)
	}

	// Testing Heap update
	cd = udc.downloadChunkCache.chunkCacheMap[strconv.Itoa(downloadCacheSize-1)]                   // this should have been the last element added and be at the bottom
	udc.downloadChunkCache.chunkCacheHeap.update(cd, cd.id, cd.data, time.Now().AddDate(0, -1, 0)) // updating it so it is at the top of Heap
	if udc.downloadChunkCache.chunkCacheHeap[0] != cd {
		t.Error("Heap order was not updated.")
	}

	// test pop
	cd = udc.downloadChunkCache.chunkCacheHeap[0]
	if pop := heap.Pop(&udc.downloadChunkCache.chunkCacheHeap).(*chunkData); pop != cd {
		t.Error("Least recently accessed chunk was not removed from the heap.")
	}

	// Pushing back on as to not cause error with addChunkToCache()
	heap.Push(&udc.downloadChunkCache.chunkCacheHeap, cd)

	// Add additional chunk to force deletion of a chunk
	udc.staticCacheID = strconv.Itoa(downloadCacheSize)
	udc.addChunkToCache([]byte{})

	// check if the chunk was removed from Map
	if _, ok := udc.downloadChunkCache.chunkCacheMap[cd.id]; ok {
		t.Error("The least recently accessed chunk wasn't pruned from the cache")
	}
}
