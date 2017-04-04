package renter

// NOTE: All chunk recovery (which involves high computation and disk syncing)
// is done in the primary download loop thread. At some point this may be a
// significant performance bottleneck.

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	defaultFilePerm         = 0666
	downloadFailureCooldown = time.Minute * 30
)

var (
	errPrevErr            = errors.New("download could not be completed due to a previous error")
	errInsufficientHosts  = errors.New("insufficient hosts to recover file")
	errInsufficientPieces = errors.New("couldn't fetch enough pieces to recover data")

	// maxActiveDownloadPieces determines the maximum number of pieces that are
	// allowed to be concurrently downloading. More pieces means more
	// parallelism, but also more RAM usage.
	//
	// TODO: Allow this number to be established in the renter settings.
	maxActiveDownloadPieces = build.Select(build.Var{
		Standard: int(40),
		Dev:      int(10),
		Testing:  int(5),
	}).(int)
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
		atomicDataReceived uint64
		downloadComplete   bool
		downloadErr        error
		finishedChunks     []bool

		// Timestamp information.
		completeTime time.Time
		startTime    time.Time

		// Static information about the file - can be read without a lock.
		chunkSize         uint64
		destination       string
		erasureCode       modules.ErasureCoder
		fileSize          uint64
		masterKey         crypto.TwofishKey
		numChunks         uint64
		pieceSet          []map[types.FileContractID]pieceData
		reportedPieceSize uint64
		siapath           string

		// Syncrhonization tools.
		downloadFinished chan struct{}
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
		// activeWorkers indicates the list of workers which are actively
		// download a piece, and can be utilized again later but are currently
		// unavailable.
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
		activeWorkers    map[types.FileContractID]struct{}
		availableWorkers []*worker
		incompleteChunks []*chunkDownload
		resultChan       chan finishedDownload
	}
)

// newDownload initializes and returns a download object.
func (r *Renter) newDownload(f *file, destination string, currentContracts map[modules.NetAddress]types.FileContractID) *download {
	d := &download{
		finishedChunks: make([]bool, f.numChunks()),

		startTime: time.Now(),

		chunkSize:   f.chunkSize(),
		destination: destination,
		erasureCode: f.erasureCode,
		fileSize:    f.size,
		masterKey:   f.masterKey,
		numChunks:   f.numChunks(),
		siapath:     f.name,

		downloadFinished: make(chan struct{}),
	}
	// Allocate the piece size and progress bar so that the download will
	// finish at exactly 100%. Due to rounding error and padding, there is not
	// a strict mapping between 'progress' and 'bytes downloaded' - it is
	// actually necessary to download more bytes than the size of the file.
	d.reportedPieceSize = d.fileSize / (d.numChunks * uint64(d.erasureCode.MinPieces()))
	d.atomicDataReceived = d.fileSize - (d.reportedPieceSize * d.numChunks * uint64(d.erasureCode.MinPieces()))

	// Assemble the piece set for the download.
	d.pieceSet = make([]map[types.FileContractID]pieceData, f.numChunks())
	for i := range d.pieceSet {
		d.pieceSet[i] = make(map[types.FileContractID]pieceData)
	}
	f.mu.RLock()
	for _, contract := range f.contracts {
		// Get latest contract ID.
		id, ok := currentContracts[contract.IP]
		if !ok {
			// No matching NetAddress; try using a revised ID.
			id = r.hostContractor.ResolveID(contract.ID)
			if id == contract.ID {
				continue
			}
		}
		for i := range contract.Pieces {
			d.pieceSet[contract.Pieces[i].Chunk][id] = contract.Pieces[i]
		}
	}
	f.mu.RUnlock()

	return d
}

// Err returns the error encountered by a download, if it exists.
func (d *download) Err() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.downloadErr
}

// fail will mark the download as complete, but with the provided error.
func (d *download) fail(err error) {
	if d.downloadComplete {
		// Either the download has already succeeded or failed, nothing to do.
		return
	}

	d.downloadComplete = true
	d.downloadErr = err
	close(d.downloadFinished)
}

// recoverChunk takes a chunk that has had a sufficient number of pieces
// downloaded and verifies, decrypts and decodes them into the file.
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
	fileDest, err := os.OpenFile(cd.download.destination, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
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
		close(cd.download.downloadFinished)
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
	if len(ds.incompleteChunks) == 0 && len(ds.activeWorkers) == 0 && len(r.chunkQueue) == 0 {
		// If the above conditions are true, it should also be the case that
		// the number of active pieces is zero.
		if ds.activePieces != 0 {
			r.log.Critical("ERROR: the renter is idle , but tracking active pieces:", ds.activePieces)
		}

		// Nothing to do. Sleep until there is something to do, or until
		// shutdown. Dislodge occasionally in case r.newDownloads misses a new
		// download.
		select {
		case d := <-r.newDownloads:
			r.addDownloadToChunkQueue(d)
		case <-r.tg.StopChan():
			return
		}
	}

	// Update the set of workers to include everyone in the worker pool.
	id := r.mu.Lock()
	r.updateWorkerPool()
	ds.availableWorkers = make([]*worker, 0, len(r.workerPool))
	for _, worker := range r.workerPool {
		// Ignore workers that are already in the active set of workers.
		_, exists := ds.activeWorkers[worker.contractID]
		if exists {
			continue
		}

		// Ignore workers that have a download failure recently.
		if time.Since(worker.recentDownloadFailure) < downloadFailureCooldown {
			continue
		}

		// TODO: Prune workers that do not provide value. If bandwidth can be
		// saturated with fewer workers, then the more expensive ones should be
		// eliminated.

		ds.availableWorkers = append(ds.availableWorkers, worker)
	}
	r.mu.Unlock(id)

	// Add new chunks to the extent that resources allow.
	r.managedScheduleNewChunks(ds)

	// Check for incomplete chunks, and assign workers to them where possible.
	r.managedScheduleIncompleteChunks(ds)

	// Wait for workers to return after downloading pieces.
	r.managedWaitOnDownloadWork(ds)
}

