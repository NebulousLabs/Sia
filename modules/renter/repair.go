package renter

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

// incompleteChunks returns a map of chunks in need of repair.
// TODO: inefficient -- O(2n) on number of pieces
func (f *file) incompleteChunks() map[uint64][]uint64 {
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
	return missing
}

// chunkHosts returns the IPs of the hosts storing a given chunk.
func (f *file) chunkHosts(index uint64) []modules.NetAddress {
	var hosts []modules.NetAddress
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			if p.Chunk == index {
				hosts = append(hosts, fc.IP)
				break
			}
		}
	}
	return hosts
}

// repair attempts to repair a file by uploading missing pieces to more hosts.
func (f *file) repair(r io.ReaderAt, pieceMap map[uint64][]uint64, hosts []uploader) error {
	// For each chunk with missing pieces, re-encode the chunk and upload each
	// missing piece.
	var wg sync.WaitGroup
	for chunkIndex, missingPieces := range pieceMap {
		// can only upload to hosts that aren't already storing this chunk
		curHosts := f.chunkHosts(chunkIndex)
		var newHosts []uploader
	outer:
		for _, h := range hosts {
			for _, ip := range curHosts {
				if ip == h.addr() {
					continue outer
				}
			}
			newHosts = append(newHosts, h)
		}
		// don't bother encoding if there aren't any hosts to upload to
		if len(newHosts) == 0 {
			continue
		}

		// read chunk data and encode
		chunk := make([]byte, f.chunkSize())
		_, err := r.ReadAt(chunk, int64(chunkIndex*f.chunkSize()))
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}
		pieces, err := f.erasureCode.Encode(chunk)
		if err != nil {
			return err
		}

		// upload pieces, split evenly among hosts
		wg.Add(len(missingPieces))
		for j, pieceIndex := range missingPieces {
			host := newHosts[j%len(newHosts)]
			up := uploadPiece{pieces[pieceIndex], chunkIndex, pieceIndex}
			go func(host uploader, up uploadPiece) {
				_ = host.addPiece(up)
				wg.Done()
			}(host, up)
		}
		wg.Wait()

		// update contracts
		for _, h := range hosts {
			contract := h.fileContract()
			f.contracts[contract.ID] = contract
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

			// determine file health
			missingPieceMap := f.incompleteChunks()
			if len(missingPieceMap) == 0 {
				// nothing to do
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

			err = f.repair(handle, missingPieceMap, hosts)
			if err != nil {
				r.log.Printf("failed to repair %v: %v", name, err)
				id = r.mu.Lock()
				delete(r.repairSet, name)
				r.mu.Unlock(id)
			}

			// save the repaired file data
			err = r.saveFile(f)
			if err != nil {
				// definitely bad, but we probably shouldn't delete from the
				// repair set if this happens
				r.log.Printf("failed to save repaired file %v: %v", name, err)
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
