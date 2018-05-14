package renter

import (
	"io"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"

	"github.com/NebulousLabs/errors"
)

// uploadChunkID is a unique identifier for each chunk in the renter.
type uploadChunkID struct {
	fileUID string // Unique to each file.
	index   uint64 // Unique to each chunk within a file.
}

// unfinishedUploadChunk contains a chunk from the filesystem that has not
// finished uploading, including knowledge of the progress.
type unfinishedUploadChunk struct {
	// Information about the file. localPath may be the empty string if the file
	// is known not to exist locally.
	id         uploadChunkID
	localPath  string
	renterFile *file

	// Information about the chunk, namely where it exists within the file.
	//
	// TODO / NOTE: As we change the file mapper, we're probably going to have
	// to update these fields. Compatibility shouldn't be an issue because this
	// struct is not persisted anywhere, it's always built from other
	// structures.
	index          uint64
	length         uint64
	memoryNeeded   uint64 // memory needed in bytes
	memoryReleased uint64 // memory that has been returned of memoryNeeded
	minimumPieces  int    // number of pieces required to recover the file.
	offset         int64  // Offset of the chunk within the file.
	piecesNeeded   int    // number of pieces to achieve a 100% complete upload

	// The logical data is the data that is presented to the user when the user
	// requests the chunk. The physical data is all of the pieces that get
	// stored across the network.
	logicalChunkData  [][]byte
	physicalChunkData [][]byte

	// Worker synchronization fields. The mutex only protects these fields.
	//
	// When a worker passes over a piece for upload to go on standby:
	//	+ the worker should add itself to the list of standby chunks
	//  + the worker should call for memory to be released
	//
	// When a worker passes over a piece because it's not useful:
	//	+ the worker should decrement the number of workers remaining
	//	+ the worker should call for memory to be released
	//
	// When a worker accepts a piece for upload:
	//	+ the worker should increment the number of pieces registered
	// 	+ the worker should mark the piece usage for the piece it is uploading
	//	+ the worker should decrement the number of workers remaining
	//
	// When a worker completes an upload (success or failure):
	//	+ the worker should decrement the number of pieces registered
	//  + the worker should call for memory to be released
	//
	// When a worker completes an upload (failure):
	//	+ the worker should unmark the piece usage for the piece it registered
	//	+ the worker should notify the standby workers of a new available piece
	//
	// When a worker completes an upload successfully:
	//	+ the worker should increment the number of pieces completed
	//	+ the worker should decrement the number of pieces registered
	//	+ the worker should release the memory for the completed piece
	mu               sync.Mutex
	pieceUsage       []bool              // 'true' if a piece is either uploaded, or a worker is attempting to upload that piece.
	piecesCompleted  int                 // number of pieces that have been fully uploaded.
	piecesRegistered int                 // number of pieces that are being uploaded, but aren't finished yet (may fail).
	released         bool                // whether this chunk has been released from the active chunks set.
	unusedHosts      map[string]struct{} // hosts that aren't yet storing any pieces or performing any work.
	workersRemaining int                 // number of inactive workers still able to upload a piece.
	workersStandby   []*worker           // workers that can be used if other workers fail.
}

// managedNotifyStandbyWorkers is called when a worker fails to upload a piece, meaning
// that the standby workers may now be needed to help the piece finish
// uploading.
func (uc *unfinishedUploadChunk) managedNotifyStandbyWorkers() {
	// Copy the standby workers into a new slice and reset it since we can't
	// hold the lock while calling the managed function.
	uc.mu.Lock()
	standbyWorkers := make([]*worker, len(uc.workersStandby))
	copy(standbyWorkers, uc.workersStandby)
	uc.workersStandby = uc.workersStandby[:0]
	uc.mu.Unlock()

	for i := 0; i < len(standbyWorkers); i++ {
		standbyWorkers[i].managedQueueUploadChunk(uc)
	}
}

// managedDistributeChunkToWorkers will take a chunk with fully prepared
// physical data and distribute it to the worker pool.
func (r *Renter) managedDistributeChunkToWorkers(uc *unfinishedUploadChunk) {
	// Give the chunk to each worker, marking the number of workers that have
	// received the chunk. The workers cannot be interacted with while the
	// renter is holding a lock, so we need to build a list of workers while
	// under lock and then launch work jobs after that.
	id := r.mu.RLock()
	uc.workersRemaining += len(r.workerPool)
	workers := make([]*worker, 0, len(r.workerPool))
	for _, worker := range r.workerPool {
		workers = append(workers, worker)
	}
	r.mu.RUnlock(id)
	for _, worker := range workers {
		worker.managedQueueUploadChunk(uc)
	}
}

