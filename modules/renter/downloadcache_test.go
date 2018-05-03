package renter

import (
	"testing"
	"time"
)

// TestAddChunkToCache tests that the oldest chunk is removed
func TestAddChunkToCache(t *testing.T) {
	var udc unfinishedDownloadChunk
	udc.download = &download{
		staticDestinationType: "httpseekstream",
	}

	data := []byte{1, 2, 3, 4}

	// Fill Cache
	for i := 0; i < downloadCacheSize; i++ {
		udc.addChunkToCache(data)
		time.Sleep(1 * time.Millisecond)
	}

	// Get oldest key
	var oldestKey string
	oldestTime := time.Now()

	for id, chunk := range udc.chunkCache {
		if chunk.lastAccess.Before(oldestTime) {
			oldestTime = chunk.lastAccess
			oldestKey = id
		}
	}

	// Add additional chunk to force deletion of a chunk
	udc.addChunkToCache(data)

	// check if the chunk with the oldestKey was removed
	if _, ok := udc.chunkCache[oldestKey]; ok {
		t.Errorf("Expected ok to be false instead it was %v", ok)
	}

}
