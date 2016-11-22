package renter

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
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
	case <-w.tg.StopChan():
		return
	case d := <-w.priorityDownloadChan:
		w.download(d)
	case u := <-w.priorityUploadChan:
		w.upload(u)
	case u := <-w.uploadChan:
		w.upload(u)
	}
}

// workLoop repeatedly issues work to a worker, stopping when the thread group
// is closed.
func (w *worker) workLoop() {
	for {
		if w.tg.Add() != nil {
			return
		}
		w.work()
		w.tg.Done()
	}
}
