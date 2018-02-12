package renter

// workerdownload.go is responsible for coordinating the actual fetching of
// pieces, determining when to add standby workers, when to perform repairs, and
// coordinating resource management between the workers operating on a chunk.

import (
	"errors"
	"sync/atomic"
)

// cleanUp will check if the download has failed, and if not it will add any
// standby workers which need to be added. Calling cleanUp too many times is not
// harmful, however missing a call to cleanUp can lead to dealocks.
//
// NOTE: The fact that calls to cleanUp must be actively managed is probably the
// weakest part of the design of the download code.
func (udc *unfinishedDownloadChunk) cleanUp() {
	// Return any unused memory.
	udc.returnMemory()

	// Check if the chunk has failed. If so, fail the download and return any
	// remaining memory.
	if udc.workersRemaining+udc.piecesCompleted < udc.erasureCode.MinPieces() && !udc.failed {
		udc.fail(errors.New("not enough workers to continue download"))
		return
	}

	// Check if standby workers are required, and add them if so.
	chunkComplete := udc.piecesCompleted >= udc.erasureCode.MinPieces()
	desiredPiecesRegistered := udc.erasureCode.MinPieces() + udc.staticOverdrive - udc.piecesCompleted
	if !chunkComplete && udc.piecesRegistered < desiredPiecesRegistered {
		for i := 0; i < len(udc.workersStandby); i++ {
			udc.workersStandby[i].managedQueueDownloadChunk(udc)
		}
		udc.workersStandby = udc.workersStandby[:0]
	}
}

// returnMemory will check on the status of all the workers and pieces, and
// determine how much memory is safe to return to the renter. This should be
// called each time a worker returns, and also after the chunk is recovered.
func (udc *unfinishedDownloadChunk) returnMemory() {
	// The maximum amount of memory is the pieces completed plus the number of
	// workers remaining.
	initialMemory := uint64(udc.staticOverdrive+udc.erasureCode.MinPieces()) * udc.staticPieceSize
	maxMemory := uint64(udc.workersRemaining+udc.piecesCompleted) * udc.staticPieceSize
	// If the maxMemory exceeds the inital memory, set the max memory equal to
	// the initial memory.
	if maxMemory > initialMemory {
		maxMemory = initialMemory
	}
	// If enough pieces have completed, max memory is the number of registered
	// pieces plus the number of completed pieces.
	if udc.piecesCompleted >= udc.erasureCode.MinPieces() {
		// udc.piecesRegistered is guaranteed to be at most equal to the number
		// of overdrive pieces, meaning it will be equal to or less than
		// initalMemory.
		maxMemory = uint64(udc.piecesCompleted+udc.piecesRegistered) * udc.staticPieceSize
	}
	// If the chunk recovery has completed, the maximum number of pieces is the
	// number of registered.
	if udc.recoveryComplete {
		maxMemory = uint64(udc.piecesRegistered) * udc.staticPieceSize
	}
	// Return any memory we don't need.
	if uint64(udc.memoryAllocated) > maxMemory {
		udc.download.memoryManager.Return(udc.memoryAllocated - maxMemory)
		udc.memoryAllocated = maxMemory
	}
}

// removeWorker will release a download chunk that the worker had queued up.
// This function should be called any time that a worker completes work.
func (udc *unfinishedDownloadChunk) removeWorker() {
	udc.workersRemaining--
	udc.cleanUp()
}

// managedUnregisterWorker will remove the worker from an unfinished download
// chunk, and then un-register the pieces that it grabbed. This function should
// only be called when a worker download fails.
func (udc *unfinishedDownloadChunk) managedUnregisterWorker(w *worker) {
	udc.mu.Lock()
	udc.piecesRegistered--
	udc.pieceUsage[udc.staticChunkMap[w.contract.ID].index] = false
	udc.removeWorker()
	udc.mu.Unlock()
}

// dropDownloadChunks will release all of the chunks that the worker is
// currently working on.
func (w *worker) dropDownloadChunks() {
	for i := 0; i < len(w.downloadChunks); i++ {
		w.downloadChunks[i].removeWorker()
	}
	w.downloadChunks = w.downloadChunks[:0]
}

