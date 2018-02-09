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
// TODO: There is no timeout protection here. If there's some misalignment of
// data, we'll never know it'll just hang forever.
type downloadDestinationWriter struct {
	failed         bool
	mu             sync.Mutex // Protects the underlying data structures.
	progress       int64      // How much data has been written yet.
	io.WriteCloser            // The underlying writer.

	// A list of write calls and their corresponding locks. When one write call
	// completes, it'll search through the list of write calls for the next one.
	// The next write call can be unblocked by unlocking the corresponding mutex
	// in the next array.
	blockingWriteCalls   []int64 // A list of write calls that are waiting for their turn
	blockingWriteSignals []sync.Mutex
}

// errFailedStreamWrite gets returned if a prior error occurred when writing to
// the stream.
var errFailedStreamWrite = errors.New("downloadDestinationWriter has a broken stream due to a prior failed write")

// newDownloadDestinationWriter takes a writer and converts it into a
func newDownloadDestinationWriter(w io.WriteCloser) downloadDestination {
	return &downloadDestinationWriter{WriteCloser: w}
}

// nextWrite will iterate over all of the blocking write calls and unblock the
// one that is next in line, if the next-in-line call is available.
func (ddw *downloadDestinationWriter) nextWrite() {
	for i, offset := range ddw.blockingWriteCalls {
		if offset == ddw.progress {
			ddw.blockingWriteSignals[i].Unlock()
			ddw.blockingWriteCalls = append(ddw.blockingWriteCalls[0:i], ddw.blockingWriteCalls[i+1:]...)
			ddw.blockingWriteSignals = append(ddw.blockingWriteSignals[0:i], ddw.blockingWriteSignals[i+1:]...)
			return
		}
		if offset < ddw.progress {
			// Sanity check - there should not be a call to WriteAt that occurs
			// earlier than the current progress. If there is, the
			// downloadDestinationWriter is being used incorrectly in an
			// unrecoverable way.
			panic("incorrect write order for downloadDestinationWriter")
		}
	}
}

// WriteAt will block until the stream has progressed to or past 'offset', and
// then it will write its own data.
func (ddw *downloadDestinationWriter) WriteAt(data []byte, offset int64) (int, error) {
	write := func() (int, error) {
		// If the stream writer has already failed, return an error.
		if ddw.failed {
			return 0, errFailedStreamWrite
		}

		// Write the data to the stream.
		n, err := ddw.Write(data)
		if err != nil {
			// If there is an error, marked the stream write as failed and then
			// unlock/unblock all of the waiting WriteAt calls.
			ddw.failed = true
			ddw.Close()
			for _, mu := range ddw.blockingWriteSignals {
				mu.Unlock()
			}
			return n, err
		}

		// Update the progress and unblock the next write.
		ddw.progress += int64(n)
		ddw.nextWrite()
		return n, nil
	}

	ddw.mu.Lock()
	// Check if the stream has already failed. If so, return immediately with
	// the failed stream error.
	if ddw.failed {
		ddw.mu.Unlock()
		return 0, errFailedStreamWrite
	}

	// Check if we are writing to the correct offset for the stream. If so, call
	// write() and return.
	if offset == ddw.progress {
		// This write is the next write in line.
		n, err := write()
		ddw.mu.Unlock()
		return n, err
	}

	// Block until we are the correct offset for the stream. The blocking is
	// coordinated by a new mutex which gets added to an array. When the earlier
	// data is written, the mutex will be unlocked, allowing us to write.
	var myMu sync.Mutex
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

func newDownloadDestinationHTTPWriter(w io.Writer) downloadDestination {
	var hwc httpWriteCloser
	hwc.Writer = w
	return newDownloadDestinationWriter(hwc)
}
