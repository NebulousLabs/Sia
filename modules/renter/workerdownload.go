package renter

// TODO: In the actual download function, no attention is paid to the worker
// timeout stuff.

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// dropDownloadChunk will release a download chunk that the worker had queued
// up.
func (w *worker) dropDownloadChunk(udc *unfinishedDownloadChunk) {
	udc.workersRemaining--

	// Check if the download has failed. If so, fail the download and return any
	// remaining memory.
	if udc.workersRemaining + udc.piecesCompleted < udc.erasureCode.MinPieces() - 1 {
		udc.fail()
		if udc.memoryAllocated > 0 {
			w.renter.managedMemoryReturn(udc.memoryAllocated)
			udc.memoryAllocated = 0
		}
	}
}

// dropDownloadChunks will release all of the chunks that the worker is
// currently working on.
func (w *worker) dropDownloadChunks() {
	for i := 0; i < len(w.downloadChunks); i++ {
		w.dropDownloadChunk(w.downloadChunks[i])
	}
	w.downloadChunks = w.downloadChunks[:0]
}

// managedKillDownloading will drop all of the pieces given to the worker.
func (w *worker) managedKillDownloading(udc *unfinishedDownloadChunk) {
	w.mu.Lock()
	w.dropDownloadChunks()
	w.downloadTerminated = true
	w.mu.Unlock()
}

// processDownloadChunk will take a potential download chunk, figure out if
// there is work to do, and then perform any registration or processing with the
// chunk before returning the chunk to the caller.
//
// If no immediate action is required, 'nil' will be returned.
//
// TODO: Currently the hostdb + worker have no way of understanding their
// latency. When a worker is able to understand their latency though, a check
// should be added to this loop that automatically adds the worker as a standby
// worker if the latency for this worker is higher than the target latency for
// this chunk.
func (w *worker) processDownloadChunk(udc *unfinishedDownloadChunk) *unfinishedDownloadChunk {
	// TODO: Compare worker latency here to see if it is below the latencyTarget
	// for the chunk.
	meetsLatencyTarget := true

	// TODO: Any other things that may make the worker standby such as price can
	// be handled here as well.

	// TODO: Consider the timeout stuff here. If the worker has been failing a
	// bunch, track that.

	// If this chunk has not had enough pieces regiseterd yet, register this
	// worker. If this worker is not able to help out, subtract the worker from
	// 'workersRemaining'. Otherwise, just put the worker on standby.
	pieceIndex, exists := udc.chunkMap[w.contract.ID]
	udc.mu.Lock()
	useful := exists && !udc.failed
	if udc.piecesRegistered < udc.erasureCode.MinPieces()+overdrive && !udc.pieceUsage[pieceIndex] && meetsLatencyTarget && useful {
		// Worker can be useful. Register the worker and return the chunk for
		// downloading.
		udc.piecesRegistered++
		udc.pieceUsage[pieceIndex] = true
		udc.mu.Unlock()
		return udc
	} else if !udc.piecesCompleted[pieceIndex] && useful {
		// Worker is not needed, but the worker's piece is also not completed,
		// so put the worker on standby.
		udc.workersStandby = append(udc.workersStandby, w)
	} else {
		// This worker is unable to work on the chunk.
		w.dropDownloadChunk(udc)
	}
	udc.mu.Unlock()
	return nil
}

// managedDownload will perform some download work.
func (w *worker) managedDownload(udc *unfinishedDownloadChunk) {
	// After finishing the download, error or not, drop the chunk.
	defer func() {
		udc.mu.Lock()
		udc.dropDownloadChunk(udc)
		udc.mu.Unlock()
	}()

	d, err := w.renter.hostContractor.Downloader(w.contract.ID, w.renter.tg.StopChan())
	if err != nil {
		return
	}
	defer d.Close()

	data, err := d.Sector(dw.dataRoot)
	if err != nil {
		return
	}
	udc.mu.Lock()
	piecesCompleted++
	totalCompleted := udc.piecesCompleted
	if totalCompleted <= udc.erasureCode.MinPieces() {
		udc.physicalChunkData[udc.chunkMap[w.contract.ID]] = data
	}
	udc.mu.Unlock()

	// Recovery only needs to be completed once, so only perform recovery if the
	// totalCompleted is equal to the minimum number of pieces.
	if totalCompleted == udc.erasureCode.MinPieces() {
		// TODO: Begin recovery.
	}
}
