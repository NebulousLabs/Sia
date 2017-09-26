package renter

import (
	"os"

	"github.com/NebulousLabs/errors"
)

// managedDownloadLogicalChunkData will fetch the logical chunk data by sending a
// download to the renter's downloader, and then using the data that gets
// returned.
func (r *Renter) managedDownloadLogicalChunkData(chunk *unfinishedChunk) error {
	// Create the download, queue the download, and then wait for the download
	// to finish.
	//
	// TODO / NOTE: Once we migrate to the uploader and downloader having a
	// shared memory pool, this part will need to signal to the download group
	// that the memory has already been allocated - upload memory always takes
	// more than download memory, and if we need to allocate two times in a row
	// from the same memory pool while other processes are asynchronously doing
	// the same, we risk deadlock.
	buf := NewDownloadBufferWriter(chunk.length, chunk.offset)
	// TODO: Should convert the inputs of newSectionDownload to use an int64 for
	// the offset.
	d := r.newSectionDownload(chunk.renterFile, buf, uint64(chunk.offset), chunk.length)
	select {
	case <-d.downloadFinished:
	case <-r.tg.StopChan():
		return errors.New("repair download interrupted by stop call")
	}
	chunk.logicalChunkData = buf.Bytes()
	return d.Err()
}

// managedFetchLogicalChunkData will get the raw data for a chunk, pulling it from disk if
// possible but otherwise queueing a download.
//
// chunk.data should be passed as 'nil' to the download, to keep memory usage as
// light as possible.
func (r *Renter) managedFetchLogicalChunkData(chunk *unfinishedChunk, download bool) error {
	// Download the chunk if it's not on disk.
	if chunk.localPath == "" && download {
		return r.managedDownloadLogicalChunkData(chunk)
	} else if chunk.localPath == "" {
		return errors.New("file not available locally")
	}

	// Try to read the data from disk. If that fails at any point, prefer to
	// download the chunk.
	//
	// TODO: Might want to remove the file from the renter tracking if the disk
	// loading fails.
	chunk.logicalChunkData = make([]byte, chunk.length)
	osFile, err := os.Open(chunk.localPath)
	if err != nil && download {
		chunk.logicalChunkData = nil
		return r.managedDownloadLogicalChunkData(chunk)
	} else if err != nil {
		return errors.Extend(err, errors.New("failed to open file locally"))
	}
	_, err = osFile.ReadAt(chunk.logicalChunkData, chunk.offset)
	if err != nil && download {
		chunk.logicalChunkData = nil
		return r.managedDownloadLogicalChunkData(chunk)
	} else if err != nil {
		return errors.Extend(err, errors.New("failed to read file locally"))
	}

	// Data successfully read from disk.
	return nil
}

// managedFetchAndRepairChunk will fetch the logical data for a chunk, create the
// physical pieces for the chunk, and then distribute them.
func (r *Renter) managedFetchAndRepairChunk(chunk *unfinishedChunk) {
	// Only download this file if more than 25% of the redundancy is missing.
	minMissingPiecesToDownload := (chunk.piecesNeeded - chunk.minimumPieces) / 4
	download := chunk.piecesCompleted + minMissingPiecesToDownload < chunk.piecesNeeded

	// Fetch the logical data for the chunk.
	err := r.managedFetchLogicalChunkData(chunk, download)
	if err != nil {
		// Logical data is not available, nothing to do.
		//
		// TODO: Log something here?
		return
	}

	// TODO: This operation ends up with extra memory, because we start with all
	// of the logical data in memory, and then at one point we have both all of
	// the logical data and also all of the physical data in memory, and there
	// is surely some way to avoid this.
	//
	// TODO: Currently the memoryNeeded stuff doesn't account for that extra bit
	// of memory which we potentially need after encoding the chunk. If we can't
	// find some way to eliminate the overhead, we need to update the code to
	// properly account for the temporary up-blip of memory.
	//
	// TODO: Even if we do fix that bit, right now we are currently encoding all
	// the pieces, and then immediately deleting the ones we don't need. We
	// could potentially save memory even more if we just didn't encode the ones
	// that we don't need.
	chunk.physicalChunkData, err = chunk.renterFile.erasureCode.Encode(chunk.logicalChunkData)
	if err != nil {
		// Logical data is not available, nothing to do.
		//
		// TODO: Log something here?
		return
	}
	// Nil out the logical chunk data so that it can be garbage collected.
	chunk.logicalChunkData = nil

	// Sanity check - we should have at least as many physical data pieces as we
	// do elements in our piece usage.
	if len(chunk.physicalChunkData) < len(chunk.pieceUsage) {
		r.log.Critical("not enough physical pieces to match the upload settings of the file")
		return
	}
	// Loop through the pieces and nil out any that are not needed, making note
	// of how much memory is freed so we can update the amount of memory in use.
	memoryFreed := uint64(0)
	for i := 0; i < len(chunk.pieceUsage); i++ {
		if chunk.pieceUsage[i] {
			memoryFreed += uint64(len(chunk.physicalChunkData[i]))
			chunk.physicalChunkData[i] = nil
		}
	}
	// Update the renter to indicate how much memory was freed.
	id := r.mu.Lock()
	r.memoryAvailable += memoryFreed
	r.mu.Unlock(id)
	// Notify the repair thread that more memory is available. If the channel is
	// full, the repair thread will already see that there is more memory, no
	// need to block.
	select {
	case r.newMemory <- struct{}{}:
	default:
	}

	// Distribute the chunk to all of the workers.
	id = r.mu.RLock()
	for _, worker := range r.workerPool {
		worker.queueChunkRepair(chunk)
	}
	r.mu.RUnlock(id)
}
