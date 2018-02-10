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
		// Worker doesn't hold a piece of the chunk. Nothing to do.
		w.mu.Unlock()
		return
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

// managedDownload will perform some download work.
func (w *worker) managedDownload(dw *downloadWork) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cd := dw.chunkDownload
	cd.mu.Lock()
	defer cd.mu.Unlock()
	defer func() {
		// Unregister worker after download finished
		dw.chunkDownload.piecesRegistered--
	}()
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
	if err == nil {
		// Add data to the comletedPieces
		if _, ok := dw.chunkDownload.completedPieces[dw.pieceIndex]; ok {
			println("already added")
		} else {
			cd.completedPieces[dw.pieceIndex] = data
		}
	}

	go func() {
		select {
		case dw.resultChan <- finishedDownload{dw.chunkDownload, data, err, dw.pieceIndex, w.contract.ID}:
		case <-w.renter.tg.StopChan():
		}
	}()
}

// managedNextDownloadChunk will pull the next potential chunk out of the worker's work queue
// for downloading.
func (w *worker) managedNextDownloadChunk() *downloadWork {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Loop through the different chunkDownload queues from highest to lowest
	// priority
	chunkQueues := []*[]*downloadWork{
		&w.unprocessedPrioDownload,
		&w.standbyPrioDownload,
		&w.unprocessedDownload,
		&w.standbyDownload,
	}
	for _, chunks := range chunkQueues {
		if len(*chunks) == 0 {
			continue
		}
		// Pull a chunk off of the unprocessed chunks stack.
		chunk := (*chunks)[0]
		*chunks = (*chunks)[1:]
		nextChunk := w.processDownloadChunk(chunk)
		if nextChunk != nil {
			return nextChunk
		}
	}
	// No work found, try again later.
	return nil
}

// processChunkDownload will process a chunk from the worker chunk queue.
func (w *worker) processDownloadChunk(dw *downloadWork) (nextChunk *downloadWork) {
	// Determine what sort of help this chunk needs.
	cd := dw.chunkDownload
	cd.mu.Lock()
	minPieces := cd.download.erasureCode.MinPieces()
	attempted, _ := cd.workerAttempts[w.contract.ID]
	chunkComplete := minPieces <= len(cd.completedPieces)
	needsHelp := minPieces > len(cd.completedPieces)+cd.piecesRegistered

	//	println("******************************************")
	//	println("workerid:      ", w.contract.ID.String())
	//	println("chunkid:       ", dw.chunkDownload.index)
	//	println("minPieces:     ", minPieces)
	//	println("attempted:     ", attempted)
	//	println("chunkComplete: ", chunkComplete)
	//	println("completedpiec: ", len(cd.completedPieces))
	//	println("registeredpie: ", cd.piecesRegistered)
	//	println("needsHelp:     ", needsHelp)
	//	println("******************************************")

	// If the chunk does not need help from this worker, release the chunk.
	if chunkComplete || attempted {
		// This worker no longer needs to track this chunk.
		cd.mu.Unlock()
		return nil
	}

	// If the chunk needs help from this worker, find a piece to upload and
	// return the stats for that piece.
	if needsHelp {
		// Mark host as used
		cd.workerAttempts[w.contract.ID] = true
		// Increase the number of currently downloading pieces
		cd.piecesRegistered++
		cd.mu.Unlock()
		return dw
	}
	cd.mu.Unlock()

	// The chunk could need help from this worker, but only if other workers who
	// are performing uploads experience failures. Put this chunk on standby.
	if cd.download.priority {
		w.standbyPrioDownload = append(w.standbyPrioDownload, dw)
	} else {
		w.standbyDownload = append(w.standbyDownload, dw)
	}
	return nil
}