// managedDownloadLogicalChunkData will fetch the logical chunk data by sending a
// download to the renter's downloader, and then using the data that gets
// returned.
func (r *Renter) managedDownloadLogicalChunkData(chunk *unfinishedUploadChunk) error {
	//  Determine what the download length should be. Normally it is just the
	//  chunk size, but if this is the last chunk we need to download less
	//  because the file is not that large.
	//
	// TODO: There is a disparity in the way that the upload and download code
	// handle the last chunk, which may not be full sized.
	downloadLength := chunk.length
	if chunk.index == chunk.renterFile.numChunks()-1 && chunk.renterFile.size%chunk.length != 0 {
		downloadLength = chunk.renterFile.size % chunk.length
	}

	// Create the download.
	buf := NewDownloadDestinationBuffer(chunk.length)
	d, err := r.managedNewDownload(downloadParams{
		destination:     buf,
		destinationType: "buffer",
		file:            chunk.renterFile,

		latencyTarget: 200e3, // No need to rush latency on repair downloads.
		length:        downloadLength,
		needsMemory:   false, // We already requested memory, the download memory fits inside of that.
		offset:        uint64(chunk.offset),
		overdrive:     0, // No need to rush the latency on repair downloads.
		priority:      0, // Repair downloads are completely de-prioritized.
	})
	if err != nil {
		return err
	}

	// Set the in-memory buffer to nil just to be safe in case of a memory
	// leak.
	defer func() {
		d.destination = nil
	}()

	// Wait for the download to complete.
	select {
	case <-d.completeChan:
	case <-r.tg.StopChan():
		return errors.New("repair download interrupted by stop call")
	}
	if d.Err() != nil {
		buf = nil
		return d.Err()
	}
	chunk.logicalChunkData = [][]byte(buf)
	return nil
}

// managedFetchAndRepairChunk will fetch the logical data for a chunk, create
// the physical pieces for the chunk, and then distribute them.
func (r *Renter) managedFetchAndRepairChunk(chunk *unfinishedUploadChunk) {
	// Calculate the amount of memory needed for erasure coding. This will need
	// to be released if there's an error before erasure coding is complete.
	erasureCodingMemory := chunk.renterFile.pieceSize * uint64(chunk.renterFile.erasureCode.MinPieces())

	// Calculate the amount of memory to release due to already completed
	// pieces. This memory gets released during encryption, but needs to be
	// released if there's a failure before encryption happens.
	var pieceCompletedMemory uint64
	for i := 0; i < len(chunk.pieceUsage); i++ {
		if chunk.pieceUsage[i] {
			pieceCompletedMemory += chunk.renterFile.pieceSize + crypto.TwofishOverhead
		}
	}

	// Ensure that memory is released and that the chunk is cleaned up properly
	// after the chunk is distributed.
	//
	// Need to ensure the erasure coding memory is released as well as the
	// physical chunk memory. Physical chunk memory is released by setting
	// 'workersRemaining' to zero if the repair fails before being distributed
	// to workers. Erasure coding memory is released manually if the repair
	// fails before the erasure coding occurs.
	defer r.managedCleanUpUploadChunk(chunk)

	// Fetch the logical data for the chunk.
	err := r.managedFetchLogicalChunkData(chunk)
	if err != nil {
		// Logical data is not available, cannot upload. Chunk will not be
		// distributed to workers, therefore set workersRemaining equal to zero.
		// The erasure coding memory has not been released yet, be sure to
		// release that as well.
		chunk.logicalChunkData = nil
		chunk.workersRemaining = 0
		r.memoryManager.Return(erasureCodingMemory + pieceCompletedMemory)
		chunk.memoryReleased += erasureCodingMemory + pieceCompletedMemory
		r.log.Debugln("Fetching logical data of a chunk failed:", err)
		return
	}

	// Create the physical pieces for the data. Immediately release the logical
	// data.
	//
	// TODO: The logical data is the first few chunks of the physical data. If
	// the memory is not being handled cleanly here, we should leverage that
	// fact to reduce the total memory required to create the physical data.
	// That will also change the amount of memory we need to allocate, and the
	// number of times we need to return memory.
	chunk.physicalChunkData, err = chunk.renterFile.erasureCode.EncodeShards(chunk.logicalChunkData)
	chunk.logicalChunkData = nil
	r.memoryManager.Return(erasureCodingMemory)
	chunk.memoryReleased += erasureCodingMemory
	if err != nil {
		// Physical data is not available, cannot upload. Chunk will not be
		// distributed to workers, therefore set workersRemaining equal to zero.
		chunk.workersRemaining = 0
		r.memoryManager.Return(pieceCompletedMemory)
		chunk.memoryReleased += pieceCompletedMemory
		for i := 0; i < len(chunk.physicalChunkData); i++ {
			chunk.physicalChunkData[i] = nil
		}
		r.log.Debugln("Fetching physical data of a chunk failed:", err)
		return
	}

	// Sanity check - we should have at least as many physical data pieces as we
	// do elements in our piece usage.
	if len(chunk.physicalChunkData) < len(chunk.pieceUsage) {
		r.log.Critical("not enough physical pieces to match the upload settings of the file")
		return
	}
	// Loop through the pieces and encrypt any that are needed, while dropping
	// any pieces that are not needed.
	for i := 0; i < len(chunk.pieceUsage); i++ {
		if chunk.pieceUsage[i] {
			chunk.physicalChunkData[i] = nil
		} else {
			// Encrypt the piece.
			key := deriveKey(chunk.renterFile.masterKey, chunk.index, uint64(i))
			chunk.physicalChunkData[i] = key.EncryptBytes(chunk.physicalChunkData[i])
		}
	}
	// Return the released memory.
	if pieceCompletedMemory > 0 {
		r.memoryManager.Return(pieceCompletedMemory)
		chunk.memoryReleased += pieceCompletedMemory
	}

	// Distribute the chunk to the workers.
	r.managedDistributeChunkToWorkers(chunk)
}

