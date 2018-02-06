package renter

// TODO: Need to structure the download + heap priority such that the earlier
// chunks in a file will always be selected before later chunks in a file. That
// is to ensure memory availability - some download formats (namely ones that
// require the download to be serialized) require that earlier chunks be written
// first, which means you can have a deadlock if it is possible for later chunks
// to begin requesting memory before earlier chunks have had a chance to request
// memory.

import (
	"bytes"
	"errors"
	"io"
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
	errDownloadRenterClosed = errors.New("download could not be scheduled because renter is shutting down")
	errInsufficientHosts    = errors.New("insufficient hosts to recover file")
	errInsufficientPieces   = errors.New("couldn't fetch enough pieces to recover data")
	errPrevErr              = errors.New("download could not be completed due to a previous error")
)

type (

	// A download is a file download that has been queued by the renter.
	download struct {
		// atomicDataReceived is updated as data is downloaded from the wire.
		// The value is not updated until the full piece is received from the
		// host, meaning that it should never go over 100% data total.
		atomicDataReceived uint64

		// Progress variables.
		chunksRemaining  uint64        // Number of chunks whose downloads are incomplete.
		downloadComplete bool          // Set to true after the full data is recovered and written.
		downloadErr      error         // Only set if there was an error which prevented the download from completing.
		downloadFinished chan struct{} // Closed once the download is complete.

		// Timestamp information.
		completeTime time.Time
		startTime    time.Time

		// Static information about the file - can be read without a lock.
		destination modules.DownloadWriter
		length      int64 // Length to download starting from the offset.
		offset      int64 // Offset within the file to start the download.
		siapath     string

		// Utilities.
		mu sync.Mutex
	}

	// downloadChunkHeap is a heap that is sorted first by file priority, then
	// by the start time of the download, and finally by the index of the chunk.
	// As downloads are queued, they are added to the downloadChunkHeap. As
	// resources become available to execute downloads, chunks are pulled off of
	// the heap and distributed to workers.
	downloadChunkHeap []*unfinishedDownloadChunk

	// downloadParams is the set of parameters to use when downloading a file.
	downloadParams struct {
		file        *file                  // The file to download.
		destination modules.DownloadWriter // The place to write the downloaded data.

		length int64 // Length of download. Cannot be 0.
		offset int64 // Offset within the file to start the download. Must be less than the total filesize.

		latencyTarget uint64 // Workers above this latency will be automatically put on standby initially.
		needsMemory   bool   // Whether new memory needs to be allocated to perform the download.
		overdrive     int    // Number of extra pieces to download as a race to improve latency at the cost of bandwidth efficiency.
		priority      uint64 // Files with a higher priority will be downloaded first.
	}

	// unfinishedDownloadChunk contains a chunk for a download that is in
	// progress.
	//
	// TODO: The memory management is not perfect here. As we collect data
	// pieces (instead of parity pieces), we don't need as much memory to
	// recover the original data. But we do already allocate only as much as we
	// potentially need, meaning that you can't naively release some memory to
	// the renter each time a data piece completes, you have to check that the
	// data piece was not already expected to be required for the download.
	unfinishedDownloadChunk struct {
		// Fetch + Write instructions - read only or otherwise thread safe.
		destination modules.DownloadWriter // Where to write the recovered logical chunk.
		erasureCode modules.ErasuerCoder
		masterKey   crypto.TwofishKey

		// Fetch + Write instructions - read only or otherwise thread safe.
		chunkIndex  uint64 // Required for deriving the encryption keys for each piece.
		chunkMap    map[types.FileContractID]int // Maps from file contract ids to the piece stored at that point.
		chunkSize   uint64
		fetchLength int64 // Length within the logical chunk to fetch.
		fetchOffset int64 // Offset within the logical chunk that is being downloaded.
		pieceSize   uint64
		writeOffset int64 // Offet within the writer to write the completed data.

		// Fetch + Write instructions - read only or otherwise thread safe.
		latencyTarget uint64
		needsMemory   bool // Set to true if memory was not pre-allocated for this chunk.
		overdrive     int
		priority      uint64

		// Download chunk state - need mutex to access.
		failed            bool       // Indicates whether this download has failed.
		mu                sync.Mutex // Protects all the fields in this codeblock.
		pieceUsage        []bool     // Which pieces are being actively fetched.
		physicalChunkData [][]byte   // Used to recover the logical data.
		piecesCompleted   int        // Number of pieces that have successfully completed.
		piecesRegistered  int        // Number of pieces that workers are actively fetching.
		workersRemaining  int        // Number of workers still able to fetch the chunk.
		workersStandby    []*worker  // Set of workers that are able to work on this download, but are not needed unless other workers fail.

		// Memory management variables.
		memoryAllocated int

		// The download object, mostly to update download progress.
		download *download
	}
)

