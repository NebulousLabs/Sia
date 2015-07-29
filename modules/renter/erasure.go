package renter

import (
	"io"

	"github.com/klauspost/reedsolomon"

	"github.com/NebulousLabs/Sia/modules"
)

// rsCode is a Reed-Solomon encoder/decoder. It implements the modules.ECC
// interface.
type rsCode struct {
	enc   reedsolomon.Encoder
	chunk []byte // allocated during initialization to save memory

	chunkSize uint64
	numPieces int
}

func (rs *rsCode) ChunkSize() uint64 { return rs.chunkSize }

func (rs *rsCode) NumPieces() int { return rs.numPieces }

// Encode reads a chunk from r and splits it into equal-length pieces. If a
// full chunk cannot be read, the remainder of the chunk will contain zeros.
func (rs *rsCode) Encode(r io.Reader) ([][]byte, error) {
	_, err := io.ReadFull(r, rs.chunk)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	pieces, err := rs.enc.Split(rs.chunk)
	if err != nil {
		return nil, err
	}
	err = rs.enc.Encode(pieces)
	if err != nil {
		return nil, err
	}
	return pieces, nil
}

// Recover recovers the original data from pieces (including parity) and
// writes it to w. pieces should be identical to the slice returned by
// Encode (length and order must be preserved), but with missing elements
// set to nil.
func (rs *rsCode) Recover(pieces [][]byte, w io.Writer) error {
	err := rs.enc.Reconstruct(pieces)
	if err != nil {
		return err
	}
	// TODO: implement this manually
	return rs.enc.Join(w, pieces, int(rs.chunkSize))
}

// NewRSCode creates a new Reed-Solomon encoder/decoder using the supplied
// parameters.
func NewRSCode(nData, nParity int, chunksize uint64) (modules.ECC, error) {
	enc, err := reedsolomon.New(nData, nParity)
	if err != nil {
		return nil, err
	}
	return &rsCode{
		enc:       enc,
		chunk:     make([]byte, chunksize),
		chunkSize: chunksize,
		numPieces: nData + nParity,
	}, nil
}
