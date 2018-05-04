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
			staticDestinationType: destinationTypeSeekStream,
		},
		chunkCache: make(map[string]*cacheData),
		cacheMu:    new(sync.Mutex),
	}

	// Fill Cache
	for i := 0; i < downloadCacheSize; i++ {
		udc.staticCacheID = strconv.Itoa(i)
		udc.addChunkToCache([]byte{})
		time.Sleep(1 * time.Millisecond)
	}

	// Add additional chunk to force deletion of a chunk
	udc.staticCacheID = strconv.Itoa(downloadCacheSize)
	udc.addChunkToCache([]byte{})

	// check if the chunk with staticCacheID = "0" was removed
	// as that would have been the first to be added
	if _, ok := udc.chunkCache["0"]; ok {
		t.Error("The least recently accessed chunk wasn't pruned from the cache")
	}
}
