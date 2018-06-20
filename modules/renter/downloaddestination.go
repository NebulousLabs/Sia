package renter

// Downloads can be written directly to a file, can be written to an http
// stream, or can be written to an in-memory buffer. The core download loop only
// has the concept of writing using WriteAt, and then calling Close when the
// download is complete.
//
// To support streaming and writing to memory buffers, the downloadDestination
// interface exists. It is used to map things like a []byte or an io.WriteCloser
// to a downloadDestination. This interface is implemented by:
//		+ os.File
//		+ downloadDestinationBuffer (an alias of a []byte)
//		+ downloadDestinationWriteCloser (created using an io.WriteCloser)
//
// There is also a helper function to convert an io.Writer to an io.WriteCloser,
// so that an io.Writer can be used to create a downloadDestinationWriteCloser
// as well.

import (
	"errors"
	"io"
	"sync"
)

// downloadDestination is a wrapper for the different types of writing that we
// can do when recovering and writing the logical data of a file. The wrapper
// needs to convert the various write-at calls into writes that make sense to
// the underlying file, buffer, or stream.
//
// For example, if the underlying object is a file, the WriteAt call is just a
// passthrough function. But if the underlying object is a stream, WriteAt may
// block while it waits for previous data to be written.
type downloadDestination interface {
	Close() error
	WriteAt(data []byte, offset int64) (int, error)
}

// downloadDestinationBuffer writes logical chunk data to an in-memory buffer.
// This buffer is primarily used when performing repairs on uploads.
type downloadDestinationBuffer [][]byte

// NewDownloadDestinationBuffer allocates the necessary number of shards for
// the downloadDestinationBuffer and returns the new buffer.
func NewDownloadDestinationBuffer(length uint64) downloadDestinationBuffer {
	// Round length up to next multiple of SectorSize.
	if length%pieceSize != 0 {
		length += pieceSize - length%pieceSize
	}
	buf := make([][]byte, 0, length/pieceSize)
	for length > 0 {
		buf = append(buf, make([]byte, pieceSize))
		length -= pieceSize
	}
	return buf
}

// Close implements Close for the downloadDestination interface.
func (dw downloadDestinationBuffer) Close() error {
	return nil
}

// ReadFrom reads data from a io.Reader until the buffer is full.
func (dw downloadDestinationBuffer) ReadFrom(r io.Reader) (int64, error) {
	var n int64
	for len(dw) > 0 {
		read, err := io.ReadFull(r, dw[0])
		if err != nil {
			return n, err
		}
		dw = dw[1:]
		n += int64(read)
	}
	return n, nil
}

// WriteAt writes the provided data to the downloadDestinationBuffer.
func (dw downloadDestinationBuffer) WriteAt(data []byte, offset int64) (int, error) {
	if uint64(len(data))+uint64(offset) > uint64(len(dw))*pieceSize || offset < 0 {
		return 0, errors.New("write at specified offset exceeds buffer size")
	}
	written := len(data)
	for len(data) > 0 {
		shardIndex := offset / int64(pieceSize)
		sliceIndex := offset % int64(pieceSize)
		n := copy(dw[shardIndex][sliceIndex:], data)
		data = data[n:]
		offset += int64(n)
	}
	return written, nil
}

// downloadDestinationWriteCloser is a downloadDestination that writes to an
// underlying data stream. The data stream is expecting sequential data while
// the download chunks will be written in an arbitrary order using calls to
// WriteAt. We need to block the calls to WriteAt until all prior data has been
// written.
//
// NOTE: If the caller accidentally leaves a gap between calls to WriteAt, for
// example writes bytes 0-100 and then writes bytes 110-200, and accidentally
// never writes bytes 100-110, the downloadDestinationWriteCloser will block
// forever waiting for those gap bytes to be written.
//
// NOTE: Calling WriteAt has linear time performance in the number of concurrent
// calls to WriteAt.
type downloadDestinationWriteCloser struct {
	closed         bool
	mu             sync.Mutex // Protects the underlying data structures.
	progress       int64      // How much data has been written yet.
	io.WriteCloser            // The underlying writer.

	// A list of write calls and their corresponding locks. When one write call
	// completes, it'll search through the list of write calls for the next one.
	// The next write call can be unblocked by unlocking the corresponding mutex
	// in the next array.
	blockingWriteCalls   []int64 // A list of write calls that are waiting for their turn
	blockingWriteSignals []*sync.Mutex
}

