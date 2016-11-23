package renter

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/sync"
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
		dataRoot crypto.Hash

		// resultChan is a channel that the worker will use to return the
		// results of the download.
		resultChan chan finishedDownload
	}

	// finishedDownload contains the data and error from performing a download.
	finishedDownload struct {
		data []byte
		err  error
	}

	// finishedUpload contains the Merkle root and error from performing an
	// upload.
	finishedUpload struct {
		dataRoot crypto.Hash
		err      error
	}

	// uploadWork contains instructions to upload a piece to a host, and a
	// channel for returning the results.
	uploadWork struct {
		// data is the payload of the upload.
		data []byte

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
		killChan             chan struct{}
		priorityDownloadChan chan downloadWork // higher priority than standard downloads (used when reparing low-redundancy files w/o original file)
		priorityUploadChan   chan uploadWork   // higher priority than standard uploads (used for low-redundancy files)
		uploadChan           chan uploadWork   // lowest priority

		// Utilities
		contractor hostContractor
		tg         *sync.ThreadGroup
	}
)

// download will perform some download work.
func (w *worker) download(dw downloadWork) {
	d, err := w.contractor.Downloader(w.contractID)
	if err != nil {
		dw.resultChan <- finishedDownload{nil, err}
		return
	}
	defer d.Close()

	data, err := d.Sector(dw.dataRoot)
	dw.resultChan <- finishedDownload{data, err}
}

// upload will perform some upload work.
func (w *worker) upload(uw uploadWork) {
	e, err := w.contractor.Editor(w.contractID)
	if err != nil {
		uw.resultChan <- finishedUpload{crypto.Hash{}, err}
		return
	}
	defer e.Close()

	root, err := e.Upload(uw.data)
	uw.resultChan <- finishedUpload{root, err}
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

	// Check for priority uploads.
	select {
	case u := <-w.priorityUploadChan:
		w.upload(u)
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
	case u := <-w.priorityUploadChan:
		w.upload(u)
		return
	case u := <-w.uploadChan:
		w.upload(u)
		return
	case <-w.tg.StopChan():
		return
	}
}

// workLoop repeatedly issues work to a worker, stopping when the thread group
// is closed.
func (w *worker) threadedWorkLoop() {
	for {
		// Check if the worker has been killed individually.
		select {
		case <-w.killChan:
			return
		default:
			// do nothing
		}

		// Wrap a call to work() in a threadgroup reservation.
		if w.tg.Add() != nil {
			return
		}
		w.work()
		w.tg.Done()
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
		killChan:             make(chan struct{}, 1),
		priorityDownloadChan: make(chan downloadWork, 1),
		priorityUploadChan:   make(chan uploadWork, 1),
		uploadChan:           make(chan uploadWork, 1),

		contractor: r.hostContractor,
		tg:         r.tg,
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
