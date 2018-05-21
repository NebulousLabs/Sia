package renter

import (
	"container/heap"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestHeapImplementation tests that the downloadChunkCache heap functions properly
func TestHeapImplementation(t *testing.T) {
	// Initializing minimum variables
	downloadChunkCache := new(downloadChunkCache)
	downloadChunkCache.Init()

	// Testing Push to Heap
	old := len(downloadChunkCache.chunkCacheHeap)
	heap.Push(&downloadChunkCache.chunkCacheHeap, &chunkData{
		id:         "Push",
		data:       []byte{},
		lastAccess: time.Now(),
	})

	// Confirming the length of the heap increased by 1
	if len(downloadChunkCache.chunkCacheHeap) != old+1 {
		t.Error("Length of heap did not change, chunkData was not pushed onto Heap. Length of heap is still ", len(downloadChunkCache.chunkCacheHeap))
	}
	// Confirming the chunk added was the one expected
	if downloadChunkCache.chunkCacheHeap[0].id != "Push" {
		t.Error("Chunk on top of heap is not the chunk that was just pushed on, chunkData.id =", downloadChunkCache.chunkCacheHeap[0].id)
	}

	// Add more chunks to heap
	for i := 0; i < 3; i++ {
		heap.Push(&downloadChunkCache.chunkCacheHeap, &chunkData{
			id:         strconv.Itoa(i),
			data:       []byte{},
			lastAccess: time.Now(),
		})
		time.Sleep(1 * time.Second)
	}

	// Testing Heap update
	// Confirming recently accessed elements get moved to the bottom of Heap
	cd := downloadChunkCache.chunkCacheHeap[0]
	downloadChunkCache.chunkCacheHeap.update(cd, cd.id, cd.data, time.Now())
	if downloadChunkCache.chunkCacheHeap[len(downloadChunkCache.chunkCacheHeap)-1] != cd {
		t.Error("Heap order was not updated. Recently accessed element not at bottom of heap")
	}
	// Confirming least recently accessed element is moved to the top of Heap
	cd = downloadChunkCache.chunkCacheHeap[len(downloadChunkCache.chunkCacheHeap)-1]
	downloadChunkCache.chunkCacheHeap.update(cd, cd.id, cd.data, time.Now().Add(-1*time.Hour))
	if downloadChunkCache.chunkCacheHeap[0] != cd {
		t.Error("Heap order was not updated. Least recently accessed element is not at top of heap")
	}

	// Testing Pop of Heap
	// Confirming element at the top of heap is removed
	cd = downloadChunkCache.chunkCacheHeap[0]
	length := len(downloadChunkCache.chunkCacheHeap)
	if pop := heap.Pop(&downloadChunkCache.chunkCacheHeap).(*chunkData); pop != cd {
		t.Error("Element at the top of the Heap was not popped off")
	}
	if len(downloadChunkCache.chunkCacheHeap) != length-1 {
		t.Error("Heap length was not reduced by 1")
	}
}

// TestStreamCache tests that the oldest chunk is removed
func TestStreamCache(t *testing.T) {
	// Initializing minimum required variables
	udc := &unfinishedDownloadChunk{
		mu:                 new(sync.Mutex),
		downloadChunkCache: new(downloadChunkCache),
	}
	udc.downloadChunkCache.Init()

	// call Add
	// 		length of downloadChunkCache Heap should never exceed cacheSize (DONE)
	//		when Heap is full, top element should be removed to add a new element (IN PROGRESS)
	//		Top element should be least recently accessed

	// Fill Cache
	// Purposefully trying to fill to a value large to cacheSize to confirm Add
	// keeps pruning cache
	for i := 0; i < int(udc.downloadChunkCache.cacheSize)+5; i++ {
		udc.downloadChunkCache.Add(strconv.Itoa(i), []byte{})
		time.Sleep(1 * time.Second)
	}
	// Confirm that the chunkCacheHeap didn't exceed the cacheSize
	if len(udc.downloadChunkCache.chunkCacheHeap) > int(udc.downloadChunkCache.cacheSize) {
		t.Error("Heap is larger than set cacheSize")
	}

	// Making the least recently accessed element the most recently accessed element
	cd := udc.downloadChunkCache.chunkCacheHeap[0]
	// udc.downloadChunkCache.chunkCacheHeap.update(cd, cd.id, cd.data, time.Now())

	udc.downloadChunkCache.Add("New", []byte{})
	if udc.downloadChunkCache.chunkCacheHeap[0] == cd {
		t.Error("Least recently accessed element still at the top of the heap")
	}

	// Add additional chunk to force deletion of a chunk
	staticCacheID = strconv.FormatUint(udc.downloadChunkCache.cacheSize, 10)
	udc.downloadChunkCache.Add(staticCacheID, []byte{})

	// check if the chunk was removed from Map
	if _, ok := udc.downloadChunkCache.chunkCacheMap[cd.id]; ok {
		t.Error("The least recently accessed chunk wasn't pruned from the cache")
	}
	// test Retrieve
	// 		should updated the element making it the most recently accessed
	//		element should be at the bottom of the Heap
	udc.downloadChunkCache.Retrieve(udc)

	// don't need to test setStreaming cacheSize, that is tested through the API endpoint testing in siaTest
}
