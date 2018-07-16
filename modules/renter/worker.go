package renter

import (
	"sync"
	"time"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"
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
// substantially reduced overall performance and throughput.
type worker struct {
	// The contract and host used by this worker.
	contract   modules.RenterContract
	hostPubKey types.SiaPublicKey
	renter     *Renter

	// Download variables that are not protected by a mutex, but also do not
	// need to be protected by a mutex, as they are only accessed by the master
	// thread for the worker.
	ownedDownloadConsecutiveFailures int       // How many failures in a row?
	ownedDownloadRecentFailure       time.Time // How recent was the last failure?

	// Download variables related to queuing work. They have a separate mutex to
	// minimize lock contention.
	downloadChan       chan struct{}              // Notifications of new work. Takes priority over uploads.
	downloadChunks     []*unfinishedDownloadChunk // Yet unprocessed work items.
	downloadMu         sync.Mutex
	downloadTerminated bool // Has downloading been terminated for this worker?

	// Upload variables.
	unprocessedChunks         []*unfinishedUploadChunk // Yet unprocessed work items.
	uploadChan                chan struct{}            // Notifications of new work.
	uploadConsecutiveFailures int                      // How many times in a row uploading has failed.
	uploadRecentFailure       time.Time                // How recent was the last failure?
	uploadTerminated          bool                     // Have we stopped uploading?

	// Utilities.
	//
	// The mutex is only needed when interacting with 'downloadChunks' and
	// 'unprocessedChunks', as everything else is only accessed from the single
	// master thread.
	killChan chan struct{} // Worker will shut down if a signal is sent down this channel.
	mu       sync.Mutex
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

				downloadChan: make(chan struct{}, 1),
				killChan:     make(chan struct{}),
				uploadChan:   make(chan struct{}, 1),

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
	defer w.managedKillUploading()
	defer w.managedKillDownloading()

	for {
		// Perform one stpe of processing download work.
		downloadChunk := w.managedNextDownloadChunk()
		if downloadChunk != nil {
			// managedDownload will handle removing the worker internally. If
			// the chunk is dropped from the worker, the worker will be removed
			// from the chunk. If the worker executes a download (success or
			// failure), the worker will be removed from the chunk. If the
			// worker is put on standby, it will not be removed from the chunk.
			w.managedDownload(downloadChunk)
			continue
		}

		// Perform one step of processing upload work.
		chunk, pieceIndex := w.managedNextUploadChunk()
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
