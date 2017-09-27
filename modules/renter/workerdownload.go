package renter

import (
	"github.com/NebulousLabs/Sia/crypto"
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
)

// download will perform some download work.
func (w *worker) download(dw downloadWork) {
	d, err := w.renter.hostContractor.Downloader(w.contract.ID, w.renter.tg.StopChan())
	if err != nil {
		go func() {
			select {
			case dw.resultChan <- finishedDownload{dw.chunkDownload, nil, err, dw.pieceIndex, w.contract.ID}:
			case <-w.renter.tg.StopChan():
			}
		}()
		return
	}
	defer d.Close()

	data, err := d.Sector(dw.dataRoot)
	go func() {
		select {
		case dw.resultChan <- finishedDownload{dw.chunkDownload, data, err, dw.pieceIndex, w.contract.ID}:
		case <-w.renter.tg.StopChan():
		}
	}()
}