var (
	// errClosedStream gets returned if the stream was closed but we are trying
	// to write.
	errClosedStream = errors.New("unable to write because stream has been closed")

	// errOffsetAlreadyWritten gets returned if a call to WriteAt tries to write
	// to a place in the stream which has already had data written to it.
	errOffsetAlreadyWritten = errors.New("cannot write to that offset in stream, data already written")
)

// newDownloadDestinationWriteCloser takes an io.WriteCloser and converts it
// into a downloadDestination.
func newDownloadDestinationWriteCloser(w io.WriteCloser) downloadDestination {
	return &downloadDestinationWriteCloser{WriteCloser: w}
}

// unblockNextWrites will iterate over all of the blocking write calls and
// unblock any whose offsets have been reached by the current progress of the
// stream.
//
// NOTE: unblockNextWrites has linear time performance in the number of currently
// blocking calls.
func (ddw *downloadDestinationWriteCloser) unblockNextWrites() {
	for i, offset := range ddw.blockingWriteCalls {
		if offset <= ddw.progress {
			ddw.blockingWriteSignals[i].Unlock()
			ddw.blockingWriteCalls = append(ddw.blockingWriteCalls[0:i], ddw.blockingWriteCalls[i+1:]...)
			ddw.blockingWriteSignals = append(ddw.blockingWriteSignals[0:i], ddw.blockingWriteSignals[i+1:]...)
		}
	}
}

// Close will unblock any hanging calls to WriteAt, and then call Close on the
// underlying WriteCloser.
func (ddw *downloadDestinationWriteCloser) Close() error {
	ddw.mu.Lock()
	if ddw.closed {
		ddw.mu.Unlock()
		return errClosedStream
	}
	ddw.closed = true
	for i := range ddw.blockingWriteSignals {
		ddw.blockingWriteSignals[i].Unlock()
	}
	ddw.mu.Unlock()
	return ddw.WriteCloser.Close()
}

// WriteAt will block until the stream has progressed to 'offset', and then it
// will write its own data. An error will be returned if the stream has already
// progressed beyond 'offset'.
func (ddw *downloadDestinationWriteCloser) WriteAt(data []byte, offset int64) (int, error) {
	write := func() (int, error) {
		// Error if the stream has been closed.
		if ddw.closed {
			return 0, errClosedStream
		}
		// Error if the stream has progressed beyond 'offset'.
		if offset < ddw.progress {
			ddw.mu.Unlock()
			return 0, errOffsetAlreadyWritten
		}

		// Write the data to the stream, and the update the progress and unblock
		// the next write.
		n, err := ddw.Write(data)
		ddw.progress += int64(n)
		ddw.unblockNextWrites()
		return n, err
	}

	ddw.mu.Lock()
	// Attempt to write if the stream progress is at or beyond the offset. The
	// write call will perform error handling.
	if offset <= ddw.progress {
		n, err := write()
		ddw.mu.Unlock()
		return n, err
	}

	// The stream has not yet progressed to 'offset'. We will block until the
	// stream has made progress. We perform the block by creating a
	// thread-specific mutex 'myMu' and adding it to the object's list of
	// blocking threads. When other threads successfully call WriteAt, they will
	// reference this list and unblock any which have enough progress. The
	// result is a somewhat strange construction where we lock myMu twice in a
	// row, but between those two calls to lock, we put myMu in a place where
	// another thread can unlock myMu.
	//
	// myMu will be unblocked when another thread calls 'unblockNextWrites'.
	myMu := new(sync.Mutex)
	myMu.Lock()
	ddw.blockingWriteCalls = append(ddw.blockingWriteCalls, offset)
	ddw.blockingWriteSignals = append(ddw.blockingWriteSignals, myMu)
	ddw.mu.Unlock()
	myMu.Lock()
	ddw.mu.Lock()
	n, err := write()
	ddw.mu.Unlock()
	return n, err
}

// writerToWriteCloser will convert an io.Writer to an io.WriteCloser by adding
// a Close function which always returns nil.
type writerToWriteCloser struct {
	io.Writer
}

// Close will always return nil.
func (writerToWriteCloser) Close() error { return nil }

// newDownloadDestinationWriteCloserFromWriter will return a
// downloadDestinationWriteCloser taking an io.Writer as input. The io.Writer
// will be wrapped with a Close function which always returns nil. If the
// underlying object is an io.WriteCloser, newDownloadDestinationWriteCloser
// should be called instead.
//
// This function is primarily used with http streams, which do not implement a
// Close function.
func newDownloadDestinationWriteCloserFromWriter(w io.Writer) downloadDestination {
	return newDownloadDestinationWriteCloser(writerToWriteCloser{Writer: w})
}
