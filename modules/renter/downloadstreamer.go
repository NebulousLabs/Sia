package renter

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/NebulousLabs/errors"
)

type (
	// streamer is a io.ReadSeeker that can be used to stream downloads from
	// the sia network.
	streamer struct {
		file   *file
		offset int64
		r      *Renter
	}
)

// min is a helper function to find the minimum of multiple values.
func min(values ...uint64) uint64 {
	min := uint64(math.MaxUint64)
	for _, v := range values {
		if v < min {
			min = v
		}
	}
	return min
}

// Streamer creates an io.ReadSeeker that can be used to stream downloads from
// the sia network.
func (r *Renter) Streamer(siaPath string) (string, io.ReadSeeker, error) {
	// Lookup the file associated with the nickname.
	lockID := r.mu.RLock()
	file, exists := r.files[siaPath]
	r.mu.RUnlock(lockID)
	if !exists || file.Deleted() {
		return "", nil, fmt.Errorf("no file with that path: %s", siaPath)
	}
	// Create the streamer
	s := &streamer{
		file: file,
		r:    r,
	}
	return file.Name(), s, nil
}

// Read implements the standard Read interface. It will download the requested
// data from the sia network and block until the download is complete.  To
// prevent http.ServeContent from requesting too much data at once, Read can
// only request a single chunk at once.
func (s *streamer) Read(p []byte) (n int, err error) {
	// Get the file's size
	s.file.mu.RLock()
	fileSize := int64(s.file.size)
	s.file.mu.RUnlock()

	// Make sure we haven't reached the EOF yet.
	if s.offset >= fileSize {
		return 0, io.EOF
	}

	// Calculate how much we can download. We never download more than a single chunk.
	chunkSize := s.file.staticChunkSize()
	remainingData := uint64(fileSize - s.offset)
	requestedData := uint64(len(p))
	remainingChunk := chunkSize - uint64(s.offset)%chunkSize
	length := min(remainingData, requestedData, remainingChunk)

	// Download data
	buffer := bytes.NewBuffer([]byte{})
	d, err := s.r.managedNewDownload(downloadParams{
		destination:       newDownloadDestinationWriteCloserFromWriter(buffer),
		destinationType:   destinationTypeSeekStream,
		destinationString: "httpresponse",
		file:              s.file,

		latencyTarget: 50 * time.Millisecond, // TODO low default until full latency suport is added.
		length:        length,
		needsMemory:   true,
		offset:        uint64(s.offset),
		overdrive:     5,    // TODO: high default until full overdrive support is added.
		priority:      1000, // TODO: high default until full priority support is added.
	})
	if err != nil {
		return 0, errors.AddContext(err, "failed to create new download")
	}

	// Set the in-memory buffer to nil just to be safe in case of a memory
	// leak.
	defer func() {
		d.destination = nil
	}()

	// Block until the download has completed.
	select {
	case <-d.completeChan:
		if d.Err() != nil {
			return 0, errors.AddContext(d.Err(), "download failed")
		}
	case <-s.r.tg.StopChan():
		return 0, errors.New("download interrupted by shutdown")
	}

	// Copy downloaded data into buffer.
	copy(p, buffer.Bytes())

	// Adjust offset
	s.offset += int64(length)
	return int(length), nil
}

// Seek sets the offset for the next Read to offset, interpreted
// according to whence: SeekStart means relative to the start of the file,
// SeekCurrent means relative to the current offset, and SeekEnd means relative
// to the end. Seek returns the new offset relative to the start of the file
// and an error, if any.
func (s *streamer) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = 0
	case io.SeekCurrent:
		newOffset = s.offset
	case io.SeekEnd:
		s.file.mu.RLock()
		newOffset = int64(s.file.size)
		s.file.mu.RUnlock()
	}
	newOffset += offset

	if newOffset < 0 {
		return s.offset, errors.New("cannot seek to negative offset")
	}
	s.offset = newOffset
	return s.offset, nil
}
