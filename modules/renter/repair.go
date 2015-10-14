package renter

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// When a file contract is within this many blocks of expiring, the renter
	// will attempt to reupload the data covered by the contract.
	renewThreshold = 2000
)

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

// incompleteChunks returns a map of chunks in need of repair.
func (f *file) incompleteChunks() map[uint64][]uint64 {
	present := make([][]bool, f.numChunks())
	for i := range present {
		present[i] = make([]bool, f.erasureCode.NumPieces())
	}
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			present[p.Chunk][p.Piece] = true
		}
	}
	incomplete := make(map[uint64][]uint64)
	for chunkIndex, pieceBools := range present {
		for pieceIndex, ok := range pieceBools {
			if !ok {
				incomplete[uint64(chunkIndex)] = append(incomplete[uint64(chunkIndex)], uint64(pieceIndex))
			}
		}
	}
	return incomplete
}

// expiringChunks returns a map of chunks whose pieces will expire within
// 'renewThreshold' blocks.
func (f *file) expiringChunks(currentHeight types.BlockHeight) map[uint64][]uint64 {
	expiring := make(map[uint64][]uint64)
	for _, fc := range f.contracts {
		if currentHeight >= fc.WindowStart-renewThreshold {
			// mark every piece in the chunk
			for _, p := range fc.Pieces {
				expiring[p.Chunk] = append(expiring[p.Chunk], p.Piece)
			}
		}
	}
	return expiring
}

// repair attempts to repair a file by uploading missing pieces to more hosts.
func (f *file) repair(r io.ReaderAt, pieceMap map[uint64][]uint64, hosts []uploader) error {
	// For each chunk with missing pieces, re-encode the chunk and upload each
	// missing piece.
	var wg sync.WaitGroup
	for chunkIndex, missingPieces := range pieceMap {
		// can only upload to hosts that aren't already storing this chunk
		// TODO: what if we're renewing?
		f.mu.RLock()
		curHosts := f.chunkHosts(chunkIndex)
		f.mu.RUnlock()
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
		f.mu.Lock()
		for _, h := range hosts {
			contract := h.fileContract()
			f.contracts[contract.ID] = contract
		}
		f.mu.Unlock()
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
			// retrieve file object and get current height
			id = r.mu.RLock()
			f, ok := r.files[name]
			height := r.blockHeight
			r.mu.RUnlock(id)
			if !ok {
				r.log.Printf("failed to repair %v: no longer tracking that file", name)
				id = r.mu.Lock()
				delete(r.repairSet, name)
				r.mu.Unlock(id)
				continue
			}

			// determine file health
			f.mu.RLock()
			badChunks := f.incompleteChunks()
			if len(badChunks) == 0 {
				badChunks = f.expiringChunks(height)
				if len(badChunks) == 0 {
					// nothing to do
					f.mu.RUnlock()
					continue
				}
			}
			f.mu.RUnlock()

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

			err = f.repair(handle, badChunks, hosts)
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
