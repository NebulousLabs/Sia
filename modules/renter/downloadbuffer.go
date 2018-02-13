package renter

// Downloads can be written directly to a file, can be written to an http
// stream, or can be written to an in-memory buffer. The core download loop only
// has the concept of writing using WriteAt, so to support writing to a stream
// or to an in-memory buffer, we need to wrap the function with something that
// will transform the WriteAt call into an in-order stream or otherwise write it
// to the right place.

import (
	"errors"
	"io"
	"sync"
)

// downloadDestination is a wrapper for the different types of writing that we
// can do when reovering and writing the logical data of a file. The wrapper
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
type downloadDestinationBuffer []byte

// Close implements Close for the downloadDestination interface.
func (dw downloadDestinationBuffer) Close() error {
	return nil
}

// WriteAt writes the provided data to the downloadDestinationBuffer.
func (dw downloadDestinationBuffer) WriteAt(data []byte, offset int64) (int, error) {
	if len(data)+int(offset) > len(dw) || offset < 0 {
		return 0, errors.New("write at specified offset exceeds buffer size")
	}
	i := copy(dw[offset:], data)
	return i, nil
}

// downloadDestinationWriter is a downloadDestination that writes to an
// underlying data stream. The data stream is expecting sequential data while
// the download chunks will be written in an aribtrary order using calls to
// WriteAt. We need to block the calls to WriteAt until all prior data has been
// written.
//
// NOTE: If the caller accedentally leaves a gap between calls to WriteAt, for
// example writes bytes 0-100 and then writes bytes 110-200, and accidentally
// never writes bytes 100-110, the downloadDestinationWriter will block forever
// waiting for those gap bytes to be written.
//
// NOTE: Calling WriteAt has linear time performance in the number of concurrent
// calls to WriteAt.
type downloadDestinationWriter struct {
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

// newDownloadDestinationWriter takes a writer and converts it into a
func newDownloadDestinationWriter(w io.WriteCloser) downloadDestination {
	return &downloadDestinationWriter{WriteCloser: w}
}

// unblockNextWrites will iterate over all of the blocking write calls and
// unblock any whose offsets have been reached by the current progress of the
// stream.
//
// NOTE: unblockNextWrites has linear time performance in the number of currently
// blocking calls.
func (ddw *downloadDestinationWriter) unblockNextWrites() {
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
func (ddw *downloadDestinationWriter) Close() error {
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
func (ddw *downloadDestinationWriter) WriteAt(data []byte, offset int64) (int, error) {
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

// httpWriteCloser wraps an hhtpWriter with a closer function so that it can be
// passed to the newDownloadDestinationWriter function.
type httpWriteCloser struct {
	io.Writer
}

// Close is a blank function that allows an httpWriter to become an
// io.WriteCloser.
func (httpWriteCloser) Close() error { return nil }

// newDownloadDestinationHTTPWriter wraps an io.Writer (typically an HTTPWriter)
// with a do-nothing Close function so that it satisfies the WriteCloser
// interface.
//
// TODO: Reconsider the name of this funciton.
func newDownloadDestinationHTTPWriter(w io.Writer) downloadDestination {
	var hwc httpWriteCloser
	hwc.Writer = w
	return newDownloadDestinationWriter(hwc)
}