// managedFetchLogicalChunkData will get the raw data for a chunk, pulling it from disk if
// possible but otherwise queueing a download.
//
// chunk.data should be passed as 'nil' to the download, to keep memory usage as
// light as possible.
func (r *Renter) managedFetchLogicalChunkData(chunk *unfinishedUploadChunk) error {
	// Only download this file if more than 25% of the redundancy is missing.
	numParityPieces := float64(chunk.piecesNeeded - chunk.minimumPieces)
	minMissingPiecesToDownload := int(numParityPieces * RemoteRepairDownloadThreshold)
	download := chunk.piecesCompleted+minMissingPiecesToDownload < chunk.piecesNeeded

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
	// loading fails. Should do this after we swap the file format, the tracking
	// data for the file should reside in the file metadata and not in a
	// separate struct.
	osFile, err := os.Open(chunk.localPath)
	if err != nil && download {
		return r.managedDownloadLogicalChunkData(chunk)
	} else if err != nil {
		return errors.Extend(err, errors.New("failed to open file locally"))
	}
	defer osFile.Close()
	// TODO: Once we have enabled support for small chunks, we should stop
	// needing to ignore the EOF errors, because the chunk size should always
	// match the tail end of the file. Until then, we ignore io.EOF.
	buf := NewDownloadDestinationBuffer(chunk.length)
	sr := io.NewSectionReader(osFile, chunk.offset, int64(chunk.length))
	_, err = buf.ReadFrom(sr)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF && download {
		r.log.Debugln("failed to read file, downloading instead:", err)
		return r.managedDownloadLogicalChunkData(chunk)
	} else if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		r.log.Debugln("failed to read file locally:", err)
		return errors.Extend(err, errors.New("failed to read file locally"))
	}
	chunk.logicalChunkData = buf

	// Data successfully read from disk.
	return nil
}

// managedCleanUpUploadChunk will check the state of the chunk and perform any
// cleanup required. This can include returning rememory and releasing the chunk
// from the map of active chunks in the chunk heap.
func (r *Renter) managedCleanUpUploadChunk(uc *unfinishedUploadChunk) {
	uc.mu.Lock()
	piecesAvailable := 0
	var memoryReleased uint64
	// Release any unnecessary pieces, counting any pieces that are
	// currently available.
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
			memoryReleased += uc.renterFile.pieceSize + crypto.TwofishOverhead
			uc.physicalChunkData[i] = nil
			// Mark this piece as taken so that we don't double release memory.
			uc.pieceUsage[i] = true
		} else {
			piecesAvailable++
		}
	}

	// Check if the chunk needs to be removed from the list of active
	// chunks. It needs to be removed if the chunk is complete, but hasn't
	// yet been released.
	chunkComplete := uc.workersRemaining == 0 && uc.piecesRegistered == 0
	released := uc.released
	if chunkComplete && !released {
		uc.released = true
	}
	uc.memoryReleased += uint64(memoryReleased)
	totalMemoryReleased := uc.memoryReleased
	uc.mu.Unlock()

	// If there are pieces available, add the standby workers to collect them.
	// Standby workers are only added to the chunk when piecesAvailable is equal
	// to zero, meaning this code will only trigger if the number of pieces
	// available increases from zero. That can only happen if a worker
	// experiences an error during upload.
	if piecesAvailable > 0 {
		uc.managedNotifyStandbyWorkers()
	}
	// If required, return the memory to the renter.
	if memoryReleased > 0 {
		r.memoryManager.Return(memoryReleased)
	}
	// If required, remove the chunk from the set of active chunks.
	if chunkComplete && !released {
		r.uploadHeap.mu.Lock()
		delete(r.uploadHeap.activeChunks, uc.id)
		r.uploadHeap.mu.Unlock()
	}
	// Sanity check - all memory should be released if the chunk is complete.
	if chunkComplete && totalMemoryReleased != uc.memoryNeeded {
		r.log.Critical("No workers remaining, but not all memory released:", uc.workersRemaining, uc.piecesRegistered, uc.memoryReleased, uc.memoryNeeded)
	}
}