// managedScheduleIncompleteChunks iterates through all of the incomplete
// chunks and finds workers to complete the chunks.
// managedScheduleIncompleteChunks also checks wheter a chunk is unable to be
// completed.
func (r *Renter) managedScheduleIncompleteChunks(ds *downloadState) {
	var newIncompleteChunks []*chunkDownload
loop:
	for _, incompleteChunk := range ds.incompleteChunks {
		// Drop this chunk if the file download has failed in any way.
		incompleteChunk.download.mu.Lock()
		downloadComplete := incompleteChunk.download.downloadComplete
		incompleteChunk.download.mu.Unlock()
		if downloadComplete {
			// The download has most likely failed. No need to complete this
			// chunk.
			ds.activePieces--                                       // For the current incomplete chunk.
			ds.activePieces -= len(incompleteChunk.completedPieces) // For all completed pieces.

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

			// If no piece exists for this worker, do not give the worker this
			// download.
			piece, exists := incompleteChunk.download.pieceSet[incompleteChunk.index][worker.contractID]
			if !exists {
				continue
			}

			dw := downloadWork{
				dataRoot:      piece.MerkleRoot,
				pieceIndex:    piece.Piece,
				chunkDownload: incompleteChunk,
				resultChan:    ds.resultChan,
			}
			incompleteChunk.workerAttempts[worker.contractID] = true
			ds.availableWorkers = append(ds.availableWorkers[:i], ds.availableWorkers[i+1:]...)
			ds.activeWorkers[worker.contractID] = struct{}{}
			select {
			case worker.priorityDownloadChan <- dw:
			default:
				r.log.Critical("Download work not immediately received by worker")
			}
			continue loop
		}

		// Determine whether any of the workers in the set of active workers is
		// able to pick up the slack, indicating that the chunk can be
		// completed just not at this time.
		for fcid := range ds.activeWorkers {
			// Check whether a piece exists for this worker.
			_, exists1 := incompleteChunk.download.pieceSet[incompleteChunk.index][fcid]
			scheduled, exists2 := incompleteChunk.workerAttempts[fcid]
			if !scheduled && exists1 && exists2 {
				// This worker is able to complete the download for this chunk,
				// but is busy. Keep this chunk until the next iteration of the
				// download loop.
				newIncompleteChunks = append(newIncompleteChunks, incompleteChunk)
				continue loop
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
		ds.activePieces--                                       // for the current incomplete chunk
		ds.activePieces -= len(incompleteChunk.completedPieces) // for all completed pieces
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
		chunkQueueLen := len(r.chunkQueue)
		if chunkQueueLen == 0 {
			// There are no more chunks to initiate, return.
			return
		}

		// View the next chunk.
		nextChunk := r.chunkQueue[0]

		// Check whether there are enough resources to perform the download.
		if ds.activePieces+nextChunk.download.erasureCode.MinPieces() > maxActiveDownloadPieces {
			// There is a limited amount of RAM available, and scheduling the
			// next piece would consume too much RAM.
			return
		}

		// Chunk is set to be downloaded. Clear it from the queue.
		r.chunkQueue = r.chunkQueue[1:]

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

// managedWaitOnDownloadWork will wait for workers to return after attempting to
// download a piece.
func (r *Renter) managedWaitOnDownloadWork(ds *downloadState) {
	// If there are no workers performing work, return early.
	if len(ds.activeWorkers) == 0 {
		return
	}

	// Wait for a piece to return. If a new download arrives while waiting, add
	// it to the download queue immediately.
	var finishedDownload finishedDownload
	select {
	case <-r.tg.StopChan():
		return
	case d := <-r.newDownloads:
		r.addDownloadToChunkQueue(d)
		return
	case finishedDownload = <-ds.resultChan:
	}

	// Prepare the piece.
	workerID := finishedDownload.workerID
	delete(ds.activeWorkers, workerID)

	// Fetch the corresponding worker.
	id := r.mu.RLock()
	worker, exists := r.workerPool[workerID]
	r.mu.RUnlock(id)
	if !exists {
		return
	}

	// Check for an error.
	cd := finishedDownload.chunkDownload
	if finishedDownload.err != nil {
		r.log.Debugln("Error when downloading a piece:", finishedDownload.err)
		worker.recentDownloadFailure = time.Now()
		ds.incompleteChunks = append(ds.incompleteChunks, cd)
		return
	}

	// Add this returned piece to the appropriate chunk.
	cd.completedPieces[finishedDownload.pieceIndex] = finishedDownload.data
	atomic.AddUint64(&cd.download.atomicDataReceived, cd.download.reportedPieceSize)

	// If the chunk has completed, perform chunk recovery.
	if len(cd.completedPieces) == cd.download.erasureCode.MinPieces() {
		err := cd.recoverChunk()
		ds.activePieces -= len(cd.completedPieces)
		cd.completedPieces = make(map[uint64][]byte)
		if err != nil {
			r.log.Println("Download failed - could not recover a chunk:", err)
			cd.download.mu.Lock()
			cd.download.fail(err)
			cd.download.mu.Unlock()
		}
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
	ds := &downloadState{
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
