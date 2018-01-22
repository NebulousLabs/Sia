package renter

import (
	"github.com/NebulousLabs/Sia/build"
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

// managedQueueChunkDownload will take a chunk and add it to the worker's download stack.
func (w *worker) managedQueueChunkDownload(ds *downloadState, cd *chunkDownload) {
	w.mu.Lock()
	// TODO check worker cooldown
	// Get the piece of the chunk that the worker's host holds
	var dw *downloadWork
	piece, exists := cd.download.pieceSet[cd.index][w.contract.ID]
	if exists {
		dw = &downloadWork{
			dataRoot:      piece.MerkleRoot,
			pieceIndex:    piece.Piece,
			chunkDownload: cd,
			resultChan:    ds.resultChan,
		}
	} else {
		build.Critical("worker doesn't hold piece of chunk but should")
	}

	// Check whether the download is a priority download or not
	if cd.download.priority {
		w.unprocessedPrioDownload = append(w.unprocessedPrioDownload, dw)
	} else {
		w.unprocessedDownload = append(w.unprocessedDownload, dw)
	}
	w.mu.Unlock()

	// Send a signal informing the work thread that there is work.
	select {
	case w.downloadChan <- struct{}{}:
	default:
	}
}

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
