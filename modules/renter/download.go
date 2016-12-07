package renter

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TODO: Move to const file.
//
// maxActiveDownloadPieces indicates the maximum number of pieces that can be
// actively getting downloaded at a time. Each piece will consume up to
// modules.SectorSize of RAM. Parallelism for downloads is improved when this
// number is increased.
const (
	defaultFilePerm = 0666
	maxActiveDownloadPieces = 25
	maxDownloadLoopIdleTime = time.Minute * 10
)

var (
	errPrevErr            = errors.New("download could not be completed due to a previous error")
	errInsufficientHosts  = errors.New("insufficient hosts to recover file")
	errInsufficientPieces = errors.New("couldn't fetch enough pieces to recover data")
)

type (
	// chunkDownload tracks the progress of a chunk. The chunk download object
	// should only be read or modified inside of the main download loop thread.
	chunkDownload struct {
		download *download
		index    uint64 // index of the chunk within the download

		// completedPieces contains information about which pieces have been
		// successfully downloaded.
		//
		// workerAttempts contains a list of workers that are able to fetch a
		// piece of the chunk, mapped to an indication of whether or not they
		// have tried to fetch a piece of the chunk.
		completedPieces map[uint64][]byte
		workerAttempts  map[types.FileContractID]bool
	}

	// A download is a file download that has been queued by the renter.
	download struct {
		// Progress variables.
		dataReceived     uint64
		downloadComplete bool
		downloadErr      error
		finishedChunks   []bool

		// Timestamp information.
		completeTime time.Time
		startTime    time.Time

		// Static information about the file - can be read without a lock.
		chunkSize   uint64
		destination string
		erasureCode modules.ErasureCoder
		fileSize    uint64
		masterKey   crypto.TwofishKey
		numChunks   uint64
		pieceSet    []map[types.FileContractID]pieceData
		siapath     string

		// Syncrhonization tools.
		downloadFinished chan error
		mu               sync.Mutex
	}

	// downloadState tracks all of the stateful information within the download
	// loop, primarily used to simplify the use of helper functions. There is
	// no thread safety with the download state, as it is only ever accessed by
	// the primary download loop thread.
	downloadState struct {
		// activePieces tracks the number of pieces that have been scheduled
		// but have not yet been written to disk as a complete chunk.
		//
		// availableWorkers tracks which workers are currently idle and ready
		// to receive work.
		//
		// incompleteChunks is a list of chunks (by index) which have had a
		// download fail. Repeat entries means that multiple downloads failed.
		// A new worker should be assigned to the chunk for each failure,
		// unless no more workers exist who can download pieces for that chunk,
		// in which case the download has failed.
		//
		// resultChan is the channel that is used to receive completed worker
		// downloads.
		activePieces     int
		activeWorkers   map[types.FileContractID]struct{}
		availableWorkers []*worker
		incompleteChunks []*chunkDownload
		resultChan       chan finishedDownload
	}
)

// newDownload initializes and returns a download object.
func newDownload(f *file, destination string) *download {
	d := &download{
		finishedChunks: make([]bool, f.numChunks()),

		startTime:   time.Now(),

		chunkSize:   f.chunkSize(),
		destination: destination,
		erasureCode: f.erasureCode,
		fileSize:    f.size,
		masterKey:   f.masterKey,
		numChunks:   f.numChunks(),
		siapath:     f.name,

		downloadFinished: make(chan error),
	}

	// Assemble the piece set for the download.
	pieceSet := make([]map[types.FileContractID]pieceData, f.numChunks())
	for i := range pieceSet {
		pieceSet[i] = make(map[types.FileContractID]pieceData)
	}
	for _, contract := range f.contracts {
		for _, piece := range contract.Pieces {
			pieceSet[piece.Chunk][contract.ID] = piece
		}
	}

	return d
}

// fail will mark the download as complete, but with the provided error.
func (d *download) fail(err error) {
	if d.downloadComplete {
		// Either the download has already succeeded or failed, nothing to do.
		return
	}

	d.downloadComplete = true
	d.downloadErr = err
	d.downloadFinished <- err
}

