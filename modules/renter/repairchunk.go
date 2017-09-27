package renter

import (
	"io"
	"os"

	"github.com/NebulousLabs/errors"
)

// managedDistributeChunkToWorkers will take a chunk with fully prepared
// physical data and distribute it to the worker pool.
func (r *Renter) managedDistributeChunkToWorkers(uc *unfinishedChunk) {
	// Give the chunk to each worker, marking the number of workers that have
	// received the chunk.
	id := r.mu.RLock()
	uc.workersRemaining += len(r.workerPool)
	for _, worker := range r.workerPool {
		worker.queueChunkRepair(uc)
	}
	r.mu.RUnlock(id)

	// Perform cleanup for any pieces that will never be used by a worker.
	r.managedReleaseIdleChunkPieces(uc)
}

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

// managedFetchAndRepairChunk will fetch the logical data for a chunk, create the
// physical pieces for the chunk, and then distribute them.
func (r *Renter) managedFetchAndRepairChunk(chunk *unfinishedChunk) {
	// Only download this file if more than 25% of the redundancy is missing.
	minMissingPiecesToDownload := (chunk.piecesNeeded - chunk.minimumPieces) / 4
	download := chunk.piecesCompleted+minMissingPiecesToDownload < chunk.piecesNeeded

	// Fetch the logical data for the chunk.
	err := r.managedFetchLogicalChunkData(chunk, download)
	if err != nil {
		// Logical data is not available, nothing to do.
		r.log.Debugln("Fetching logical data of a chunk failed:", err)
		return
	}

	// Create the phsyical pieces for the data.
	chunk.physicalChunkData, err = chunk.renterFile.erasureCode.Encode(chunk.logicalChunkData)
	if err != nil {
		// Logical data is not available, nothing to do.
		r.log.Debugln("Fetching physical data of a chunk failed:", err)
		return
	}
	// Nil out the logical chunk data so that it can be garbage collected.
	memoryFreed := uint64(len(chunk.logicalChunkData))
	chunk.logicalChunkData = nil

	// Sanity check - we should have at least as many physical data pieces as we
	// do elements in our piece usage.
	if len(chunk.physicalChunkData) < len(chunk.pieceUsage) {
		r.log.Critical("not enough physical pieces to match the upload settings of the file")
		return
	}
	// Loop through the pieces and encrypt any that our needed, while dropping
	// any pieces that are not needed.
	for i := 0; i < len(chunk.pieceUsage); i++ {
		if chunk.pieceUsage[i] {
			memoryFreed += uint64(len(chunk.physicalChunkData[i]))
			chunk.physicalChunkData[i] = nil
		} else {
			// Encrypt the piece.
			key := deriveKey(chunk.renterFile.masterKey, chunk.index, uint64(i))
			chunk.physicalChunkData[i] = key.EncryptBytes(chunk.physicalChunkData[i])
		}
	}
	// Update the renter to indicate how much memory was freed.
	r.managedMemoryAvailableAdd(memoryFreed)

	// Distribute the chunk to the workers.
	r.managedDistributeChunkToWorkers(chunk)
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
	// TODO: Once we have enabled support for small chunks, we should stop
	// needing to ignore the EOF errors, because the chunk size should always
	// match the tail end of the file. Until then, we ignore io.EOF.
	_, err = osFile.ReadAt(chunk.logicalChunkData, chunk.offset)
	if err != nil && err != io.EOF && download {
		chunk.logicalChunkData = nil
		return r.managedDownloadLogicalChunkData(chunk)
	} else if err != nil && err != io.EOF {
		return errors.Extend(err, errors.New("failed to read file locally"))
	}

	// Data successfully read from disk.
	return nil
}

// releaseIdleChunkPieces will drop any chunk pieces that are no longer going to
// be used by workers because the number of remaining pieces is greater than the
// number of remaining workers. The memory will be returned to the renter.
func (r *Renter) managedReleaseIdleChunkPieces(uc *unfinishedChunk) {
	uc.mu.Lock()
	piecesAvailable := 0
	memoryReleased := 0
	for i := 0; i < len(uc.pieceUsage); i++ {
		// Skip the piece if it's not available.
		if uc.pieceUsage[i] {
			continue
		}

		// If we have all the available pieces we need, release this piece.
		// Otherwise, mark that there's another piece available. This algorithm
		// will prefer releasing later pieces, which improves computational
		// complexity for erasure coding.
		if piecesAvailable >= uc.workersRemaining {
			uc.pieceUsage[i] = false
			memoryReleased += len(uc.physicalChunkData[i])
			uc.physicalChunkData[i] = nil
		} else {
			piecesAvailable++
		}
	}
	uc.mu.Unlock()
	if memoryReleased > 0 {
		r.managedMemoryAvailableAdd(uint64(memoryReleased))
	}
}
