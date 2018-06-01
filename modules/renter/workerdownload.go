package renter

// workerdownload.go is responsible for coordinating the actual fetching of
// pieces, determining when to add standby workers, when to perform repairs, and
// coordinating resource management between the workers operating on a chunk.

import (
	"sync/atomic"
	"time"
)

// managedDownload will perform some download work.
func (w *worker) managedDownload(udc *unfinishedDownloadChunk) {
	// Process this chunk. If the worker is not fit to do the download, or is
	// put on standby, 'nil' will be returned. After the chunk has been
	// processed, the worker will be registered with the chunk.
	//
	// If 'nil' is returned, it is either because the worker has been removed
	// from the chunk entirely, or because the worker has been put on standby.
	udc = w.ownedProcessDownloadChunk(udc)
	if udc == nil {
		return
	}
	// Worker is being given a chance to work. After the work is complete,
	// whether successful or failed, the worker needs to be removed.
	defer udc.managedRemoveWorker()

	// Fetch the sector. If fetching the sector fails, the worker needs to be
	// unregistered with the chunk.
	d, err := w.renter.hostContractor.Downloader(w.contract.HostPublicKey, w.renter.tg.StopChan())
	if err != nil {
		w.renter.log.Debugln("worker failed to create downloader:", err)
		udc.managedUnregisterWorker(w)
		return
	}
	defer d.Close()
	data, err := d.Sector(udc.staticChunkMap[string(w.contract.HostPublicKey.Key)].root)
	if err != nil {
		w.renter.log.Debugln("worker failed to download sector:", err)
		udc.managedUnregisterWorker(w)
		return
	}
	// TODO: Instead of adding the whole sector after the download completes,
	// have the 'd.Sector' call add to this value ongoing as the sector comes
	// in. Perhaps even include the data from creating the downloader and other
	// data sent to and received from the host (like signatures) that aren't
	// actually payload data.
	atomic.AddUint64(&udc.download.atomicTotalDataTransferred, udc.staticPieceSize)

	// Mark the piece as completed. Perform chunk recovery if we newly have
	// enough pieces to do so. Chunk recovery is an expensive operation that
	// should be performed in a separate thread as to not block the worker.
	udc.mu.Lock()
	udc.piecesCompleted++
	udc.piecesRegistered--
	if udc.piecesCompleted <= udc.erasureCode.MinPieces() {
		udc.physicalChunkData[udc.staticChunkMap[string(w.contract.HostPublicKey.Key)].index] = data
	}
	if udc.piecesCompleted == udc.erasureCode.MinPieces() {
		go udc.threadedRecoverLogicalData()
	}
	udc.mu.Unlock()
}

// managedKillDownloading will drop all of the download work given to the
// worker, and set a signal to prevent the worker from accepting more download
// work.
//
// The chunk cleanup needs to occur after the worker mutex is released so that
// the worker is not locked while chunk cleanup is happening.
func (w *worker) managedKillDownloading() {
	w.downloadMu.Lock()
	var removedChunks []*unfinishedDownloadChunk
	for i := 0; i < len(w.downloadChunks); i++ {
		removedChunks = append(removedChunks, w.downloadChunks[i])
	}
	w.downloadChunks = w.downloadChunks[:0]
	w.downloadTerminated = true
	w.downloadMu.Unlock()
	for i := 0; i < len(removedChunks); i++ {
		removedChunks[i].managedRemoveWorker()
	}
}

// managedNextDownloadChunk will pull the next potential chunk out of the work
// queue for downloading.
func (w *worker) managedNextDownloadChunk() *unfinishedDownloadChunk {
	w.downloadMu.Lock()
	defer w.downloadMu.Unlock()

	if len(w.downloadChunks) == 0 {
		return nil
	}
	nextChunk := w.downloadChunks[0]
	w.downloadChunks = w.downloadChunks[1:]
	return nextChunk
}

