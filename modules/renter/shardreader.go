package renter

import (
	"io"
)

// shardReader is a helper struct that can read data into shards of
// modules.SectorSize instead of whole byte slices.
type shardReader struct {
	r io.ReaderAt
}

// NewShardReader creates a new shardReader from an object that implements the
// ReaderAt interface.
func NewShardReader(r io.ReaderAt) *shardReader {
	return &shardReader{
		r: r,
	}
}

// ReadAt reads data into a slice of shards from a certain offset.
func (sr *shardReader) ReadAt(d [][]byte, offset int64) (int, error) {
	var n int
	for len(d) > 0 {
		read, err := sr.r.ReadAt(d[0], offset)
		if err != nil {
			return 0, err
		}
		d = d[1:]
		offset += int64(read)
		n += read
	}
	return n, nil
}
