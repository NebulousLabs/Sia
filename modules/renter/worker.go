package renter

// TODO: Need to make sure that we do not end up with two workers for the same
// host, which could potentially happen over renewals because the contract ids
// will be different.

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// errWorkerDoesNotExist is returned if retireWorker is called for a
	// contract id that does not have a worker associated with it.
	errWorkerDoesNotExist = errors.New("no worker exists for that contract id")

	// errWorkerExists is returned if addWorker is called for a contract id
	// that already has a worker.
	errWorkerExists = errors.New("there is already a worker for that contract id")
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

	// finishedUpload contains the Merkle root and error from performing an
	// upload.
	finishedUpload struct {
		chunkID    chunkID
		dataRoot   crypto.Hash
		err        error
		pieceIndex uint64
		workerID   types.FileContractID
	}

	// uploadWork contains instructions to upload a piece to a host, and a
	// channel for returning the results.
	uploadWork struct {
		// data is the payload of the upload.
		chunkID    chunkID
		data       []byte
		file       *file
		pieceIndex uint64

		// resultChan is a channel that the worker will use to return the
		// results of the upload.
		resultChan chan finishedUpload
	}

	// A worker listens for work on a certain host.
	worker struct {
		// contractID specifies which contract the worker specifically works
		// with.
		contractID types.FileContractID

		// If there is work on all three channels, the worker will first do all
		// of the work in the download chan, then all of the work in the
		// priority upload chan, and finally all of the work in the upload
		// chan.
		//
		// A busy higher priority channel is able to entriely starve all of the
		// channels with lower priority.
		downloadChan         chan downloadWork // higher priority than all uploads
		killChan             chan struct{}     // highest priority
		priorityDownloadChan chan downloadWork // higher priority than downloads (used for user-initiated downloads)
		uploadChan           chan uploadWork   // lowest priority

		// recentUploadFailure documents the most recent time that an upload
		// has failed. The repair loop ignores workers that have had an upload
		// failure in the past two hours.
		recentUploadFailure time.Time

		// Utilities
		renter *Renter
	}
)

// download will perform some download work.
func (w *worker) download(dw downloadWork) {
	d, err := w.renter.hostContractor.Downloader(w.contractID)
	if err != nil {
		select {
		case dw.resultChan <- finishedDownload{dw.chunkDownload, nil, err, dw.pieceIndex, w.contractID}:
		case <-w.renter.tg.StopChan():
		}
		return
	}
	defer d.Close()

	data, err := d.Sector(dw.dataRoot)
	select {
	case dw.resultChan <- finishedDownload{dw.chunkDownload, data, err, dw.pieceIndex, w.contractID}:
	case <-w.renter.tg.StopChan():
	}
}

// upload will perform some upload work.
func (w *worker) upload(uw uploadWork) {
	e, err := w.renter.hostContractor.Editor(w.contractID)
	if err != nil {
		w.recentUploadFailure = time.Now()
		select {
		case uw.resultChan <- finishedUpload{uw.chunkID, crypto.Hash{}, err, uw.pieceIndex, w.contractID}:
		case <-w.renter.tg.StopChan():
		}
		return
	}
	defer e.Close()

	root, err := e.Upload(uw.data)
	if err != nil {
		w.recentUploadFailure = time.Now()
		select {
		case uw.resultChan <- finishedUpload{uw.chunkID, root, err, uw.pieceIndex, w.contractID}:
		case <-w.renter.tg.StopChan():
		}
		return
	}

	// Update the renter metadata.
	id := w.renter.mu.Lock()
	uw.file.mu.Lock()
	contract, exists := uw.file.contracts[w.contractID]
	if !exists {
		contract = fileContract{
			ID:          w.contractID,
			IP:          e.Address(),
			WindowStart: e.EndHeight(),
		}
	}
	contract.Pieces = append(contract.Pieces, pieceData{
		Chunk:      uw.chunkID.index,
		Piece:      uw.pieceIndex,
		MerkleRoot: root,
	})
	uw.file.contracts[w.contractID] = contract
	w.renter.saveFile(uw.file)
	uw.file.mu.Unlock()
	w.renter.mu.Unlock(id)

	select {
	case uw.resultChan <- finishedUpload{uw.chunkID, root, err, uw.pieceIndex, w.contractID}:
	case <-w.renter.tg.StopChan():
	}
}

// work will perform one unit of work, exiting early if there is a kill signal
// given before work is completed.
func (w *worker) work() {
	// Check for priority downloads.
	select {
	case d := <-w.priorityDownloadChan:
		w.download(d)
		return
	default:
		// do nothing
	}

	// Check for standard downloads.
	select {
	case d := <-w.downloadChan:
		w.download(d)
		return
	default:
		// do nothing
	}

	// None of the priority channels have work, listen on all channels.
	select {
	case d := <-w.downloadChan:
		w.download(d)
		return
	case <-w.killChan:
		return
	case d := <-w.priorityDownloadChan:
		w.download(d)
		return
	case u := <-w.uploadChan:
		w.upload(u)
		return
	case <-w.renter.tg.StopChan():
		return
	}
}

// threadedWorkLoop repeatedly issues work to a worker, stopping when the
// thread group is closed.
func (w *worker) threadedWorkLoop() {
	for {
		// Check if the worker has been killed individually.
		select {
		case <-w.killChan:
			return
		default:
			// do nothing
		}

		if w.renter.tg.Add() != nil {
			return
		}
		w.work()
		w.renter.tg.Done()
	}
}

// addWorker will create a worker for the provided file contract id and add it
// to the renter. Upon return, the work loop for the worker will have been
// spawned.
func (r *Renter) addWorker(fcid types.FileContractID) error {
	_, exists := r.workerPool[fcid]
	if exists {
		return errWorkerExists
	}

	worker := &worker{
		contractID: fcid,

		downloadChan:         make(chan downloadWork, 1),
		killChan:             make(chan struct{}),
		priorityDownloadChan: make(chan downloadWork, 1),
		uploadChan:           make(chan uploadWork, 1),

		renter: r,
	}
	r.workerPool[fcid] = worker
	go worker.threadedWorkLoop()
	return nil
}

// retireWorker will remove a worker from the work pool, terminating the work
// loop and deleting the worker from the renter's worker pool.
func (r *Renter) retireWorker(fcid types.FileContractID) error {
	w, exists := r.workerPool[fcid]
	if !exists {
		return errWorkerDoesNotExist
	}

	delete(r.workerPool, fcid)
	close(w.killChan)
	return nil
}

// updateWorkerPool will grab the set of contracts from the contractor and
// update the worker pool to match.
func (r *Renter) updateWorkerPool() (errorSet error) {
	// Get a map of all the contracts in the contractor.
	newContracts := make(map[types.FileContractID]struct{})
	for _, nc := range r.hostContractor.Contracts() {
		newContracts[nc.ID] = struct{}{}
	}

	// Add a worker for any contract that does not already have a worker.
	for id := range newContracts {
		_, exists := r.workerPool[id]
		if !exists {
			errorSet = build.ComposeErrors(errorSet, r.addWorker(id))
		}
	}

	// Remove a worker for any worker that is not in the set of new contracts.
	for id := range r.workerPool {
		_, exists := newContracts[id]
		if !exists {
			errorSet = build.ComposeErrors(errorSet, r.retireWorker(id))
		}
	}
	return errorSet
}
