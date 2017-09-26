package renter

// TODO: Need to make sure that we do not end up with two workers for the same
// host, which could potentially happen over renewals because the contract ids
// will be different.

// TODO: Add failure cooldowns to the workers, particulary for uploading tasks.

import (
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// downloadWork contains instructions to download a piece from a host, and
	// a channel for returning the results.
	downloadWork struct {
		// dataRoot is the MerkleRoot of the data being requested, which serves
		// as an ID when requesting data from the host.
		dataRoot   crypto.Hash
		pieceIndex uint64

		chunkDownload *chunkDownload

		// resultChan is a channel that the worker will use to return the
		// results of the download.
		resultChan chan finishedDownload
	}

	// finishedDownload contains the data and error from performing a download.
	finishedDownload struct {
		chunkDownload *chunkDownload
		data          []byte
		err           error
		pieceIndex    uint64
		workerID      types.FileContractID
	}

	// A worker listens for work on a certain host.
	//
	// The mutex of the worker only protects the 'unprocessedChunks' and the
	// 'standbyChunks' fields of the worker. The rest of the fields are only
	// interacted with exclusively by the primary worker thread, and only one of
	// those ever exists at a time.
	worker struct {
		// contractID specifies which contract the worker specifically works
		// with.
		contract   modules.RenterContract
		contractID types.FileContractID
		hostPubKey types.SiaPublicKey

		// If there is work on all three channels, the worker will first do all
		// of the work in the priority download chan, then all of the work in the
		// download chan, and finally all of the work in the upload chan.
		//
		// A busy higher priority channel is able to entirely starve all of the
		// channels with lower priority.
		downloadChan         chan downloadWork // higher priority than all uploads
		killChan             chan struct{}     // highest priority
		priorityDownloadChan chan downloadWork // higher priority than downloads (used for user-initiated downloads)
		uploadChan           chan struct{}     // lowest priority

		// recentUploadFailure documents the most recent time that an upload
		// has failed.
		consecutiveUploadFailures time.Duration
		recentUploadFailure       time.Time // Only modified by primary repair loop.

		// recentDownloadFailure documents the most recent time that a download
		// has failed.
		recentDownloadFailure time.Time // Only modified by the primary download loop.

		// Two lists of chunks that relate to worker upload tasks. The first
		// list is the set of chunks that the worker hasn't examined yet. The
		// second list is the list of chunks that the worker examined, but was
		// unable to process because other workers had taken on all of the work
		// already. This list is maintained in case any of the other workers
		// fail - this worker will be able to pick up the slack.
		unprocessedChunks []*unfinishedChunk
		standbyChunks     []*unfinishedChunk

		// Utilities.
		renter *Renter
		mu     sync.Mutex
	}
)

// download will perform some download work.
func (w *worker) download(dw downloadWork) {
	d, err := w.renter.hostContractor.Downloader(w.contractID, w.renter.tg.StopChan())
	if err != nil {
		go func() {
			select {
			case dw.resultChan <- finishedDownload{dw.chunkDownload, nil, err, dw.pieceIndex, w.contractID}:
			case <-w.renter.tg.StopChan():
			}
		}()
		return
	}
	defer d.Close()

	data, err := d.Sector(dw.dataRoot)
	go func() {
		select {
		case dw.resultChan <- finishedDownload{dw.chunkDownload, data, err, dw.pieceIndex, w.contractID}:
		case <-w.renter.tg.StopChan():
		}
	}()
}

// queueChunkRepair will take a chunk and add it to the worker's repair stack.
func (w *worker) queueChunkRepair(chunk *unfinishedChunk) {
	// Add the new chunk to our list of unprocessed chunks.
	w.mu.Lock()
	w.unprocessedChunks = append(w.unprocessedChunks, chunk)
	w.mu.Unlock()

	// Send a signal informing the work thread that there is work (in the event
	// that it is sleeping).
	select {
	case w.uploadChan <- struct{}{}:
	default:
	}
}

// upload will perform some upload work.
func (w *worker) upload(uc *unfinishedChunk, pieceIndex uint64) {
	// Open an editing connection to the host.
	e, err := w.renter.hostContractor.Editor(w.contractID, w.renter.tg.StopChan())
	if err != nil {
		w.recentUploadFailure = time.Now()
		w.consecutiveUploadFailures++
		uc.mu.Lock()
		uc.piecesRegistered--
		uc.pieceUsage[pieceIndex] = false
		uc.mu.Unlock()
		return
	}
	defer e.Close()

	// Perform the upload, and update the failure stats based on the success of
	// the upload attempt.
	root, err := e.Upload(uc.physicalChunkData[pieceIndex])
	if err != nil {
		w.recentUploadFailure = time.Now()
		w.consecutiveUploadFailures++
		uc.mu.Lock()
		uc.piecesRegistered--
		uc.pieceUsage[pieceIndex] = false
		uc.mu.Unlock()
		return
	}
	w.consecutiveUploadFailures = 0

	// Update the renter metadata.
	addr := e.Address()
	endHeight := e.EndHeight()
	id := w.renter.mu.Lock()
	uc.renterFile.mu.Lock()
	contract, exists := uc.renterFile.contracts[w.contractID]
	if !exists {
		contract = fileContract{
			ID:          w.contractID,
			IP:          addr,
			WindowStart: endHeight,
		}
	}
	contract.Pieces = append(contract.Pieces, pieceData{
		Chunk:      uc.index,
		Piece:      pieceIndex,
		MerkleRoot: root,
	})
	uc.renterFile.contracts[w.contractID] = contract
	w.renter.saveFile(uc.renterFile)
	uc.renterFile.mu.Unlock()
	w.renter.mu.Unlock(id)

	// Clear memory in the renter now that we're done.
	w.renter.managedMemoryAvailableAdd(uint64(len(uc.physicalChunkData[pieceIndex])))
}

