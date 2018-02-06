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

// managedQueueDownloadChunk adds a chunk to the worker's queue.
func (w *worker) managedQueueDownloadChunk(udc *unfinishedDownloadChunk) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Don't add the chunk to a terminated worker.
	//
	// TODO: Don't add chunk to a worker on timeout either.
	if w.downloadTerminated {
		udc.mu.Lock()
		w.dropDownloadChunk(udc)
		udc.mu.Unlock()
		return
	}

	w.downloadChunks = append(w.downloadChunks, udc)
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
		nextChunk, pieceIndex := w.processChunk(chunk)
		if nextChunk != nil {
			return nextChunk, pieceIndex
		}
	}
	return nil, 0 // no work found
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
		case d := <-w.downloadChan:
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
