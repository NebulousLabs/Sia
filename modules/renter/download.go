package renter

// NOTE: All chunk recovery (which involves high computation and disk syncing)
// is done in the primary download loop thread. At some point this may be a
// significant performance bottleneck.

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
	errInsufficientHosts  = errors.New("insufficient hosts to recover file")
	errInsufficientPieces = errors.New("couldn't fetch enough pieces to recover data")
	errPrevErr            = errors.New("download could not be completed due to a previous error")
)

type (
	// chunkDownload tracks the progress of a chunk. The chunk download object
	// should only be read or modified inside of the main download loop thread.
	chunkDownload struct {
		download     *download
		index        uint64 // index of the chunk within the download
		memoryNeeded uint64 // memory required to restore chunk in bytes

		// Worker synchronization fields. The mutex only protects these fields.
		mu               sync.Mutex
		completedPieces  map[uint64][]byte             // pieces that were already downloaded
		workerAttempts   map[types.FileContractID]bool // workers that tried to download a piece
		piecesRegistered int                           // number of pieces that are being uploaded, but aren't finished yet.
		workersRemaining int                           // number of workers who have received the chunk, but haven't finished processing it.

	}

	// A download is a file download that has been queued by the renter.
	download struct {
		// Progress variables.
		atomicDataReceived uint64
		downloadComplete   bool
		downloadErr        error
		finishedChunks     map[uint64]bool
		offset             uint64
		length             uint64
		priority           bool

		// Timestamp information.
		completeTime time.Time
		startTime    time.Time

		// Static information about the file - can be read without a lock.
		chunkSize   uint64
		destination modules.DownloadWriter
		erasureCode modules.ErasureCoder
		fileSize    uint64
		masterKey   crypto.TwofishKey
		numChunks   uint64

		// pieceSet contains a sparse map of the chunk indices to be downloaded to
		// their piece data.
		pieceSet          map[uint64]map[types.FileContractID]pieceData
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
		availableWorkers []*worker
		incompleteChunks []*chunkDownload
		resultChan       chan finishedDownload
	}
)

// newSectionDownload initializes and returns a download object for the specified chunk.
func (r *Renter) newSectionDownload(f *file, destination modules.DownloadWriter, offset, length uint64, priority bool) *download {
	d := newDownload(f, destination, priority)

	if length == 0 {
		build.Critical("download length should not be zero")
		d.fail(errors.New("download length should not be zero"))
		return d
	}

	// Settings specific to a chunk download.
	d.offset = offset
	d.length = length

	// Calculate chunks to download.
	minChunk := offset / f.chunkSize()
	maxChunk := (offset + length - 1) / f.chunkSize() // maxChunk is 1-indexed

	// mark the chunks as not being downloaded yet
	for i := minChunk; i <= maxChunk; i++ {
		d.finishedChunks[i] = false
	}

	d.initPieceSet(f, r)
	return d
}

// newDownload creates a newly initialized download.
func newDownload(f *file, destination modules.DownloadWriter, priority bool) *download {
	return &download{
		startTime:        time.Now(),
		chunkSize:        f.chunkSize(),
		destination:      destination,
		erasureCode:      f.erasureCode,
		fileSize:         f.size,
		masterKey:        f.masterKey,
		numChunks:        f.numChunks(),
		siapath:          f.name,
		downloadFinished: make(chan struct{}),
		finishedChunks:   make(map[uint64]bool),
		priority:         priority,
	}
}