// managedQueueDownloadChunk adds a chunk to the worker's queue.
func (w *worker) managedQueueDownloadChunk(udc *unfinishedDownloadChunk) {
	// Accept the chunk unless the worker has been terminated. Accepting the
	// chunk needs to happen under the same lock as fetching the termination
	// status.
	w.downloadMu.Lock()
	terminated := w.downloadTerminated
	if !terminated {
		// Accept the chunk and issue a notification to the master thread that
		// there is a new download.
		w.downloadChunks = append(w.downloadChunks, udc)
		select {
		case w.downloadChan <- struct{}{}:
		default:
		}
	}
	w.downloadMu.Unlock()

	// If the worker has terminated, remove it from the udc. This call needs to
	// happen without holding the worker lock.
	if terminated {
		udc.managedRemoveWorker()
	}
}

// managedUnregisterWorker will remove the worker from an unfinished download
// chunk, and then un-register the pieces that it grabbed. This function should
// only be called when a worker download fails.
func (udc *unfinishedDownloadChunk) managedUnregisterWorker(w *worker) {
	udc.mu.Lock()
	udc.piecesRegistered--
	udc.pieceUsage[udc.staticChunkMap[string(w.contract.HostPublicKey.Key)].index] = false
	udc.mu.Unlock()
}

// ownedOnDownloadCooldown returns true if the worker is on cooldown from failed
// downloads. This function should only be called by the master worker thread,
// and does not require any mutexes.
func (w *worker) ownedOnDownloadCooldown() bool {
	requiredCooldown := downloadFailureCooldown
	for i := 0; i < w.ownedDownloadConsecutiveFailures && i < maxConsecutivePenalty; i++ {
		requiredCooldown *= 2
	}
	return time.Now().Before(w.ownedDownloadRecentFailure.Add(requiredCooldown))
}

// ownedProcessDownloadChunk will take a potential download chunk, figure out if
// there is work to do, and then perform any registration or processing with the
// chunk before returning the chunk to the caller.
//
// If no immediate action is required, 'nil' will be returned.
func (w *worker) ownedProcessDownloadChunk(udc *unfinishedDownloadChunk) *unfinishedDownloadChunk {
	// Determine whether the worker needs to drop the chunk. If so, remove the
	// worker and return nil. Worker only needs to be removed if worker is being
	// dropped.
	udc.mu.Lock()
	chunkComplete := udc.piecesCompleted >= udc.erasureCode.MinPieces()
	chunkFailed := udc.piecesCompleted+udc.workersRemaining < udc.erasureCode.MinPieces()
	pieceData, workerHasPiece := udc.staticChunkMap[string(w.contract.HostPublicKey.Key)]
	pieceTaken := udc.pieceUsage[pieceData.index]
	if chunkComplete || chunkFailed || w.ownedOnDownloadCooldown() || !workerHasPiece || pieceTaken {
		udc.mu.Unlock()
		udc.managedRemoveWorker()
		return nil
	}
	defer udc.mu.Unlock()

	// TODO: This is where we would put filters based on worker latency, worker
	// price, worker throughput, etc. There's a lot of fancy stuff we can do
	// with filtering to make sure that for any given chunk we always use the
	// optimal set of workers, and this is the spot where most of the filtering
	// will happen.
	//
	// One major thing that we will want to be careful about when we improve
	// this section is total memory vs. worker bandwidth. If the renter is
	// consistently memory bottlenecked such that the slow hosts are hogging all
	// of the memory and choking out the fasts hosts, leading to underutilized
	// network connections where we actually have enough fast hosts to be fully
	// utilizing the network. Part of this will be solved by adding bandwidth
	// stats to the hostdb, but part of it will need to be solved by making sure
	// that we automatically put low-bandwidth or high-latency workers on
	// standby if we know that memory is the bottleneck as opposed to download
	// bandwidth.
	//
	// Workers that do not meet the extra criteria are not discarded but rather
	// put on standby, so that they can step in if the workers that do meet the
	// extra criteria fail or otherwise prove insufficient.
	//
	// NOTE: Any metrics that we pull from the worker here need to be 'owned'
	// metrics, so that we can avoid holding the worker lock and the udc lock
	// simultaneously (deadlock risk). The 'owned' variables of the worker are
	// variables that are only accessed by the master worker thread.
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
		udc.pieceUsage[pieceData.index] = true
		return udc
	}
	// Worker is not needed unless another worker fails, so put this worker on
	// standby for this chunk. The worker is still available to help with the
	// download, so the worker is not removed from the chunk in this codepath.
	udc.workersStandby = append(udc.workersStandby, w)
	return nil
}
