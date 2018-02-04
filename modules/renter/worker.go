package renter

// TODO: Add failure cooldowns to the workers, particularly for uploading tasks.

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
	downloadChan          chan downloadWork // higher priority than all uploads
	downloadRecentFailure time.Time
	priorityDownloadChan  chan downloadWork // higher priority than downloads (used for user-initiated downloads)

	// Uploading variables.
	unprocessedChunks         []*unfinishedChunk // Yet unprocessed work items.
	uploadChan                chan struct{}      // Notifications of new work.
	uploadConsecutiveFailures int                // How many times in a row uploading has failed.
	uploadRecentFailure       time.Time          // How recent was the last failure?
	uploadTerminated          bool               // Have we stopped uploading?

	// Utilities.
	killChan chan struct{} // Worker will shut down if a signal is sent down this channel.
	mu       sync.Mutex
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

	for {
		// Check for priority downloads.
		select {
		case d := <-w.priorityDownloadChan:
			w.download(d)
			continue
		default:
		}

		// Check for standard downloads.
		select {
		case d := <-w.downloadChan:
			w.download(d)
			continue
		default:
		}

		// Perform one step of processing upload work.
		chunk, pieceIndex := w.managedNextChunk()
		if chunk != nil {
			w.managedUpload(chunk, pieceIndex)
			continue
		}

		// Block until new work is received via the upload or download channels,
		// or until the standby chunks are ready to be revisited, or until a
		// kill signal is received.
		select {
		case d := <-w.priorityDownloadChan:
			w.download(d)
			continue
		case d := <-w.downloadChan:
			w.download(d)
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

				downloadChan:         make(chan downloadWork, 1),
				killChan:             make(chan struct{}),
				priorityDownloadChan: make(chan downloadWork, 1),
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