// nextChunk will pull the next potential chunk out of the worker's work queue
// for uploading.
func (w *worker) nextChunk() (nextChunk *unfinishedChunk, pieceIndex uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Loop through the unprocessed chunks and find some work to do.
	for i := 0; i < len(w.unprocessedChunks); i++ {
		// Pull a chunk off of the unprocessed chunks stack.
		nextChunk := w.unprocessedChunks[0]
		w.unprocessedChunks = w.unprocessedChunks[1:]

		// See if we are a candidate host for this chunk, and also if this
		// chunk still needs work performed on it.
		nextChunk.mu.Lock()
		_, candidateHost := nextChunk.unusedHosts[w.hostPubKey.String()]
		chunkComplete := nextChunk.piecesNeeded <= nextChunk.piecesCompleted
		needsHelp := nextChunk.piecesNeeded > nextChunk.piecesCompleted+nextChunk.piecesRegistered
		pieceIndex := 0
		if candidateHost && needsHelp {
			// Select a piece and mark that a piece has been selected.
			for i := 0; i < len(nextChunk.pieceUsage); i++ {
				if !nextChunk.pieceUsage[i] {
					pieceIndex = i
					nextChunk.pieceUsage[i] = true
					break
				}
			}
			delete(nextChunk.unusedHosts, w.hostPubKey.String())
			nextChunk.piecesRegistered++
		}
		nextChunk.mu.Unlock()

		// Return this chunk for work if the worker is able to help.
		if candidateHost && needsHelp {
			return nextChunk, uint64(pieceIndex)
		}

		// Add this chunk to the standby chunks if this chunk may need help,
		// but is already being helped.
		if !chunkComplete && candidateHost {
			w.standbyChunks = append(w.standbyChunks, nextChunk)
		}
	}

	// Loop through the standby chunks to see if there is work to do.
	for i := 0; i < len(w.standbyChunks); i++ {
		nextChunk := w.standbyChunks[0]
		w.standbyChunks = w.standbyChunks[1:]

		// See if we are a candidate host for this chunk, and also if this
		// chunk still needs work performed on it.
		nextChunk.mu.Lock()
		chunkComplete := nextChunk.piecesNeeded <= nextChunk.piecesCompleted
		needsHelp := nextChunk.piecesNeeded > nextChunk.piecesCompleted+nextChunk.piecesRegistered
		pieceIndex := 0
		if needsHelp {
			// Select a piece and mark that a piece has been selected.
			for i := 0; i < len(nextChunk.pieceUsage); i++ {
				if !nextChunk.pieceUsage[i] {
					pieceIndex = i
					nextChunk.pieceUsage[i] = true
					break
				}
			}
			delete(nextChunk.unusedHosts, w.hostPubKey.String())
			nextChunk.piecesRegistered++
		}
		nextChunk.mu.Unlock()

		// Return this chunk for work if the worker is able to help.
		if needsHelp {
			return nextChunk, uint64(pieceIndex)
		}

		// Add this chunk to the standby chunks if this chunk may need help,
		// but is already being helped.
		if !chunkComplete {
			w.standbyChunks = append(w.standbyChunks, nextChunk)
		}
	}

	// No work found, try again later.
	return nil, 0
}

// threadedWorkLoop repeatedly issues work to a worker, stopping when the worker
// is killed or when the thread group is closed.
func (w *worker) threadedWorkLoop() {
	err := w.renter.tg.Add()
	if err != nil {
		return
	}
	defer w.renter.tg.Done()

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
		chunk, pieceIndex := w.nextChunk()
		if chunk != nil {
			w.upload(chunk, pieceIndex)
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
		_, exists := r.workerPool[id]
		if !exists {
			worker := &worker{
				contract:   contract,
				contractID: id,
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
	}

	// Remove a worker for any worker that is not in the set of new contracts.
	for id, worker := range r.workerPool {
		_, exists := contractMap[id]
		if !exists {
			delete(r.workerPool, id)
			close(worker.killChan)
		}
	}
}
