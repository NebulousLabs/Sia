package renter

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestAddChunkToCache tests that the oldest chunk is removed
func TestAddChunkToCache(t *testing.T) {
	udc := &unfinishedDownloadChunk{
		download: &download{
			staticDestinationType: "httpseekstream",
		},
		chunkCache: make(map[string]*cacheData),
		cacheMu:    new(sync.Mutex),
	}

	data := []byte{1, 2, 3, 4}

	// Fill Cache
	for i := 0; i < downloadCacheSize; i++ {
		udc.staticCacheID = strconv.Itoa(i)
		udc.addChunkToCache(data)
		time.Sleep(1 * time.Millisecond)
	}

	// Add additional chunk to force deletion of a chunk
	udc.staticCacheID = strconv.Itoa(downloadCacheSize)
	udc.addChunkToCache(data)

	// check if the chunk with staticCacheID = "0" was removed
	// as that would have been the first to be added
	if _, ok := udc.chunkCache["0"]; ok {
		t.Errorf("Expected ok to be false instead it was %v", ok)
	}

}
