package renter

import (
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A worker listens for work on a certain host.
//
// The mutex of the worker only protects the 'unprocessedChunks' and the
// 'standbyChunks' fields of the worker. The rest of the fields are only
// interacted with exclusively by the primary worker thread, and only one of
// those ever exists at a time.
//
// The workers have a concept of 'cooldown' for uploads and downloads. If a
// download or upload operation fails, the assumption is that future attempts
// are also likely to fail, because whatever condition resulted in the failure
// will still be present until some time has passed. Without any cooldowns,
// uploading and downloading with flaky hosts in the worker sets has
// substantially reduced overall performacne and throughput.
type worker struct {
	// The contract and host used by this worker.
	contract   modules.RenterContract
	hostPubKey types.SiaPublicKey
	renter     *Renter

	// Download variables.
	downloadChan                chan struct{} // Notifications of new work. Takes priority over uploads.
	downloadChunks              []*unfinishedDownloadChunk // Yet unprocessed work items.
	downloadConsecutiveFailures time.Time         // How many failures in a row?
	downloadRecentFailure       time.Time         // How recent was the last failure?
	downloadTerminated          bool              // Has downloading been terminated for this worker?

	// Upload variables.
	unprocessedChunks         []*unfinishedChunk // Yet unprocessed work items.
	uploadChan                chan struct{}      // Notifications of new work.
	uploadConsecutiveFailures int                // How many times in a row uploading has failed.
	uploadRecentFailure       time.Time          // How recent was the last failure?
	uploadTerminated          bool               // Have we stopped uploading?

	// Utilities.
	killChan chan struct{} // Worker will shut down if a signal is sent down this channel.
	mu       sync.Mutex
}

// managedKillDownloading will drop all of the pieces given to the worker.
func (w *worker) managedKillDownloading(udc *unfinishedDownloadChunk) {
	w.mu.Lock()
	w.dropDownloadChunks()
	w.downloadTerminated = true
	w.mu.Unlock()
}

// managedKillUploading will disable all uploading for the worker.
func (w *worker) managedKillUploading() {
	w.mu.Lock()
	w.dropUploadChunks()
	w.uploadTerminated = true
	w.mu.Unlock()
}

// onDownloadCooldown returns true if the worker is on cooldown from failed
// downloads.
func (w *worker) onDownloadCooldown() bool {
	requiredCooldown := downloadFailureCooldown
	for i := 0; i < w.downloadConsecutiveFailures && i < maxConsecutivePenalty; i++ {
		requiredCooldown *= 2
	}
	return time.Now().Before(w.downloadRecentFailure.Add(requiredCooldown))
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

// managedQueueDownloadChunk adds a chunk to the worker's queue.
func (w *worker) managedQueueDownloadChunk(udc *unfinishedDownloadChunk) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Drop the chunk if the worker is terminated or on cooldown.
	if w.downloadTerminated || w.onDownloadCooldown() {
		udc.mu.Lock()
		w.dropDownloadChunk(udc)
		udc.mu.Unlock()
		return
	}

	// Append the chunk and send a signal that download chunks are available.
	w.downloadChunks = append(w.downloadChunks, udc)
	select {
	case w.downloadChan <- struct{}{}:
	default:
	}
}

// managedQueueUploadChunk will take a chunk and add it to the worker's repair
// stack.
func (w *worker) managedQueueUploadChunk(uc *unfinishedChunk) {
	// Check that the worker is allowed to be uploading before grabbing the
	// worker lock.
	utility, exists := w.renter.hostContractor.ContractUtility(w.contract.ID)
	goodForUpload := exists && utility.GoodForUpload
	w.mu.Lock()
	defer w.mu.Unlock()

	if !goodForUpload || w.uploadTerminated || w.onUploadCooldown() {
		// The worker should not be uploading, remove the chunk.
		w.dropChunk(uc)
		return
	}
	w.unprocessedChunks = append(w.unprocessedChunks, uc)

	// Send a signal informing the work thread that there is work.
	select {
	case w.uploadChan <- struct{}{}:
	default:
	}
}

// managedNextDownloadChunk will pull the next potential chunk out of the work
// queue for downloading.
func (w *worker) managedNextDownloadChunk() (nextChunk *unfinishedDownloadChunk) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Loop through the download chunks to find something to do.
	for range w.downloadChunks {
		chunk := w.downloadChunks[0]
		w.downloadChunks = w.downloadChunks[1:]
		nextDownloadChunk = w.processDownloadChunk(chunk)
		if nextDownloadChunk != nil {
			return nextDownloadChunk
		}
	}
	return nil
}

// managedNextUploadChunk will pull the next potential chunk out of the worker's
// work queue for uploading.
func (w *worker) managedNextUploadChunk() (nextChunk *unfinishedChunk, pieceIndex uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Loop through the unprocessed chunks and find some work to do.
	for range w.unprocessedChunks {
		// Pull a chunk off of the unprocessed chunks stack.
		chunk := w.unprocessedChunks[0]
		w.unprocessedChunks = w.unprocessedChunks[1:]
		nextChunk, pieceIndex := w.processUploadChunk(chunk)
		if nextChunk != nil {
			return nextChunk, pieceIndex
		}
	}
	return nil, 0 // no work found
}

// updateWorkerPool will grab the set of contracts from the contractor and
// update the worker pool to match.
func (r *Renter) managedUpdateWorkerPool() {
	contractSlice := r.hostContractor.Contracts()
	contractMap := make(map[types.FileContractID]modules.RenterContract)
	for i := 0; i < len(contractSlice); i++ {
		contractMap[contractSlice[i].ID] = contractSlice[i]
	}

	// Add a worker for any contract that does not already have a worker.
	for id, contract := range contractMap {
		lockID := r.mu.Lock()
		_, exists := r.workerPool[id]
		if !exists {
			worker := &worker{
				contract:   contract,
				hostPubKey: contract.HostPublicKey,

				downloadChan:         make(chan struct{}, 1),
				killChan:             make(chan struct{}),
				uploadChan:           make(chan struct{}, 1),

				renter: r,
			}
			r.workerPool[id] = worker
			go worker.threadedWorkLoop()
		}
		r.mu.Unlock(lockID)
	}

	// Remove a worker for any worker that is not in the set of new contracts.
	lockID := r.mu.Lock()
	for id, worker := range r.workerPool {
		_, exists := contractMap[id]
		if !exists {
			delete(r.workerPool, id)
			close(worker.killChan)
		}
	}
	r.mu.Unlock(lockID)
}

// threadedWorkLoop repeatedly issues work to a worker, stopping when the worker
// is killed or when the thread group is closed.
func (w *worker) threadedWorkLoop() {
	err := w.renter.tg.Add()
	if err != nil {
		return
	}
	defer w.renter.tg.Done()
	// The worker may have upload chunks and it needs to drop them before
	// terminating.
	defer w.managedKillUploading()
	defer w.managedKillDownloading()

	for {
		// Perform one stpe of processing download work.
		downloadChunk := w.managedNextDownloadChunk()
		if downloadChunk != nil {
			w.managedDownload(chunk)
			continue
		}

		// Perform one step of processing upload work.
		chunk, pieceIndex := w.managedNextChunk()
		if chunk != nil {
			w.managedUpload(chunk, pieceIndex)
			continue
		}

		// Block until new work is received via the upload or download channels,
		// or until a kill or stop signal is received.
		select {
		case <-w.downloadChan:
			continue
		case <-w.uploadChan:
			continue
		case <-w.killChan:
			return
		case <-w.renter.tg.StopChan():
			return
		}
	}
}
