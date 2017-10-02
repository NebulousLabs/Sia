package renter

// TODO: Add failure cooldowns to the workers, particulary for uploading tasks.

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

	// Channels that inform the worker of kill signals and of new work.
	downloadChan         chan downloadWork // higher priority than all uploads
	killChan             chan struct{}     // highest priority
	priorityDownloadChan chan downloadWork // higher priority than downloads (used for user-initiated downloads)
	uploadChan           chan struct{}     // lowest priority

	// Operation failure statistics for the worker.
	downloadRecentFailure     time.Time // Only modified by the primary download loop.
	uploadRecentFailure       time.Time // Only modified by primary repair loop.
	uploadConsecutiveFailures int

	// Two lists of chunks that relate to worker upload tasks. The first list is
	// the set of chunks that the worker hasn't examined yet. The second list is
	// the list of chunks that the worker examined, but was unable to process
	// because other workers had taken on all of the work already. This list is
	// maintained in case any of the other workers fail - this worker will be
	// able to pick up the slack.
	mu                sync.Mutex
	standbyChunks     []*unfinishedChunk
	terminated        bool
	unprocessedChunks []*unfinishedChunk
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

		// Determine the maximum amount of time to wait for any standby chunks.
		var sleepDuration time.Duration
		w.mu.Lock()
		numStandby := len(w.standbyChunks)
		w.mu.Unlock()
		if numStandby > 0 {
			// TODO: Pick a random time instead of just a constant time.
			sleepDuration = time.Second * 3 // TODO: Constant
		} else {
			sleepDuration = time.Hour // TODO: Constant
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
		case <-time.After(sleepDuration):
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