// recoverChunk takes a chunk that has had a sufficient number of pieces
// downloaded and verified and decryptps + decodes them into the file.
func (cd *chunkDownload) recoverChunk() error {
	// Assemble the chunk from the download.
	cd.download.mu.Lock()
	chunk := make([][]byte, cd.download.erasureCode.NumPieces())
	for pieceIndex, pieceData := range cd.completedPieces {
		chunk[pieceIndex] = pieceData
	}
	complete := cd.download.downloadComplete
	prevErr := cd.download.downloadErr
	cd.download.mu.Unlock()

	// Return early if the download has previously suffered an error.
	if complete {
		return build.ComposeErrors(errPrevErr, prevErr)
	}

	// Decrypt the chunk pieces.
	for i := range chunk {
		// Skip pieces that were not downloaded.
		if chunk[i] == nil {
			continue
		}

		// Decrypt the piece.
		key := deriveKey(cd.download.masterKey, cd.index, uint64(i))
		decryptedPiece, err := key.DecryptBytes(chunk[i])
		if err != nil {
			return build.ExtendErr("unable to decrypt piece", err)
		}
		chunk[i] = decryptedPiece
	}

	// Recover the chunk into a byte slice.
	recoverWriter := new(bytes.Buffer)
	recoverSize := cd.download.chunkSize
	if cd.index == cd.download.numChunks-1 && cd.download.fileSize%cd.download.chunkSize != 0 {
		recoverSize = cd.download.fileSize % cd.download.chunkSize
	}
	err := cd.download.erasureCode.Recover(chunk, recoverSize, recoverWriter)
	if err != nil {
		return build.ExtendErr("unable to recover chunk", err)
	}

	// Open a file handle for the download.
	fileDest, err := os.OpenFile(cd.download.destination, os.O_CREATE|os.O_RDWR|os.O_TRUNC, defaultFilePerm)
	if err != nil {
		return build.ExtendErr("unable to open download destination", err)
	}
	defer fileDest.Close()

	// Write the bytes to the download file.
	result := recoverWriter.Bytes()
	_, err = fileDest.WriteAt(result, int64(cd.index*cd.download.chunkSize))
	if err != nil {
		return build.ExtendErr("unable to write to download destination", err)
	}

	// Sync the write to provide proper durability.
	err = fileDest.Sync()
	if err != nil {
		return build.ExtendErr("unable to sync downlaod destination", err)
	}

	cd.download.mu.Lock()
	defer cd.download.mu.Unlock()

	// Update the download to signal that this chunk has completed. Only update
	// after the sync, so that durability is maintained.
	if cd.download.finishedChunks[cd.index] {
		build.Critical("recovering chunk when the chunk has already finished downloading")
	}
	cd.download.finishedChunks[cd.index] = true

	// Determine whether the download is complete.
	nowComplete := true
	for _, chunkComplete := range cd.download.finishedChunks {
		if !chunkComplete {
			nowComplete = false
			break
		}
	}
	if nowComplete {
		// Signal that the download is complete.
		cd.download.downloadComplete = true
		cd.download.downloadFinished <- nil
	}
	return nil
}

// addDownloadToChunkQueue takes a file and adds all incomplete work from the file
// to the renter's chunk queue.
func (r *Renter) addDownloadToChunkQueue(d *download) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Skip this file if it has already errored out or has already finished
	// downloading.
	if d.downloadComplete {
		return
	}

	// Add the unfinished chunks one at a time.
	for i := range d.finishedChunks {
		// Skip chunks that have already finished downloading.
		if d.finishedChunks[i] {
			continue
		}

		// Add this chunk to the chunk queue.
		cd := &chunkDownload{
			download: d,
			index:    uint64(i),

			completedPieces: make(map[uint64][]byte),
			workerAttempts:  make(map[types.FileContractID]bool),
		}
		for fcid := range d.pieceSet[i] {
			cd.workerAttempts[fcid] = false
		}
		r.chunkQueue = append(r.chunkQueue, cd)
	}
}

// downloadIteration performs one iteration of the download loop.
func (r *Renter) managedDownloadIteration(ds *downloadState) {
	// Check for sleep and break conditions.
	id := r.mu.RLock()
	queueSize := len(r.chunkQueue)
	r.mu.RUnlock(id)
	if len(ds.incompleteChunks) == 0 && len(ds.activeWorkers) == 0 && queueSize == 0 {
		// Nothing to do. Sleep until there is something to do, or until
		// shutdown. Dislodge occasionally as a preventative measure.
		select {
		case d := <-r.newDownloads:
			r.addDownloadToChunkQueue(d)
		case <-time.After(maxDownloadLoopIdleTime):
			return
		case <-r.tg.StopChan():
			return
		}
	}

	// TODO: Update the set of workers to optimally choose workers based on
	// price, reliability, local bandwidth speeds, etc.

	// Check for incomplete chunks, and assign workers to them where possible.
	r.managedScheduleIncompleteChunks(ds)

	// Add new chunks to the extent that resources allow.
	r.managedScheduleNewChunks(ds)

	// Wait for workers to return after downloading pieces.
	r.managedWaitOnWork(ds)
}