// Implementation of heap.Interface for downloadChunkHeap.
func (dch downloadChunkHeap) Len() int { return len(dch) }
func (dch downloadChunkHeap) Less(i, j int) bool {
	// First sort by priority.
	if dch[i].priority != dch[j].priority {
		return dch[i].priority > dch[j].priority
	}
	// For equal priority, sort by start time.
	if dch[i].download.startTime != dch[j].download.startTime {
		return dch[i].download.startTime.Before(dch[j].download.startTime)
	}
	// For equal start time (typically meaning it's the same file), sort by
	// chunkIndex.
	//
	// NOTE: To prevent deadlocks when acquiring memory and using writers that
	// will streamline / order different chunks, we must make sure that we sort
	// by chunkIndex such that the earlier chunks are selected first from the
	// heap.
	return dch[i].chunkIndex < dch[j].chunkIndex
}
func (dch downloadChunkHeap) Swap(i, j int)       { dch[i], dch[j] = dch[j], dch[i] }
func (dch *downloadChunkHeap) Push(x interface{}) { *dch = append(*dch, x.(*unfinishedDownloadChunk)) }
func (dch *downloadChunkHeap) Pop() interface{} {
	old := *dch
	n := len(old)
	x := old[n-1]
	*dch = old[0 : n-1]
	return x
}

// fail will
func (udc *unfinishedDownloadChunk) fail() {
	if udc.failed {
		// Failure code has already run, no need to run again.
		return
	}

	udc.failed = true
	udc.download.fail(errors.New("could not recover enough pieces"))
}