// initPieceSet initializes the piece set, including calculations of the total download size.
func (d *download) initPieceSet(f *file, r *Renter) {
	// Allocate the piece size and progress bar so that the download will
	// finish at exactly 100%. Due to rounding error and padding, there is not
	// a strict mapping between 'progress' and 'bytes downloaded' - it is
	// actually necessary to download more bytes than the size of the file.
	// The effective size of the download is determined by the number of chunks
	// to be downloaded. TODO: Handle variable-size last chunk - Same in downloadqueue.go
	numChunks := uint64(len(d.finishedChunks))

	dlSize := d.length
	d.reportedPieceSize = dlSize / (numChunks * uint64(d.erasureCode.MinPieces()))
	d.atomicDataReceived = dlSize - (d.reportedPieceSize * numChunks * uint64(d.erasureCode.MinPieces()))

	// Assemble the piece set for the download.
	d.pieceSet = make(map[uint64]map[types.FileContractID]pieceData)
	for i := range d.finishedChunks {
		d.pieceSet[i] = make(map[types.FileContractID]pieceData)
	}

	f.mu.RLock()
	for _, contract := range f.contracts {
		id := r.hostContractor.ResolveID(contract.ID)
		for i := range contract.Pieces {
			// Only add pieceSet entries for chunks that are going to be downloaded.
			m, exists := d.pieceSet[contract.Pieces[i].Chunk]
			if exists {
				m[id] = contract.Pieces[i]
			}
		}
	}
	f.mu.RUnlock()
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

// addDownloadToChunkQueue takes a file and adds all incomplete work from the file
// to the renter's chunk queue.
func (r *Renter) addDownloadToChunkQueue(d *download) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Skip this file if it has already errored out or has already finished
	// downloading.
	if d.downloadComplete {
		build.Critical("Complete download was added to chunkQueue")
		return
	}

	// Add the unfinished chunks one at a time.
	for i, isChunkFinished := range d.finishedChunks {
		// New download shouldn't contain finished chunks
		if isChunkFinished {
			build.Critical("New download contains finished chunks")
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

// managedDistributeChunkToWorkers adds the download jobs to every worker that
// holds a piece of the incomplete chunk
func (r *Renter) managedDistributeDownloadsToWorkers(ds *downloadState) {
	// TODO: Make sure we only distribute if there are enough workers
	for len(ds.incompleteChunks) > 0 {
		cd := ds.incompleteChunks[0]
		ds.incompleteChunks = ds.incompleteChunks[1:]

		id := r.mu.RLock()
		workers := make([]*worker, 0, len(cd.workerAttempts))
		for _, worker := range r.workerPool {
			// Make sure the worker contains a relevant piece
			if _, exists := cd.workerAttempts[worker.contract.ID]; exists {
				workers = append(workers, worker)
			}
		}
		r.mu.RUnlock(id)
		for _, worker := range workers {
				worker.managedQueueChunkDownload(ds, cd)
		}
	}
}

// downloadIteration performs one iteration of the download loop.
func (r *Renter) managedDownloadIteration(ds *downloadState) {
	select {
	case d := <-r.newDownloads:
		r.addDownloadToChunkQueue(d)
	case <-r.tg.StopChan():
		return
	default:
	}

	// Add new chunks to the download state to the extent that resources allow.
	r.managedScheduleNewChunks(ds)

	// Update the set of workers to include everyone in the worker pool.
	r.managedUpdateWorkerPool()

	// Distribute the chunks to workers
	r.managedDistributeDownloadsToWorkers(ds)

	// Wait for workers to return after downloading pieces.
	r.managedWaitOnDownloadWork(ds)
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
		memoryAvailable := r.managedMemoryAvailableGet()
		if nextChunk.memoryNeeded > memoryAvailable {
			// TODO wait for memory
			println("out of memory")
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

		// Add an incomplete chunk entry for this chunk
		ds.incompleteChunks = append(ds.incompleteChunks, nextChunk)

		// Reduce the available memory accordingly
		r.managedMemoryAvailableSub(nextChunk.memoryNeeded)
	}
}

// managedWaitOnDownloadWork will wait for workers to return after attempting to
// download a piece.
func (r *Renter) managedWaitOnDownloadWork(ds *downloadState) {
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

	// Fetch the corresponding worker.
	id := r.mu.RLock()
	worker, exists := r.workerPool[workerID]
	r.mu.RUnlock(id)
	if !exists {
		println("worker doesn't exist")
		ds.incompleteChunks = append(ds.incompleteChunks, finishedDownload.chunkDownload)
		return
	}

	// Check for an error.
	cd := finishedDownload.chunkDownload
	if finishedDownload.err != nil {
		r.log.Debugln("Error when downloading a piece:", finishedDownload.err)
		worker.downloadRecentFailure = time.Now()
		// TODO Is this necessary?
		ds.incompleteChunks = append(ds.incompleteChunks, cd)
		return
	}

	// Add this returned piece to the appropriate chunk.
	if _, ok := cd.completedPieces[finishedDownload.pieceIndex]; ok {
		r.log.Debugln("Piece", finishedDownload.pieceIndex, "already added")
		// TODO Is this necessary?
		ds.incompleteChunks = append(ds.incompleteChunks, cd)
		return
	}
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.completedPieces[finishedDownload.pieceIndex] = finishedDownload.data
	atomic.AddUint64(&cd.download.atomicDataReceived, cd.download.reportedPieceSize)

	// If the chunk has completed, perform chunk recovery.
	if len(cd.completedPieces) == cd.download.erasureCode.MinPieces() {
		err := cd.recoverChunk()
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
		availableWorkers: availableWorkers,
		incompleteChunks: make([]*chunkDownload, 0),
		resultChan:       make(chan finishedDownload),
	}
	for {
		if r.tg.Add() != nil {
			return
		}
		select {
		case <-r.tg.StopChan():
			return
		default:
		}
		r.managedDownloadIteration(ds)
		r.tg.Done()
	}
}

// DownloadBufferWriter is a buffer-backed implementation of DownloadWriter.
type DownloadBufferWriter struct {
	data   []byte
	offset int64
}

// NewDownloadBufferWriter creates a new DownloadWriter that writes to a buffer.
func NewDownloadBufferWriter(size uint64, offset int64) *DownloadBufferWriter {
	return &DownloadBufferWriter{
		data:   make([]byte, size),
		offset: offset,
	}
}

// Destination implements the Destination method of the DownloadWriter
// interface and informs callers where this download writer is
// being written to.
func (dw *DownloadBufferWriter) Destination() string {
	return "buffer"
}

// WriteAt writes the passed bytes to the DownloadBuffer.
func (dw *DownloadBufferWriter) WriteAt(bytes []byte, off int64) (int, error) {
	off -= dw.offset
	if len(bytes)+int(off) > len(dw.data) || off < 0 {
		return 0, errors.New("write at specified offset exceeds buffer size")
	}

	i := copy(dw.data[off:], bytes)
	return i, nil
}

// Bytes returns the underlying byte slice of the
// DownloadBufferWriter.
func (dw *DownloadBufferWriter) Bytes() []byte {
	return dw.data
}

// Close() implements DownloadWriter's Close method.
func (dw *DownloadBufferWriter) Close() error {
	return nil
}

// DownloadFileWriter is a file-backed implementation of DownloadWriter.
type DownloadFileWriter struct {
	f        *os.File
	location string
	offset   uint64
	written  uint64
	length   uint64
}

// NewDownloadFileWriter creates a new instance of a DownloadWriter backed by the file named.
func NewDownloadFileWriter(fname string, offset, length uint64) (*DownloadFileWriter, error) {
	l, err := os.OpenFile(fname, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return nil, err
	}
	return &DownloadFileWriter{
		f:        l,
		location: fname,
		offset:   offset,
		written:  0,
		length:   length,
	}, nil
}

// Destination implements the Location method of the DownloadWriter interface
// and informs callers where this download writer is being written to.
func (dw *DownloadFileWriter) Destination() string {
	return dw.location
}

// WriteAt writes the passed bytes at the specified offset.
func (dw *DownloadFileWriter) WriteAt(b []byte, off int64) (int, error) {
	if dw.written+uint64(len(b)) > dw.length {
		// TODO remove this again
		return 0, nil
		build.Critical("DownloadFileWriter write exceeds file length")
	}
	n, err := dw.f.WriteAt(b, off-int64(dw.offset))
	if err != nil {
		return n, err
	}
	dw.written += uint64(n)
	return n, err
}

// Close implements DownloadWriter's Close method and releases the file opened
// by the DownloadFileWriter.
func (dw *DownloadFileWriter) Close() error {
	return dw.f.Close()
}

// DownloadHttpWriter is a http response writer-backed implementation of
// DownloadWriter.  The writer writes all content that is written to the
// current `offset` directly to the ResponseWriter, and buffers all content
// that is written at other offsets.  After every write to the ResponseWriter
// the `offset` and `length` fields are updated, and buffer content written
// until
type DownloadHttpWriter struct {
	w              io.Writer
	offset         int            // The index in the original file of the last byte written to the response writer.
	firstByteIndex int            // The index of the first byte in the original file.
	length         int            // The total size of the slice to be written.
	buffer         map[int][]byte // Buffer used for storing the chunks until download finished.
}

// NewDownloadHttpWriter creates a new instance of http.ResponseWriter backed DownloadWriter.
func NewDownloadHttpWriter(w io.Writer, offset, length uint64) *DownloadHttpWriter {
	return &DownloadHttpWriter{
		w:              w,
		offset:         0,           // Current offset in the output file.
		firstByteIndex: int(offset), // Index of first byte in original file.
		length:         int(length),
		buffer:         make(map[int][]byte),
	}
}

// Destination implements the Destination method of the DownloadWriter
// interface and informs callers where this download writer is
// being written to.
func (dw *DownloadHttpWriter) Destination() string {
	return "httpresp"
}

// Cloes implements DownloadWriter's Close method.
func (dw *DownloadHttpWriter) Close() error {
	return nil
}

// WriteAt buffers parts of the file until the entire file can be
// flushed to the client. Returns the number of bytes written or an error.
func (dw *DownloadHttpWriter) WriteAt(b []byte, off int64) (int, error) {
	// Write bytes to buffer.
	offsetInBuffer := int(off) - dw.firstByteIndex
	dw.buffer[offsetInBuffer] = b

	// Send all chunks to the client that can be sent.
	totalDataSent := 0
	for {
		data, exists := dw.buffer[dw.offset]
		if exists {
			// Send data to client.
			dw.w.Write(data)

			// Remove chunk from map.
			delete(dw.buffer, dw.offset)

			// Increment offset to point to the beginning of the next chunk.
			dw.offset += len(data)
			totalDataSent += len(data)
		} else {
			break
		}
	}

	return totalDataSent, nil
}