// managedScheduleIncompleteChunks iterates through all of the incomplete
// chunks and finds workers to complete the chunks.
// managedScheduleIncompleteChunks also checks wheter a chunk is unable to be
// completed.
func (r *Renter) managedScheduleIncompleteChunks(ds *downloadState) {
	var newIncompleteChunks []*chunkDownload
ICL:
	for _, incompleteChunk := range ds.incompleteChunks {
		// Drop this chunk if the file download has failed in any way.
		incompleteChunk.download.mu.Lock()
		downloadComplete := incompleteChunk.download.downloadComplete
		incompleteChunk.download.mu.Unlock()
		if downloadComplete {
			// The download has most likely failed. No need to complete this
			// chunk.
			ds.activePieces-- // for the current incomplete chunk
			for _ = range incompleteChunk.completedPieces {
				ds.activePieces-- // for the completed chunk
			}
			// Clear the set of completed pieces so that we do not
			// over-subtract if the above code is run multiple times.
			incompleteChunk.completedPieces = make(map[uint64][]byte)
			continue
		}

		// Try to find a worker that is able to pick up the slack on the
		// incomplete download from the set of available workers.
		for i, worker := range ds.availableWorkers {
			scheduled, exists := incompleteChunk.workerAttempts[worker.contractID]
			if scheduled || !exists {
				// Either this worker does not contain a piece of this chunk,
				// or this worker has already been scheduled to download a
				// piece for this chunk.
				continue
			}

			dw := downloadWork{
				dataRoot: incompleteChunk.download.pieceSet[incompleteChunk.index][worker.contractID].MerkleRoot,
				chunkDownload: incompleteChunk,
				resultChan: ds.resultChan,
			}
			incompleteChunk.workerAttempts[worker.contractID] = true
			worker.downloadChan <- dw
			ds.availableWorkers = append(ds.availableWorkers[:i], ds.availableWorkers[i:]...)
			ds.activeWorkers[worker.contractID] = struct{}{}
			continue ICL
		}

		// Determine whether any of the workers in the set of active workers is
		// able to pick up the slack, indicating that the chunk can be
		// completed just not at this time.
		for fcid := range ds.activeWorkers {
			scheduled, exists := incompleteChunk.workerAttempts[fcid]
			if !scheduled && exists {
				// This worker is able to complete the download for this chunk,
				// but is busy. Keep this chunk until the next iteration of the
				// download loop.
				newIncompleteChunks = append(newIncompleteChunks, incompleteChunk)
				continue ICL
			}
		}

		// TODO: Determine whether any of the workers not in the available set
		// or the active set is able to pick up the slack. Verify that they are
		// safe to be scheduled, and then schedule them if so.

		// Cannot find workers to complete this download, fail the download
		// connected to this chunk.
		r.log.Println("Not enough workers to finish download:", errInsufficientHosts)
		incompleteChunk.download.fail(errInsufficientHosts)

		// Clear out the piece burden for this chunk.
		ds.activePieces-- // for the current incomplete chunk
		for _ = range incompleteChunk.completedPieces {
			ds.activePieces-- // for the completed chunk
		}
		// Clear the set of completed pieces so that we do not
		// over-subtract if the above code is run multiple times.
		incompleteChunk.completedPieces = make(map[uint64][]byte)
	}
	ds.incompleteChunks = newIncompleteChunks
}

