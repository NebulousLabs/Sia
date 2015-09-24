package renter

import (
	"io"
	"sync"
	"sync/atomic"
)

// repair attempts to repair a file by uploading missing pieces to more hosts.
func (f *file) repair(r io.ReaderAt, hosts []uploader) error {
	// determine which chunks need to be repaired
	// TODO: inefficient -- O(2n) on number of pieces
	present := make([][]bool, f.numChunks())
	for i := range present {
		present[i] = make([]bool, f.erasureCode.NumPieces())
	}
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			present[p.Chunk][p.Piece] = true
		}
	}
	missing := make(map[uint64][]uint64)
	for chunkIndex, pieceBools := range present {
		for pieceIndex, ok := range pieceBools {
			if !ok {
				missing[uint64(chunkIndex)] = append(missing[uint64(chunkIndex)], uint64(pieceIndex))
			}
		}
	}

	// For each chunk with missing pieces, re-encode the chunk and upload each
	// missing piece.
	var wg sync.WaitGroup
	for chunkIndex, missingPieces := range missing {
		// read chunk data
		// NOTE: ReadAt is stricter than Read, and is guaranteed to return an
		// error after a partial read.
		chunk := make([]byte, f.chunkSize())
		_, err := r.ReadAt(chunk, int64(chunkIndex*f.chunkSize()))
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}

		// encode
		pieces, err := f.erasureCode.Encode(chunk)
		if err != nil {
			return err
		}
		// upload pieces, split evenly among hosts
		wg.Add(len(missingPieces))
		for j, pieceIndex := range missingPieces {
			host := hosts[j%len(hosts)]
			up := uploadPiece{pieces[pieceIndex], chunkIndex, pieceIndex}
			go func(host uploader, up uploadPiece) {
				err := host.addPiece(up)
				if err == nil {
					atomic.AddUint64(&f.bytesUploaded, uint64(len(up.data)))
				}
				wg.Done()
			}(host, up)
		}
		wg.Wait()
		atomic.AddUint64(&f.chunksUploaded, 1)

		// update contracts
		for _, h := range hosts {
			contract := h.fileContract()
			f.contracts[contract.IP] = contract
		}
	}

	return nil
}

// threadedRepairUploads improves the health of files tracked by the renter by
// reuploading their missing pieces. Multiple repair attempts may be necessary
// before the file reaches full redundancy.
//
// TODO: can't have this running on foreign .sia files...
func (r *Renter) threadedRepairUploads() {

}
