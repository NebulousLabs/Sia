package renter

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// managedDropChunk will remove a worker from the responsibility of tracking a chunk.
//
// This function is managed instead of static because it is against convention
// to be calling functions on other objects (in this case, the renter) while
// holding a lock.
func (w *worker) managedDropChunk(uc *unfinishedUploadChunk) {
	uc.mu.Lock()
	uc.workersRemaining--
	uc.mu.Unlock()
	w.renter.managedCleanUpUploadChunk(uc)
}

// managedDropUploadChunks will release all of the upload chunks that the worker
// has received.
func (w *worker) managedDropUploadChunks() {
	// Make a copy of the slice under lock, clear the slice, then drop the
	// chunks without a lock (managed function).
	var chunksToDrop []*unfinishedUploadChunk
	w.mu.Lock()
	for i := 0; i < len(w.unprocessedChunks); i++ {
		chunksToDrop = append(chunksToDrop, w.unprocessedChunks[i])
	}
	w.unprocessedChunks = w.unprocessedChunks[:0]
	w.mu.Unlock()

	for i := 0; i < len(chunksToDrop); i++ {
		w.managedDropChunk(chunksToDrop[i])
	}
}

// managedKillUploading will disable all uploading for the worker.
func (w *worker) managedKillUploading() {
	// Mark the worker as disabled so that incoming chunks are rejected.
	w.mu.Lock()
	w.uploadTerminated = true
	w.mu.Unlock()

	// After the worker is marked as disabled, clear out all of the chunks.
	w.managedDropUploadChunks()
}

// managedNextUploadChunk will pull the next potential chunk out of the worker's
// work queue for uploading.
func (w *worker) managedNextUploadChunk() (nextChunk *unfinishedUploadChunk, pieceIndex uint64) {
	// Loop through the unprocessed chunks and find some work to do.
	for {
		// Pull a chunk off of the unprocessed chunks stack.
		w.mu.Lock()
		if len(w.unprocessedChunks) <= 0 {
			w.mu.Unlock()
			break
		}
		chunk := w.unprocessedChunks[0]
		w.unprocessedChunks = w.unprocessedChunks[1:]
		w.mu.Unlock()

		// Process the chunk and return it if valid.
		nextChunk, pieceIndex := w.managedProcessUploadChunk(chunk)
		if nextChunk != nil {
			return nextChunk, pieceIndex
		}
	}
	return nil, 0 // no work found
}

// managedQueueUploadChunk will take a chunk and add it to the worker's repair
// stack.
func (w *worker) managedQueueUploadChunk(uc *unfinishedUploadChunk) {
	// Check that the worker is allowed to be uploading before grabbing the
	// worker lock.
	utility, exists := w.renter.hostContractor.ContractUtility(w.contract.HostPublicKey)
	goodForUpload := exists && utility.GoodForUpload
	w.mu.Lock()
	if !goodForUpload || w.uploadTerminated || w.onUploadCooldown() {
		// The worker should not be uploading, remove the chunk.
		w.mu.Unlock()
		w.managedDropChunk(uc)
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

// managedUpload will perform some upload work.
func (w *worker) managedUpload(uc *unfinishedUploadChunk, pieceIndex uint64) {
	// Open an editing connection to the host.
	e, err := w.renter.hostContractor.Editor(w.contract.HostPublicKey, w.renter.tg.StopChan())
	if err != nil {
		w.renter.log.Debugln("Worker failed to acquire an editor:", err)
		w.managedUploadFailed(uc, pieceIndex)
		return
	}
	defer e.Close()

	// Perform the upload, and update the failure stats based on the success of
	// the upload attempt.
	root, err := e.Upload(uc.physicalChunkData[pieceIndex])
	if err != nil {
		w.renter.log.Debugln("Worker failed to upload via the editor:", err)
		w.managedUploadFailed(uc, pieceIndex)
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
	w.renter.memoryManager.Return(uint64(releaseSize))
	w.renter.managedCleanUpUploadChunk(uc)
}

// onUploadCooldown returns true if the worker is on cooldown from failed
// uploads.
func (w *worker) onUploadCooldown() bool {
	requiredCooldown := uploadFailureCooldown
	for i := 0; i < w.uploadConsecutiveFailures && i < maxConsecutivePenalty; i++ {
		requiredCooldown *= 2
	}
	return time.Now().Before(w.uploadRecentFailure.Add(requiredCooldown))
}

// managedProcessUploadChunk will process a chunk from the worker chunk queue.
func (w *worker) managedProcessUploadChunk(uc *unfinishedUploadChunk) (nextChunk *unfinishedUploadChunk, pieceIndex uint64) {
	// Determine the usability value of this worker.
	utility, exists := w.renter.hostContractor.ContractUtility(w.contract.HostPublicKey)
	goodForUpload := exists && utility.GoodForUpload
	w.mu.Lock()
	onCooldown := w.onUploadCooldown()
	w.mu.Unlock()

	// Determine what sort of help this chunk needs.
	uc.mu.Lock()
	_, candidateHost := uc.unusedHosts[w.hostPubKey.String()]
	chunkComplete := uc.piecesNeeded <= uc.piecesCompleted
	needsHelp := uc.piecesNeeded > uc.piecesCompleted+uc.piecesRegistered
	// If the chunk does not need help from this worker, release the chunk.
	if chunkComplete || !candidateHost || !goodForUpload || onCooldown {
		// This worker no longer needs to track this chunk.
		uc.mu.Unlock()
		w.managedDropChunk(uc)
		return nil, 0
	}

	// If the worker does not need help, add the worker to the sent of standby
	// chunks.
	if !needsHelp {
		uc.workersStandby = append(uc.workersStandby, w)
		uc.mu.Unlock()
		w.renter.managedCleanUpUploadChunk(uc)
		return nil, 0
	}

	// If the chunk needs help from this worker, find a piece to upload and
	// return the stats for that piece.
	//
	// Select a piece and mark that a piece has been selected.
	index := -1
	for i := 0; i < len(uc.pieceUsage); i++ {
		if !uc.pieceUsage[i] {
			index = i
			uc.pieceUsage[i] = true
			break
		}
	}
	if index == -1 {
		build.Critical("worker was supposed to upload but couldn't find unused piece")
		uc.mu.Unlock()
		w.managedDropChunk(uc)
		return nil, 0
	}
	delete(uc.unusedHosts, w.hostPubKey.String())
	uc.piecesRegistered++
	uc.workersRemaining--
	uc.mu.Unlock()
	return uc, uint64(index)
}

// managedUploadFailed is called if a worker failed to upload part of an unfinished
// chunk.
func (w *worker) managedUploadFailed(uc *unfinishedUploadChunk, pieceIndex uint64) {
	// Mark the failure in the worker if the gateway says we are online. It's
	// not the worker's fault if we are offline.
	if w.renter.g.Online() {
		w.mu.Lock()
		w.uploadRecentFailure = time.Now()
		w.uploadConsecutiveFailures++
		w.mu.Unlock()
	}

	// Unregister the piece from the chunk and hunt for a replacement.
	uc.mu.Lock()
	uc.piecesRegistered--
	uc.pieceUsage[pieceIndex] = false
	uc.mu.Unlock()

	// Notify the standby workers of the chunk
	uc.managedNotifyStandbyWorkers()
	w.renter.managedCleanUpUploadChunk(uc)

	// Because the worker is now on cooldown, drop all remaining chunks.
	w.managedDropUploadChunks()
}
