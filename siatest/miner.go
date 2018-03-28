package siatest

import (
	"bytes"
	"errors"
	"unsafe"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// MineBlock makes the underlying node mine a single block and broadcast it.
func (tn *TestNode) MineBlock() error {
	// Get the header
	target, header, err := tn.MinerHeaderGet()
	if err != nil {
		return build.ExtendErr("failed to get header for work", err)
	}
	// Solve the header
	header, err = solveHeader(target, header)
	if err != nil {
		return build.ExtendErr("failed to solve header", err)
	}
	// Submit the header
	if err := tn.MinerHeaderPost(header); err != nil {
		return build.ExtendErr("failed to submit header", err)
	}
	return nil
}

// solveHeader solves the header by finding a nonce for the target
func solveHeader(target types.Target, bh types.BlockHeader) (types.BlockHeader, error) {
	header := encoding.Marshal(bh)
	var nonce uint64
	for i := 0; i < 256; i++ {
		id := crypto.HashBytes(header)
		if bytes.Compare(target[:], id[:]) >= 0 {
			copy(bh.Nonce[:], header[32:40])
			return bh, nil
		}
		*(*uint64)(unsafe.Pointer(&header[32])) = nonce
		nonce++
	}
	return bh, errors.New("couldn't solve block")
}