// managedScheduleNewChunks uses the set of available workers to schedule new
// chunks if there are resources available to begin downloading them.
func (r *Renter) managedScheduleNewChunks(ds *downloadState) {
	// Keep adding chunks until a break condition is hit.
	for {
		id := r.mu.RLock()
		chunkQueueLen := len(r.chunkQueue)
		r.mu.RUnlock(id)
		if chunkQueueLen == 0 {
			// There are no more chunks to initiate, return.
			return
		}

		// View the next chunk.
		id = r.mu.RLock()
		nextChunk := r.chunkQueue[0]
		r.mu.RUnlock(id)

		// Check whether there are enough resources to perform the download.
		if ds.activePieces + nextChunk.download.erasureCode.MinPieces() > maxActiveDownloadPieces {
			// There is a limited amount of RAM available, and scheduling the
			// next piece would consume too much RAM.
			return
		}

		// Chunk is set to be downloaded. Clear it from the queue.
		id = r.mu.Lock()
		r.chunkQueue = r.chunkQueue[1:]
		r.mu.Unlock(id)

		// Check if the download has already completed. If it has, it's because
		// the download failed.
		nextChunk.download.mu.Lock()
		downloadComplete := nextChunk.download.downloadComplete
		nextChunk.download.mu.Unlock()
		if downloadComplete {
			// Download has already failed.
			continue
		}

		// Add an incomplete chunk entry for every piece of the download.
		for i := 0; i < nextChunk.download.erasureCode.MinPieces(); i++ {
			ds.incompleteChunks = append(ds.incompleteChunks, nextChunk)
		}
		ds.activePieces += nextChunk.download.erasureCode.MinPieces()
	}
}

// managedWaitOnWork will wait for workers to return after attempting to
// download a piece.
func (r *Renter) managedWaitOnWork(ds *downloadState) {
	// Wait for a piece to return.
	finishedDownload := <-ds.resultChan
	workerID := finishedDownload.workerID
	delete(ds.activeWorkers, workerID)

	// Return the worker to the list of available workers.
	id := r.mu.RLock()
	worker, exists := r.workerPool[workerID]
	r.mu.RUnlock(id)
	if exists {
		ds.availableWorkers = append(ds.availableWorkers, worker)
	}

	// Check for an error.
	if finishedDownload.err != nil {
		ds.incompleteChunks = append(ds.incompleteChunks, finishedDownload.chunkDownload)
		return
	}
	cd := finishedDownload.chunkDownload

	// Add this returned piece to the appropriate chunk.
	cd.completedPieces[cd.index] = finishedDownload.data

	// Recover the chunk and save to disk.
	if len(cd.completedPieces) == cd.download.erasureCode.MinPieces() {
		err := cd.recoverChunk()
		if err != nil {
			r.log.Println("Download failed - could not recover a chunk:", err)
			cd.download.mu.Lock()
			cd.download.fail(err)
			cd.download.mu.Unlock()
		}
		ds.activePieces -= len(cd.completedPieces)
	}
}

// threadedDownloadLoop utilizes the worker pool to make progress on any queued
// downloads.
func (r *Renter) threadedDownloadLoop() {
	// Compile the set of available workers.
	id := r.mu.RLock()
	availableWorkers := make([]*worker, 0, len(r.workerPool))
	for _, worker := range r.workerPool {
		availableWorkers = append(availableWorkers, worker)
	}
	r.mu.RUnlock(id)

	// Create the download state.
	ds := &downloadState {
		activeWorkers:    make(map[types.FileContractID]struct{}),
		availableWorkers: availableWorkers,
		incompleteChunks: make([]*chunkDownload, 0),
		resultChan:       make(chan finishedDownload),
	}
	for {
		if r.tg.Add() != nil {
			return
		}
		r.managedDownloadIteration(ds)
		r.tg.Done()
	}
}

// Download downloads a file, identified by its path, to the destination
// specified.
func (r *Renter) Download(path, destination string) error {
	// Lookup the file associated with the nickname.
	lockID := r.mu.RLock()
	file, exists := r.files[path]
	r.mu.RUnlock(lockID)
	if !exists {
		return errors.New("no file with that path")
	}

	// Create the download object and add it to the queue.
	d := newDownload(file, destination)
	lockID = r.mu.Lock()
	r.downloadQueue = append(r.downloadQueue, d)
	r.mu.Unlock(lockID)
	r.newDownloads <-d

	// Block until the download has completed.
	//
	// TODO: Eventually just return the channel to the error instead of the
	// error itself.
	return <-d.downloadFinished
}

// DownloadQueue returns the list of downloads in the queue.
func (r *Renter) DownloadQueue() []modules.DownloadInfo {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// order from most recent to least recent
	downloads := make([]modules.DownloadInfo, len(r.downloadQueue))
	for i := range r.downloadQueue {
		d := r.downloadQueue[len(r.downloadQueue)-i-1]
		downloads[i] = modules.DownloadInfo{
			SiaPath:     d.siapath,
			Destination: d.destination,
			Filesize:    d.fileSize,
			Received:    d.dataReceived,
			StartTime:   d.startTime,
		}
	}
	return downloads
}