// newDownload creates a newly initialized download.
func (r *Renter) newDownload(params downloadParams) (*download, error) {
	// Input validation.
	if params.file == nil {
		return nil, errors.New("no file provided when requesting download")
	}
	if params.length <= 0 {
		return nil, errors.New("cannot perform a download of length 0")
	}
	if params.offset+params.length > params.file.size {
		return nil, errors.New("download is requesting data past the boundary of the file")
	}

	// Create the download object.
	d := &download{
		downloadFinished: make(chan struct{}),

		startTime: time.Now(),

		destination: params.destination,
		length:      params.length,
		offset:      params.offset,
		siapath:     params.file.name,
	}

	// Determine which chunks to download.
	writeOffset := int64(0)
	minChunk := params.offset / params.file.chunkSize()
	maxChunk := (params.offset + params.length - 1) / params.file.chunkSize()

	// For each chunk, assemble a mapping from the contract id to the index of
	// the piece within the chunk that the contract is responsible for.
	chunkMaps := make([]map[types.FileContractID]int, maxChunk-minChunk+1)
	for id, contract := range file.contracts {
		id = r.hostContractor.ResolveID(id)
		for _, piece := range contract.Pieces {
			if piece.Chunk >= minChunk && piece.Chunk <= maxChunk {
				// Sanity check - the same worker should not have to pieces for
				// the same chunk.
				_, exists := chunkMaps[piece.Chunk-minChunk]
				if exists {
					r.log.Println("Worker has multiple pieces uploaded for the same chunk")
					fmt.Println("NOTE: WORKER HAS MULTIPLE PIECES UPLOADED FOR SAME CHUNK")
				}
				chunkMaps[piece.Chunk-minChunk][id] = piece.Piece
			}
		}
	}

	// Queue the downloads for each chunk.
	udcs := make([]*unfinishedDownloadChunk, maxChunk-minChunk+1)
	for i := minChunk; i <= maxChunk; i++ {
		d.chunksRemaining++
		udc := &unfinishedDownloadChunk{
			destination: params.destination,
			erasureCode: params.file.erasureCode,
			masterKey:   params.file.masterKey,

			chunkIndex: i,
			chunkMap:   chunkMaps[i-minChunk],
			chunkSize:  params.file.chunkSize(),
			pieceSize:  params.file.pieceSize,

			// TODO: 25ms is just a guess for a good default. Really, we want to
			// set the latency target such that slower workers will pick up the
			// later chunks, but only if there's a very strong chance that
			// they'll finish before the earlier chunks finish, so that they do
			// no contribute to low latency.
			//
			// TODO: There is some sane minimum latency that should actually be
			// set based on the number of pieces 'n', and the 'n' fastest
			// workers that we have.
			latencyTarget: params.latencyTarget + (25*(i-minChunk)), // Latency target is dropped by 25ms for each following chunk.
			needsMemory:   params.needsMemory,
			priority:      params.priority,

			pieceUsage:        make([]bool, params.file.erasureCode.NumPieces()),
			piecesCompleted:   make([]bool, params.file.erasureCode.NumPieces()),
			physicalChunkData: make([][]byte, params.file.erasureCode.NumPieces()),

			download: d,
		}

		// Set the fetchOffset - the offset within the chunk that we start
		// downloading from.
		if i == minChunk {
			udc.fetchOffset = params.offset % params.file.chunkSize()
		} else {
			udc.fetchOffset = 0
		}
		// Set the fetchLength - the number of bytes to fetch within the chunk
		// that we start downloading from.
		if i == maxChunk {
			udc.fetchLength = ((params.length + params.offset) % params.file.chunkSize()) - udc.fetchOffset
		} else {
			udc.fetchLength = params.file.chunkSize() - udc.fetchOffset
		}
		// Set the writeOffset within the destination for where the data should
		// be written.
		udc.writeOffset = writeOffset
		writeOffset += udc.fetchLength

		// Set the latency and overdrive for the chunk. These are parameters
		// which prioritize latency over throughput and efficiency, and are only
		// necessary for the first few chunks in a download.
		if i < 2 {
			udc.latencyTarget = params.latencyTarget
			udc.overdrive = params.overdrive
		}

		// Add this to the list of chunks.
		udcs = append(udcs, udc)
	}

	// Send the set of downloads down a channel to be put in a heap.
	select {
	case r.newDownloads <- udcs:
		return d, nil
	case <-r.tg.StopChan():
		return nil, errDownloadRenterClosed
	}
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
	// TODO: log the error from Close().
	d.destination.Close()
}

/*
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

	result := recoverWriter.Bytes()

	// Calculate the offset. If the offset is within the chunk, the
	// requested offset is passed, otherwise the offset of the chunk
	// within the overall file is passed.
	chunkBaseAddress := cd.index * cd.download.chunkSize
	chunkTopAddress := chunkBaseAddress + cd.download.chunkSize - 1
	off := chunkBaseAddress
	lowerBound := 0
	if cd.download.offset >= chunkBaseAddress && cd.download.offset <= chunkTopAddress {
		off = cd.download.offset
		offsetInBlock := off - chunkBaseAddress
		lowerBound = int(offsetInBlock) // If the offset is within the block, part of the block will be ignored
	}

	// Truncate b if writing the whole buffer at the specified offset would
	// exceed the maximum file size.
	upperBound := cd.download.chunkSize
	if chunkTopAddress > cd.download.length+cd.download.offset {
		diff := chunkTopAddress - (cd.download.length + cd.download.offset)
		upperBound -= diff + 1
	}
	if upperBound > uint64(len(result)) {
		upperBound = uint64(len(result))
	}

	result = result[lowerBound:upperBound]

	// Write the bytes to the requested output.
	_, err = cd.download.destination.WriteAt(result, int64(off))
	if err != nil {
		return build.ExtendErr("unable to write to download destination", err)
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
		err = cd.download.destination.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
*/

