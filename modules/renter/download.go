package renter

// TODO: There's very little thread safety on the file - what if the file is
// renamed, etc. while it is being downloaded? Unclear what the behavior will
// be.

import (
	"bytes"
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TODO: Move to const file.
const maxConcurrentDownloadChunks = 4

var (
	errInsufficientHosts  = errors.New("insufficient hosts to recover file")
	errInsufficientPieces = errors.New("couldn't fetch enough pieces to recover data")
)

// chunkDownload tracks the progress of a chunk.
type chunkDownload struct {
	// Map of workers who have attempted downloads for this chunk, from the
	// worker id to the index of the piece they are trying to download.
	workerAttempts map[types.FileContractID]uint64

	// List of completed pieces, mapping from the index of the piece to the
	// data.
	completedPieces map[uint64][]byte
}

// A download is a file download that has been queued by the renter.
type download struct {
	atomicReceived   uint64
	atomicChunkIndex uint64

	startTime   time.Time
	siapath     string
	destination string

	chunkSize   uint64
	erasureCode modules.ErasureCoder
	fileSize    uint64
	masterKey   crypto.TwofishKey
	numChunks   uint64

	downloadFinished chan error
}

func recoverChunk(download *download, chunkIndex uint64, chunkDownload chunkDownload, fileDest *os.File) error {
	// Assemble the repair pieces for the chunk.
	chunk := make([][]byte, download.erasureCode.NumPieces())
	for pieceIndex, pieceData := range chunkDownload.completedPieces {
		key := deriveKey(download.masterKey, chunkIndex, pieceIndex)
		pieceBytes, err := key.DecryptBytes(pieceData)
		if err != nil {
			return err
		}
		chunk[pieceIndex] = pieceBytes
	}

	// Recover the chunk into a byte slice.
	recoverWriter := new(bytes.Buffer)
	recoverSize := download.chunkSize
	if chunkIndex == download.numChunks-1 && download.fileSize % download.chunkSize != 0 {
		recoverSize = download.fileSize % download.chunkSize
	}
	err := download.erasureCode.Recover(chunk, recoverSize, recoverWriter)
	if err != nil {
		return err
	}

	// Write the bytes to the download file.
	result := recoverWriter.Bytes()
	_, err = fileDest.WriteAt(result, int64(chunkIndex*download.chunkSize))
	return err
}

// newDownload initializes and returns a download object.
func (f *file) newDownload(destination string) *download {
	d := &download{
		atomicChunkIndex: 0,
		atomicReceived:   0,

		startTime:   time.Now(),
		siapath:     f.name,
		destination: destination,

		chunkSize:   f.chunkSize(),
		erasureCode: f.erasureCode,
		fileSize:    f.size,
		masterKey:   f.masterKey,
		numChunks:   f.numChunks(),

		downloadFinished: make(chan error),
	}
	return d
}

// managedDownloadIteraiton downloads a chunk from the next available file.
func (r *Renter) managedDownloadIteration() {
	// Get the set of available workers.
	id := r.mu.RLock()
	availableWorkers := make([]*worker, 0, len(r.workerPool))
	for _, worker := range r.workerPool {
		availableWorkers = append(availableWorkers, worker)
	}
	r.mu.RUnlock(id)

	// Grab the next file to be downloaded.
	id = r.mu.RLock()
	if len(r.downloadQueue) == 0 {
		// TODO: Block until either there's another download, or until
		// shutdown, or until enough time has passed that the workers should be
		// reassembled?
		//
		// TODO: This should probably be moved to the outer loop, and then the
		// managedDownloadIteraton can be turned into managedDownloadFile
		// taking a file as input.
		r.mu.RUnlock(id)
		return
	}
	nextDownload := r.downloadQueue[0]
	r.downloadQueue = r.downloadQueue[1:]
	r.mu.RUnlock(id)

	// Reject this download if there are not enough workers to complete the
	// download.
	if len(availableWorkers) < nextDownload.erasureCode.MinPieces() {
		// Signal that the download has errored and quit.
		nextDownload.downloadFinished <- errInsufficientHosts
		return
	}

	// TODO: Exclude the most expensive workers, and perhaps the slowest as
	// well.

	// Open a file handle for the download.
	fileDest, err := os.OpenFile(nextDownload.destination, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		nextDownload.downloadFinished <- err
		return
	}
	defer fileDest.Close()

	// Grab each chunk from the download.
	chunkDownloads := make([]chunkDownload, nextDownload.numChunks)

	// Assemble the file pieces as a set of chunks pointing from the file
	// contract id to the piece that the file contract is protecting.
	pieceSet := make([]map[types.FileContractID]pieceData, len(chunkDownloads))
	for i := range pieceSet {
		pieceSet[i] = make(map[types.FileContractID]pieceData)
	}
	id = r.mu.RLock()
	file := r.files[nextDownload.siapath]
	r.mu.RUnlock(id)
	file.mu.Lock()
	for _, contract := range file.contracts {
		for _, piece := range contract.Pieces {
			pieceSet[piece.Chunk][contract.ID] = piece
		}
	}
	file.mu.Unlock()

	var activeDownloads int
	var chunkIndex uint64
	var incompleteChunks []uint64
	resultChan := make(chan finishedDownload)
	for {
		// Break if tg.Stop() has been called, to facilitate quick shutdown.
		select {
		case <-r.tg.StopChan():
			break
		default:
			// Stop is not called, continue with the iteration.
		}

		// Break if there are no active downloads and no chunks remain, as that
		// indicates that the file has finished downloading.
		if activeDownloads == 0 && len(incompleteChunks) == 0 && int(chunkIndex) == len(chunkDownloads) {
			break
		}

		// Check if there are any available workers that can be matched to an
		// incmplete download.
		if len(incompleteChunks) > 0 {
			chunkIndex := incompleteChunks[0]
			incompleteChunks = incompleteChunks[1:]

			// Scan the set of available workers for a worker that is not
			// working on this chunk.
			for i := range availableWorkers {
				_, exists := chunkDownloads[chunkIndex].workerAttempts[availableWorkers[i].contractID]
				if !exists {
					dw := downloadWork{
						dataRoot:   pieceSet[chunkIndex][availableWorkers[i].contractID].MerkleRoot,
						chunkIndex: chunkIndex,
						resultChan: resultChan,
					}
					chunkDownloads[chunkIndex].workerAttempts[availableWorkers[i].contractID] = pieceSet[chunkIndex][availableWorkers[i].contractID].Piece
					availableWorkers[i].downloadChan <- dw
					availableWorkers = append(availableWorkers[:i], availableWorkers[:i+1]...)
					activeDownloads++
					break
				}
			}
		}

		// Check that there are still enough workers to complete the download.
		if len(availableWorkers)+activeDownloads < nextDownload.erasureCode.MinPieces() {
			// Signal that the download has errored and quit.
			nextDownload.downloadFinished <- errInsufficientHosts
			return
		}

		// If there is room to download another chunk, and enough workers to
		// tackle the task, begin performing another download.
		if activeDownloads < maxConcurrentDownloadChunks && int(chunkIndex) < len(chunkDownloads) && len(availableWorkers) >= nextDownload.erasureCode.MinPieces() {
			// Create the chunkDownload object and insert it into the
			// chunkDownloads slice.
			chunkDownloads[chunkIndex] = chunkDownload{
				workerAttempts:  make(map[types.FileContractID]uint64),
				completedPieces: make(map[uint64][]byte),
			}

			// Assign the available workers to this download.
			for i := 0; i < nextDownload.erasureCode.MinPieces(); i++ {
				dw := downloadWork{
					dataRoot:   pieceSet[chunkIndex][availableWorkers[i].contractID].MerkleRoot,
					chunkIndex: chunkIndex,
					resultChan: resultChan,
				}
				chunkDownloads[chunkIndex].workerAttempts[availableWorkers[i].contractID] = pieceSet[chunkIndex][availableWorkers[i].contractID].Piece

				// Send the work to the worker.
				availableWorkers[i].downloadChan <- dw
			}
			// Clear out the set of available workers.
			availableWorkers = availableWorkers[nextDownload.erasureCode.MinPieces():]

			// Indicate that this chunk has been started.
			activeDownloads++
			chunkIndex++
		} else if activeDownloads > 0 {
			// Wait for a piece to return.
			finishedDownload := <-resultChan
			chunkIndex := finishedDownload.chunkIndex
			workerID := finishedDownload.workerID
			activeDownloads--

			// Check for an error.
			if finishedDownload.err != nil {
				// Add this piece to the list of incomplete chunks.
				incompleteChunks = append(incompleteChunks, chunkIndex)

				// Continue using this worker if it has had less than 20
				// consecutive failures. Such a high tolerance allows the
				// renter to reliable download files that are hundreds of GBs
				// in size from hosts that may be missing as much as 25% of the
				// pieces due to disk failure.
				id := r.mu.Lock()
				worker, exists := r.workerPool[workerID]
				if exists {
					worker.consecutiveDownloadFailures++
					if worker.consecutiveDownloadFailures < 20 {
						availableWorkers = append(availableWorkers, worker)
					}
				}
				r.mu.Unlock(id)
			} else {
				// Add this returned piece to the appropriate chunk.
				chunkDownloads[chunkIndex].completedPieces[pieceSet[chunkIndex][workerID].Piece] = finishedDownload.data
				if len(chunkDownloads[chunkIndex].completedPieces) == file.erasureCode.MinPieces() {
					err := recoverChunk(nextDownload, chunkIndex, chunkDownloads[chunkIndex], fileDest)
					if err != nil {
						nextDownload.downloadFinished <- err
						return
					}
				}

				// Return the worker to the list of available workers.
				id := r.mu.RLock()
				worker, exists := r.workerPool[workerID]
				r.mu.RUnlock(id)
				if exists {
					availableWorkers = append(availableWorkers, worker)
				}
			}
		} else {
			time.Sleep(time.Millisecond * 500)
		}
	}

	// Download complete, signal that there was no error during the download.
	nextDownload.downloadFinished <- nil
}

// threadedDownloadLoop utilizes the worker pool to make progress on any queued
// downloads.
func (r *Renter) threadedDownloadLoop() {
	for {
		if r.tg.Add() != nil {
			return
		}
		r.managedDownloadIteration()
		r.tg.Done()
		// TODO: Come up with a better method for blocking during downloads.
		time.Sleep(time.Millisecond * 500)
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
	d := file.newDownload(destination)
	lockID = r.mu.Lock()
	r.downloadQueue = append(r.downloadQueue, d)
	r.mu.Unlock(lockID)

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
			Received:    atomic.LoadUint64(&d.atomicReceived),
			StartTime:   d.startTime,
		}
	}
	return downloads
}
