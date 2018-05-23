package renter

import (
	"container/heap"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// TestHeapImplementation tests that the streamCache heap functions properly
func TestHeapImplementation(t *testing.T) {
	// Initializing minimum variables
	sc := new(streamCache)
	sc.Init()

	// Testing Push to Heap
	length := len(sc.streamHeap)
	cd := &chunkData{
		id:         "Push",
		data:       []byte{},
		lastAccess: time.Now(),
	}
	heap.Push(&sc.streamHeap, cd)

	// Confirming the length of the heap increased by 1
	if len(sc.streamHeap) != length+1 {
		t.Error("Length of heap did not change, chunkData was not pushed onto Heap. Length of heap is still ", len(sc.streamHeap))
	}
	// Confirming the chunk added was the one expected
	if !reflect.DeepEqual(cd, sc.streamHeap[0]) {
		t.Error("Chunk on top of heap is not the chunk that was just pushed on, chunkData.id =", sc.streamHeap[0].id)
	}

	// Add more chunks to heap
	for i := 0; i < 3; i++ {
		heap.Push(&sc.streamHeap, &chunkData{
			id:         strconv.Itoa(i),
			data:       []byte{},
			lastAccess: time.Now(),
		})
	}

	// Testing Heap update
	// Confirming recently accessed elements get moved to the bottom of Heap
	cd = sc.streamHeap[0]
	sc.streamHeap.update(cd, cd.id, cd.data, time.Now())
	if !reflect.DeepEqual(cd, sc.streamHeap[len(sc.streamHeap)-1]) {
		t.Error("Heap order was not updated. Recently accessed element not at bottom of heap")
	}
	// Confirming least recently accessed element is moved to the top of Heap
	cd = sc.streamHeap[len(sc.streamHeap)-1]
	sc.streamHeap.update(cd, cd.id, cd.data, time.Now().Add(-1*time.Hour))
	if !reflect.DeepEqual(cd, sc.streamHeap[0]) {
		t.Error("Heap order was not updated. Least recently accessed element is not at top of heap")
	}

	// Testing Pop of Heap
	// Confirming element at the top of heap is removed
	cd = sc.streamHeap[0]
	length = len(sc.streamHeap)
	if pop := heap.Pop(&sc.streamHeap).(*chunkData); !reflect.DeepEqual(cd, pop) {
		t.Error("Element at the top of the Heap was not popped off")
	}
	if len(sc.streamHeap) != length-1 {
		t.Error("Heap length was not reduced by 1")
	}
}

// TestStreamCache tests that when Add() is called, chunks are added and removed
// from both the Heap and the Map
// Retrieve() is tested through the Streaming tests in the siatest packages
// SetStreamingCacheSize() is tested through the API endpoint tests in the
// siatest packages
func TestStreamCache(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Initializing minimum required variables
	sc := new(streamCache)
	sc.Init()

	// Fill Cache
	// Purposefully trying to fill to a value larger than cacheSize to confirm Add()
	// keeps pruning cache
	for i := 0; i < int(sc.cacheSize)+5; i++ {
		sc.Add(strconv.Itoa(i), []byte{})
	}
	// Confirm that the streamHeap didn't exceed the cacheSize
	if len(sc.streamHeap) > int(sc.cacheSize) {
		t.Error("Heap is larger than set cacheSize")
	}

	// Add new chunk with known staticCacheID
	sc.Add("chunk1", []byte{}) // "chunk1" should be at the bottom of the Heap

	// Confirm chunk is in the Map and at the bottom of the Heap
	cd, ok := sc.streamMap["chunk1"]
	if !ok {
		t.Error("The chunk1 was not added to the Map")
	}
	if !reflect.DeepEqual(cd, sc.streamHeap[len(sc.streamHeap)-1]) {
		t.Error("The chunk1 is not at the bottom of the Heap")
	}

	// Make chunk1 least recently accessed element, so it is at the top
	sc.streamHeap.update(cd, cd.id, cd.data, time.Now().Add(-1*time.Hour))

	// Confirm chunk1 is at the top of the heap
	if !reflect.DeepEqual(cd, sc.streamHeap[0]) {
		t.Error("Chunk1 is not at the top of the heap")
	}

	// Add additional chunk to force deletion of a chunk
	sc.Add("chunk2", []byte{})

	// check if chunk1 was removed from Map
	if _, ok := sc.streamMap["chunk1"]; ok {
		t.Error("chunk1 wasn't removed from the map")
	}
	if reflect.DeepEqual(cd, sc.streamHeap[0]) {
		t.Error("chunk1 wasn't removed from the heap")
	}
}