// managedProcessDownloadChunk will take a potential download chunk, figure out
// if there is work to do, and then perform any registration or processing with
// the chunk before returning the chunk to the caller.
//
// If no immediate action is required, 'nil' will be returned.
func (w *worker) managedProcessDownloadChunk(udc *unfinishedDownloadChunk) *unfinishedDownloadChunk {
	udc.mu.Lock()
	defer udc.mu.Unlock()

	// Determine whether the worker needs to drop the chunk.
	chunkComplete := udc.piecesCompleted >= udc.erasureCode.MinPieces()
	chunkFailed := udc.piecesCompleted+udc.workersRemaining < udc.erasureCode.MinPieces()
	chunkData, workerHasPiece := udc.staticChunkMap[w.contract.ID]
	if chunkComplete || chunkFailed || w.onDownloadCooldown() || !workerHasPiece {
		udc.removeWorker()
		return nil
	}

	// TODO: This is where we would put filters based on worker latency, worker
	// price, worker throughput, etc. There's a lot of fancy stuff we can do
	// with filtering to make sure that for any given chunk we always use the
	// optimal set of workers, and this is the spot where most of the filtering
	// will happen.
	//
	// Workers that do not meet the extra criteria are not discarded but rather
	// put on standby, so that they can step in if the workers that do meet the
	// extra criteria fail or otherwise prove insufficient.
	meetsExtraCriteria := true

	// TODO: There's going to need to be some method for relaxing criteria after
	// the first wave of workers are sent off. If the first waves of workers
	// fail, the next wave need to realize that they shouldn't immediately go on
	// standby because for some reason there were failures in the first wave and
	// now the second/etc. wave of workers is needed.

	// Figure out if this chunk needs another worker actively downloading
	// pieces. The number of workers that should be active simultaneously on
	// this chunk is the minimum number of pieces required for recovery plus the
	// number of overdrive workers (typically zero). For our purposes, completed
	// pieces count as active workers, though the workers have actually
	// finished.
	piecesInProgress := udc.piecesRegistered + udc.piecesCompleted
	desiredPiecesInProgress := udc.erasureCode.MinPieces() + udc.staticOverdrive
	workersDesired := piecesInProgress < desiredPiecesInProgress

	if workersDesired && meetsExtraCriteria {
		// Worker can be useful. Register the worker and return the chunk for
		// downloading.
		udc.piecesRegistered++
		udc.pieceUsage[chunkData.index] = true
		return udc
	}
	// Worker is not needed unless another worker fails, so put this worker on
	// standby for this chunk.
	udc.workersStandby = append(udc.workersStandby, w)
	return nil
}

// managedDownload will perform some download work.
func (w *worker) managedDownload(udc *unfinishedDownloadChunk) {
	// Process this chunk. If the worker is not fit to do the download, or is
	// put on standby, 'nil' will be returned.
	udc = w.managedProcessDownloadChunk(udc)
	if udc == nil {
		return
	}

	// Fetch the sector.
	d, err := w.renter.hostContractor.Downloader(w.contract.ID, w.renter.tg.StopChan())
	if err != nil {
		udc.managedUnregisterWorker(w)
		return
	}
	defer d.Close()
	data, err := d.Sector(udc.staticChunkMap[w.contract.ID].root)
	if err != nil {
		udc.managedUnregisterWorker(w)
		return
	}
	// TODO: Instead of adding the whole sector after the download completes,
	// have the 'd.Sector' call add to this value ongoing as the sector comes
	// in. Perhaps even include the data from creating the downloader and other
	// data sent to and received from the host (like signatures) that aren't
	// actually payload data.
	atomic.AddUint64(&udc.download.atomicTotalDataTransfered, udc.staticPieceSize)

	// Mark the piece as completed. Perform chunk recovery if we have enough
	// pieces to do so. Chunk recovery is an expensive operation that should be
	// performed in a separate thread as to not block the worker.
	udc.mu.Lock()
	udc.piecesCompleted++
	udc.piecesRegistered--
	if udc.piecesCompleted <= udc.erasureCode.MinPieces() {
		udc.physicalChunkData[udc.staticChunkMap[w.contract.ID].index] = data
	}
	if udc.piecesCompleted == udc.erasureCode.MinPieces() {
		go udc.threadedRecoverLogicalData()
	}
	udc.removeWorker()
	udc.mu.Unlock()
}
