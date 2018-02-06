package renter

// Downloads can be written directly to a file, can be written to an http
// stream, or can be written to an in-memory buffer. The core download loop only
// has the concept of writing using WriteAt, so to support writing to a stream
// or to an in-memory buffer, we need to wrap the function with something that
// will transform the WriteAt call into an in-order stream or otherwise write it
// to the right place.

// DownloadBufferWriter will write the results of a download to an in-memory
// buffer.
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

// Close implements DownloadWriter's Close method.
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

// DownloadHTTPWriter is a http response writer-backed implementation of
// DownloadWriter.  The writer writes all content that is written to the
// current `offset` directly to the ResponseWriter, and buffers all content
// that is written at other offsets.  After every write to the ResponseWriter
// the `offset` and `length` fields are updated, and buffer content written
// until
type DownloadHTTPWriter struct {
	w              io.Writer
	offset         int            // The index in the original file of the last byte written to the response writer.
	firstByteIndex int            // The index of the first byte in the original file.
	length         int            // The total size of the slice to be written.
	buffer         map[int][]byte // Buffer used for storing the chunks until download finished.
}

// NewDownloadHTTPWriter creates a new instance of http.ResponseWriter backed DownloadWriter.
func NewDownloadHTTPWriter(w io.Writer, offset, length uint64) *DownloadHTTPWriter {
	return &DownloadHTTPWriter{
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
func (dw *DownloadHTTPWriter) Destination() string {
	return "httpresp"
}

// Close implements DownloadWriter's Close method.
func (dw *DownloadHTTPWriter) Close() error {
	return nil
}

// WriteAt buffers parts of the file until the entire file can be
// flushed to the client. Returns the number of bytes written or an error.
func (dw *DownloadHTTPWriter) WriteAt(b []byte, off int64) (int, error) {
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
