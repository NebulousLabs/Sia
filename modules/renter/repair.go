package renter

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
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
		// TODO: ping host, and don't mark pieces if unresponsive?
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
func (r *Renter) threadedRepairUploads() {
	for {
		time.Sleep(5 * time.Second)

		// make copy of repair set under lock
		repairing := make(map[string]string)
		id := r.mu.RLock()
		for name, path := range r.repairSet {
			repairing[name] = path
		}
		r.mu.RUnlock(id)

		for name, path := range repairing {
			// retrieve file object
			id = r.mu.RLock()
			f, ok := r.files[name]
			r.mu.RUnlock(id)
			if !ok {
				r.log.Printf("failed to repair %v: no longer tracking that file", name)
				id = r.mu.Lock()
				delete(r.repairSet, name)
				r.mu.Unlock(id)
				continue
			}

			// open file handle
			handle, err := os.Open(path)
			if err != nil {
				r.log.Printf("failed to repair %v: %v", name, err)
				id = r.mu.Lock()
				delete(r.repairSet, name)
				r.mu.Unlock(id)
				continue
			}

			// build host list
			var hosts []uploader
			randHosts := r.hostDB.RandomHosts(f.erasureCode.NumPieces())
			for i := range randHosts {
				// TODO: use smarter duration
				hostUploader, err := r.newHostUploader(randHosts[i], f.size, defaultDuration, f.masterKey)
				if err != nil {
					continue
				}
				defer hostUploader.Close()
				hosts = append(hosts, hostUploader)
			}

			// repair
			err = f.repair(handle, hosts)
			if err != nil {
				r.log.Printf("failed to repair %v: %v", name, err)
				id = r.mu.Lock()
				delete(r.repairSet, name)
				r.mu.Unlock(id)
			}

			// close the file
			handle.Close()

			// close the host connections
			for i := range hosts {
				hosts[i].(*hostUploader).Close()
			}
		}

	}
}
