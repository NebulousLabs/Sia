package renter

import (
	"time"
)

// dropChunk will remove a worker from the responsibility of tracking a chunk.
func (w *worker) dropChunk(uc *unfinishedChunk) {
	uc.mu.Lock()
	uc.workersRemaining--
	uc.mu.Unlock()
	w.renter.managedReleaseIdleChunkPieces(uc)
	w.renter.heapWG.Done()
}

// dropUploadChunks will release all of the upload chunks that the worker has
// received.
func (w *worker) dropUploadChunks() {
	for i := 0; i < len(w.unprocessedChunks); i++ {
		w.dropChunk(w.unprocessedChunks[i])
	}
	w.unprocessedChunks = w.unprocessedChunks[:0]
	for i := 0; i < len(w.standbyChunks); i++ {
		w.dropChunk(w.standbyChunks[i])
	}
	w.standbyChunks = w.standbyChunks[:0]
}

// managedKillUploading will disable all uploading for the worker.
func (w *worker) managedKillUploading() {
	w.mu.Lock()
	w.dropUploadChunks()
	w.terminated = true
	w.mu.Unlock()
}

// managedNextChunk will pull the next potential chunk out of the worker's work queue
// for uploading.
func (w *worker) managedNextChunk() (nextChunk *unfinishedChunk, pieceIndex uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Loop through the unprocessed chunks and find some work to do.
	for range w.unprocessedChunks {
		// Pull a chunk off of the unprocessed chunks stack.
		chunk := w.unprocessedChunks[0]
		w.unprocessedChunks = w.unprocessedChunks[1:]
		nextChunk, pieceIndex := w.processChunk(chunk)
		if nextChunk != nil {
			return nextChunk, pieceIndex
		}
	}

	// Loop through the standby chunks to see if there is work to do.
	for range w.standbyChunks {
		chunk := w.standbyChunks[0]
		w.standbyChunks = w.standbyChunks[1:]
		nextChunk, pieceIndex := w.processChunk(chunk)
		if nextChunk != nil {
			return nextChunk, pieceIndex
		}
	}

	// No work found, try again later.
	return nil, 0
}

// processChunk will process a chunk from the worker chunk queue.
func (w *worker) processChunk(uc *unfinishedChunk) (nextChunk *unfinishedChunk, pieceIndex uint64) {
	// Determine what sort of help this chunk needs.
	uc.mu.Lock()
	_, candidateHost := uc.unusedHosts[w.hostPubKey.String()]
	chunkComplete := uc.piecesNeeded <= uc.piecesCompleted
	needsHelp := uc.piecesNeeded > uc.piecesCompleted+uc.piecesRegistered

	// If the chunk does not need help from this worker, release the chunk.
	if chunkComplete || !candidateHost || !w.contract.GoodForUpload {
		// This worker no longer needs to track this chunk.
		uc.mu.Unlock()
		w.dropChunk(uc)
		return nil, 0
	}

	// If the chunk needs help from this worker, find a piece to upload and
	// return the stats for that piece.
	index := 0
	if needsHelp {
		// Select a piece and mark that a piece has been selected.
		for i := 0; i < len(uc.pieceUsage); i++ {
			if !uc.pieceUsage[i] {
				index = i
				uc.pieceUsage[i] = true
				break
			}
		}
		delete(uc.unusedHosts, w.hostPubKey.String())
		uc.piecesRegistered++
		uc.mu.Unlock()
		return uc, uint64(index)
	}
	uc.mu.Unlock()

	// The chunk could need help from this worker, but only if other workers who
	// are performing uploads experience failures. Put this chunk on standby.
	w.standbyChunks = append(w.standbyChunks, uc)
	return nil, 0
}

// managedQueueChunkRepair will take a chunk and add it to the worker's repair stack.
func (w *worker) managedQueueChunkRepair(uc *unfinishedChunk) {
	// Check that the worker is allowed to be uploading.
	contract, exists := w.renter.hostContractor.ContractByID(w.contract.ID)
	w.mu.Lock()
	// Figure out how long the worker would need to be on cooldown.
	requiredCooldown := uploadFailureCooldown
	for i := 0; i < w.uploadConsecutiveFailures && i < maxConsecutivePenalty; i++ {
		requiredCooldown *= 2
	}
	onCooldown := time.Now().Before(w.uploadRecentFailure.Add(requiredCooldown))
	if !exists || !contract.GoodForUpload || w.terminated || onCooldown {
		// The worker should not be uploading, remove the chunk.
		w.dropChunk(uc)
		w.mu.Unlock()
		return
	}
	w.unprocessedChunks = append(w.unprocessedChunks, uc)
	w.mu.Unlock()

	// Send a signal informing the work thread that there is work.
	select {
	case w.uploadChan <- struct{}{}:
	default:
	}
}

// uploadFailed is called if a worker failed to upload part of an unfinished
// chunk.
func (w *worker) uploadFailed(uc *unfinishedChunk, pieceIndex uint64) {
	w.uploadRecentFailure = time.Now()
	w.uploadConsecutiveFailures++
	uc.mu.Lock()
	uc.piecesRegistered--
	uc.pieceUsage[pieceIndex] = false
	uc.mu.Unlock()
	w.dropChunk(uc)
	w.dropUploadChunks()
}

// managedUpload will perform some upload work.
func (w *worker) managedUpload(uc *unfinishedChunk, pieceIndex uint64) {
	// Open an editing connection to the host.
	e, err := w.renter.hostContractor.Editor(w.contract.ID, w.renter.tg.StopChan())
	if err != nil {
		w.renter.log.Debugln("Worker failed to acquire an editor:", err)
		w.uploadFailed(uc, pieceIndex)
		return
	}
	defer e.Close()

	// Perform the upload, and update the failure stats based on the success of
	// the upload attempt.
	root, err := e.Upload(uc.physicalChunkData[pieceIndex])
	if err != nil {
		w.renter.log.Debugln("Worker failed to upload via the editor:", err)
		w.mu.Lock()
		w.uploadFailed(uc, pieceIndex)
		w.mu.Unlock()
		return
	}
	w.mu.Lock()
	w.uploadConsecutiveFailures = 0
	w.mu.Unlock()

	// Update the renter metadata.
	addr := e.Address()
	endHeight := e.EndHeight()
	id := w.renter.mu.Lock()
	uc.renterFile.mu.Lock()
	contract, exists := uc.renterFile.contracts[w.contract.ID]
	if !exists {
		contract = fileContract{
			ID:          w.contract.ID,
			IP:          addr,
			WindowStart: endHeight,
		}
	}
	contract.Pieces = append(contract.Pieces, pieceData{
		Chunk:      uc.index,
		Piece:      pieceIndex,
		MerkleRoot: root,
	})
	uc.renterFile.contracts[w.contract.ID] = contract
	w.renter.saveFile(uc.renterFile)
	uc.renterFile.mu.Unlock()
	w.renter.mu.Unlock(id)

	// Upload is complete. Update the state of the chunk and the renter's memory
	// available to reflect the completed upload.
	uc.mu.Lock()
	releaseSize := len(uc.physicalChunkData[pieceIndex])
	uc.piecesRegistered--
	uc.piecesCompleted++
	uc.physicalChunkData[pieceIndex] = nil
	uc.memoryReleased += uint64(releaseSize)
	uc.mu.Unlock()
	w.renter.managedMemoryAvailableAdd(uint64(releaseSize))
	w.dropChunk(uc)
}
