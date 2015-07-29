package renter

import (
	"io"

	"github.com/klauspost/reedsolomon"

	"github.com/NebulousLabs/Sia/modules"
)

// rsCode is a Reed-Solomon encoder/decoder. It implements the modules.ECC
// interface.
type rsCode struct {
	enc reedsolomon.Encoder

	numPieces int
}

// NumPieces returns the number of pieces returned by Encode.
func (rs *rsCode) NumPieces() int { return rs.numPieces }

// Encode splits data into equal-length pieces, some containing the original
// data and some containing parity data.
func (rs *rsCode) Encode(data []byte) ([][]byte, error) {
	pieces, err := rs.enc.Split(data)
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
func NewRSCode(nData, nParity int) (modules.ECC, error) {
	enc, err := reedsolomon.New(nData, nParity)
	if err != nil {
		return nil, err
	}
	return &rsCode{
		enc:       enc,
		chunk:     make([]byte, chunksize),
		numPieces: nData + nParity,
	}, nil
}
