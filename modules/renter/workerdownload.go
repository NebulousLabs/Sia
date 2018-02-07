package renter

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// workerdownload.go is responsible for coordinating the actual fetching of
// pieces, determining when to add standby workers, when to perform repairs, and
// coordinating resource management between the workers operating on a chunk.
//
// NOTE: the 'piecesRegistered' field of the unfinishedDownloadChunk will only
// be decremented if a piece download fails or is un-needed to perform recovery.
// Otherwise it just keeps incrementing.
//
// TODO: all remaining TODOs in this file relate to relaxing standby
// requirements for workers. Currently, we don't have any standby requirements,
// so there's not really any code to write, need to get latency and pricing
// situation figured out.

// removeWorker will release a download chunk that the worker had queued up.
// This function should be called any time that a worker completes work.
//
// NOTE: The udc is expected to be locked, the worker is expected to be
// unlocked.
func (udc *unfinishedDownloadChunk) removeWorker(w *worker) {
	udc.workersRemaining--

	// The maximum amount of memory is the pieces completed plus the number of
	// workers remaining.
	maxMemory := (udc.workersRemaining + udc.piecesCompleted) * udc.pieceSize
	// If enough pieces have completed, max memory is the number of registered
	// pieces plus the number of completed pieces.
	if udc.piecesCompleted >= udc.erasureCode.MinPieces() {
		maxMemory = udc.piecesRegistered * udc.pieceSize
	}
	// If the chunk recovery has completed, the maximum number of pieces is the
	// number of registered pieces minus the number of completed pieces (the
	// completed pieces have been recovered and the memory is no longer needed).
	if udc.recoveryComplete {
		maxMemory = (udc.piecesRegistered-udc.piecesCompleted)*udc.pieceSize
	}
	// Return any memory we don't need.
	if udc.memoryAllocated > maxMemory {
		w.renter.managedMemoryReturn(udc.memoryAllocated-maxMemory)
		udc.memoryAllocated = maxMemory
	}

	// Check if the chunk has failed. If so, fail the download and return any
	// remaining memory.
	if udc.workersRemaining + udc.piecesCompleted < udc.erasureCode.MinPieces() - 1 && !udc.failed {
		udc.fail()
		return
	}

	// Add the chunk as work to any standby workers.
	if udc.workersRemaining + udc.piecesCompleted - len(udc.workersStandby) < udc.erasureCode.MinPieces()+udc.overdrive {
		for i := 0; i < len(udc.workersStandby); i++ {
			udc.workersStandby[i].managedQueueDownloadChunk(udc)
		}
		udc.workersStandby = udc.workersStandby[:0]
	}
}

// managedUnregisterWorker will remove the worker from an unfinished download
// chunk, and then un-register the pieces that it grabbed. This function should
// only be called when a worker download fails.
func (udc *unfinishedDownloadChunk) managedUnregisterWorker(w *worker) {
	udc.mu.Lock()
	udc.piecesRegisterd--
	udc.pieceUsage[udc.chunkMap[w.contract.ID]] = false
	udc.removeWorker(w)
	udc.mu.Unlock()
}

// dropDownloadChunks will release all of the chunks that the worker is
// currently working on.
func (w *worker) dropDownloadChunks() {
	for i := 0; i < len(w.downloadChunks); i++ {
		w.downloadChunks[i].removeWorker(w)
	}
	w.downloadChunks = w.downloadChunks[:0]
}

// processDownloadChunk will take a potential download chunk, figure out if
// there is work to do, and then perform any registration or processing with the
// chunk before returning the chunk to the caller.
//
// If no immediate action is required, 'nil' will be returned.
func (w *worker) processDownloadChunk(udc *unfinishedDownloadChunk) *unfinishedDownloadChunk {
	udc.mu.Lock()
	defer udc.mu.Unlock()

	// Determine whether the worker needs to drop the chunk.
	chunkComplete := udc.piecesCompleted >= udc.erasureCode.MinPieces()
	pieceIndex, workerHasPiece := udc.chunkMap[w.contract.ID]
	if chunkComplete || chunkFailed || w.onDownloadCooldown() || !workerHasPiece {
		udc.removeWorker(w)
		return nil
	}

	// TODO: Compare worker latency here to see if it is below the latencyTarget
	// for the chunk.
	//
	// TODO: Any other things that may result in the worker being automatically
	// set to standby (such as price) should be handled here as well.
	meetsLatencyTarget := true

	// If this chunk has not had enough pieces regiseterd yet, register this
	// worker. Otherwies put the worker on standby.
	if udc.piecesRegistered < udc.erasureCode.MinPieces()+udc.overdrive && !udc.pieceUsage[pieceIndex] && meetsLatencyTarget {
		// Worker can be useful. Register the worker and return the chunk for
		// downloading.
		udc.piecesRegistered++
		udc.pieceUsage[pieceIndex] = true
		return udc
	}
	// Worker is not needed unless another worker fails, so put this worker on
	// standby for this chunk.
	udc.workersStandby = append(udc.workersStandby, w)
	return nil
}

// managedDownload will perform some download work.
func (w *worker) managedDownload(udc *unfinishedDownloadChunk) {
	// Fetch the sector.
	d, err := w.renter.hostContractor.Downloader(w.contract.ID, w.renter.tg.StopChan())
	if err != nil {
		udc.managedUnregisterWorker(w)
		return
	}
	defer d.Close()
	data, err := d.Sector(dw.dataRoot)
	if err != nil {
		udc.managedUnregisterWorker(w)
		return
	}

	udc.mu.Lock()
	udc.piecesCompleted++
	udc.removeWorker(w)
	if udc.piecesCompleted <= udc.erasureCode.MinPieces() {
		udc.physicalChunkData[udc.chunkMap[w.contract.ID]] = data
		if udc.piecesCompleted == udc.erasureCode.MinPieces() {
			udc.recoverAndWrite()
		}
	}
	udc.mu.Unlock()
}
