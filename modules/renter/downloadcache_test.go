package renter

import (
	"container/heap"
	"strconv"
	"testing"
	"time"
)

// TestHeapImplementation tests that the downloadChunkCache heap functions properly
func TestHeapImplementation(t *testing.T) {
	// Initializing minimum variables
	dcc := new(downloadChunkCache)
	dcc.Init()

	// Testing Push to Heap
	length := len(dcc.chunkCacheHeap)
	heap.Push(&dcc.chunkCacheHeap, &chunkData{
		id:         "Push",
		data:       []byte{},
		lastAccess: time.Now(),
	})

	// Confirming the length of the heap increased by 1
	if len(dcc.chunkCacheHeap) != length+1 {
		t.Error("Length of heap did not change, chunkData was not pushed onto Heap. Length of heap is still ", len(dcc.chunkCacheHeap))
	}
	// Confirming the chunk added was the one expected
	if dcc.chunkCacheHeap[0].id != "Push" {
		t.Error("Chunk on top of heap is not the chunk that was just pushed on, chunkData.id =", dcc.chunkCacheHeap[0].id)
	}

	// Add more chunks to heap
	for i := 0; i < 3; i++ {
		heap.Push(&dcc.chunkCacheHeap, &chunkData{
			id:         strconv.Itoa(i),
			data:       []byte{},
			lastAccess: time.Now(),
		})
		time.Sleep(1 * time.Second)
	}

	// Testing Heap update
	// Confirming recently accessed elements get moved to the bottom of Heap
	cd := dcc.chunkCacheHeap[0]
	dcc.chunkCacheHeap.update(cd, cd.id, cd.data, time.Now())
	if dcc.chunkCacheHeap[len(dcc.chunkCacheHeap)-1] != cd {
		t.Error("Heap order was not updated. Recently accessed element not at bottom of heap")
	}
	// Confirming least recently accessed element is moved to the top of Heap
	cd = dcc.chunkCacheHeap[len(dcc.chunkCacheHeap)-1]
	dcc.chunkCacheHeap.update(cd, cd.id, cd.data, time.Now().Add(-1*time.Hour))
	if dcc.chunkCacheHeap[0] != cd {
		t.Error("Heap order was not updated. Least recently accessed element is not at top of heap")
	}

	// Testing Pop of Heap
	// Confirming element at the top of heap is removed
	cd = dcc.chunkCacheHeap[0]
	length = len(dcc.chunkCacheHeap)
	if pop := heap.Pop(&dcc.chunkCacheHeap).(*chunkData); pop != cd {
		t.Error("Element at the top of the Heap was not popped off")
	}
	if len(dcc.chunkCacheHeap) != length-1 {
		t.Error("Heap length was not reduced by 1")
	}
}

// TestStreamCache tests that when Add() is called, chunks are added and removed
// from both the Heap and the Map
// Retrieve() is tested through the Streaming tests in the siatest packages
// SetStreamingCacheSize() is tested through the API endpoint tests in the
// siatest packages
func TestStreamCache(t *testing.T) {
	// Initializing minimum required variables
	dcc := new(downloadChunkCache)
	dcc.Init()

	// Fill Cache
	// Purposefully trying to fill to a value larger than cacheSize to confirm Add()
	// keeps pruning cache
	for i := 0; i < int(dcc.cacheSize)+5; i++ {
		dcc.Add(strconv.Itoa(i), []byte{})
		time.Sleep(1 * time.Second)
	}
	// Confirm that the chunkCacheHeap didn't exceed the cacheSize
	if len(dcc.chunkCacheHeap) > int(dcc.cacheSize) {
		t.Error("Heap is larger than set cacheSize")
	}

	// Add new chunk with known staticCacheID
	dcc.Add("chunk1", []byte{}) // "chunk1" should be at the bottom of the Heap

	// Confirm chunk is in the Map and at the bottom of the Heap
	cd, ok := dcc.chunkCacheMap["chunk1"]
	if !ok {
		t.Error("The chunk1 was not added to the Map")
	}
	if cd != dcc.chunkCacheHeap[len(dcc.chunkCacheHeap)-1] {
		t.Error("The chunk1 is not at the bottom of the Heap")
	}

	// Make chunk1 least recently accessed element, so it is at the top
	dcc.chunkCacheHeap.update(cd, cd.id, cd.data, time.Now().Add(-1*time.Hour))

	// Confirm chunk1 is at the top of the heap
	if dcc.chunkCacheHeap[0] != cd {
		t.Error("Chunk1 is not at the top of the heap")
	}

	// Add additional chunk to force deletion of a chunk
	dcc.Add("chunk2", []byte{})

	// check if chunk1 was removed from Map
	if _, ok := dcc.chunkCacheMap["chunk1"]; ok {
		t.Error("chunk1 wasn't removed from the map")
	}
	if dcc.chunkCacheHeap[0] == cd {
		t.Error("chunk1 wasn't removed from the heap")
	}
}
