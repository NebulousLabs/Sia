package renter

import (
	"testing"
	"time"
)

// Stream uses the streaming endpoint to download a file.
func TestAddChunkToCache(t *testing.T) {
	var udc *unfinishedDownloadChunk

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

	udc.addChunkToCache(data)

	// check if the oldestKey was removed
	if _, ok := udc.chunkCache[oldestKey]; ok {
		t.Errorf("Expected ok to be false instead it was %v", ok)
	}

}
