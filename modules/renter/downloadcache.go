package renter

import "time"

// addChunkToCache adds the chunk to the cache if the download is a streaming
// endpoint download.
func (udc *unfinishedDownloadChunk) addChunkToCache(data []byte) {
	if udc.download.staticDestinationType == destinationTypeSeekStream {
		udc.cacheMu.Lock()
		// Prune cache if necessary.
		for key := range udc.chunkCache {
			if len(udc.chunkCache) < downloadCacheSize {
				break
			}
			delete(udc.chunkCache, key)
		}
		udc.chunkCache[udc.staticCacheID] = data
		udc.cacheMu.Unlock()
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
	data, cached := r.chunkCache[udc.staticCacheID]
	r.cmu.Unlock()
	if !cached {
		return false
	}
	start := udc.staticFetchOffset
	end := start + udc.staticFetchLength
	_, err := udc.destination.WriteAt(data[start:end], udc.staticWriteOffset)
	if err != nil {
		r.log.Println("WARN: failed to write cached chunk to destination")
	}

	// Check if the download is complete now.
	udc.download.mu.Lock()
	udc.download.chunksRemaining--
	if udc.download.chunksRemaining == 0 {
		udc.download.endTime = time.Now()
		close(udc.download.completeChan)
	}
	udc.download.mu.Unlock()
	return true
}