// managedBlockUntilOnline will block until the renter is online. The renter
// will appropriately handle incoming download requests and stop signals while
// waiting.
func (r *Renter) managedBlockUntillOnline() {
	for !r.g.Online() {
		select {
		case <-r.tg.StopChan():
			return
		case newDownload := <-r.newDownloads:
			for i := 0; i < len(newDownload); i++ {
				heap.Push(downloadHeap, newDownload[i])
			}
		case <-time.After(offlineCheckFrequency):
		}
	}
}

// managedFetchDownloadMemory will block until the requested memory has been
// obtained.. The renter will appropriately handle incoming download requests
// and stop signals while waiting.
func (r *Renter) managedFetchDownloadMemory(memoryNeeded uint64) bool {
	memChan, memoryAcquired := r.managedMemoryGet(memoryNeeded)
	for !memoryAcquired {
		select {
		case <-memChan:
			memChan, memoryAcquired = r.managedMemoryGet(memoryNeeded)
		case newDownload := <-r.newDownloads:
			for i := 0; i < len(newDownload); i++ {
				heap.Push(downloadHeap, newDownload[i])
			}
		case <-r.tg.StopChan():
			// Shutdown occurred before memory was acquired.
			return false
		}
	}
	// Memory acquired successfully.
	return true
}

// threadedDownloadLoop utilizes the worker pool to make progress on any queued
// downloads.
func (r *Renter) threadedDownloadLoop() {
	err := r.tg.Add()
	if err != nil {
		return
	}
	defer r.tg.Done()

	downloadHeap := new(downloadChunkHeap)
	for {
		// Wait until we are online.
		r.managedBlockUntilOnline()

		// Return if the renter has shut down.
		select {
		case <-r.tg.StopChan():
			return
		default:
		}

		// Update the worker pool.
		r.managedUpdateWorkerPool()

		// Pull downloads out of the heap
		for downloadHeap.Len() > 0 {
			// Get the chunk.
			nextChunk := heap.Pop(downloadHeap).(*unfinishedDownloadChunk)

			// Acquire memory if required.
			if nextChunk.needsMemory {
				// The amount of memory needed is maximally 2x the number of min
				// pieces - if you do not get any data pieces, you will need to
				// recover the parity pieces that you do get into the data
				// pieces. In addition to that, you need room for any overdrive
				// pieces that you end up downloading.
				memoryNeeded := nextChunk.pieceSize*nextChunk.erasureCode.MinPieces()*2 + nextChunk.overdrivePieces*nextChunk.pieceSize
				if memoryNeeded > nextChunk.erasureCode.NumPieces()*nextChunk.pieceSize {
					// You will never need more memory than one slot for each
					// piece in the chunk.
					memoryNeeded = nextChunk.erasureCode.NumPieces() * nextChunk.pieceSize
				}
				if !r.managedFetchDownloadMemory(memoryNeeded) {
					// Indicates that the renter is shutting down.
					return
				}

				nextChunk.mu.Lock()
				udc.memoryAllocated = memoryNeeded
				nextChunk.mu.Unlock()
			}

			// TODO: Distribute the chunk to workers.
			//
			// Remember to set the 'workers remaining' flag.
			r.mu.Lock()
			nextChunk.mu.Lock()
			nextChunk.workersRemaining = len(r.workerPool)
			nextChunk.mu.Unlock()
			for _, worker := range r.workerPool {
				worker.managedQueueDownloadChunk(nextChunk)
			}
			r.mu.Unlock()
		}

		// Wait for more work.
		select {
		case <-r.tg.StopChan():
			return
		case newDownload := <-r.newDownloads:
			for i := 0; i < len(newDownload); i++ {
				heap.Push(downloadHeap, newDownload[i])
			}
		}
	}
}
