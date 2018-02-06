package renter

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// TODO: Clean up the concurrency patterns / assumptions with regards to the
// drop function.

// dropDownloadChunk will release a download chunk that the worker had queued
// up.
//
// NOTE: This function expects that the udc lock is held in addition to the
// worker lock.
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
	udc.mu.Lock()
	defer udc.mu.Unlock()

	// TODO: Clean up the concurrency patterns and stuff with regards to the
	// dopr function.

	// Determine whether the worker needs to drop the chunk.
	chunkComplete := udc.piecesCompleted >= udc.erasureCode.MinPieces()
	pieceIndex, workerHasPiece := udc.chunkMap[w.contract.ID]
	if chunkComplete || chunkFailed || w.onDownloadCooldown() || !workerHasPiece {
		w.dropDownloadChunk(udc)
		return nil
	}

	// TODO TODO TODO: pick up here.


	// TODO: Compare worker latency here to see if it is below the latencyTarget
	// for the chunk.
	meetsLatencyTarget := true

	// TODO: Any other things that may make the worker standby such as price can
	// be handled here as well.

	// TODO: Consider the timeout stuff here. If the worker has been failing a
	// bunch, track that.

	// If this chunk has not had enough pieces regiseterd yet, register this
	// worker. Otherwies put the worker on standby.
	if udc.piecesRegistered < udc.erasureCode.MinPieces()+overdrive && !udc.pieceUsage[pieceIndex] && meetsLatencyTarget {
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
	// After finishing the download, error or not, drop the chunk.
	defer func() {
		udc.mu.Lock()
		udc.dropDownloadChunk(udc)
		udc.mu.Unlock()
	}()

	// Fetch the sector.
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
