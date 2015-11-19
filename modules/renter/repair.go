package renter

import (
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// When a file contract is within this many blocks of expiring, the renter
	// will attempt to reupload the data covered by the contract.
	renewThreshold = 2000

	hostTimeout = 15 * time.Second
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

// A repairMap is a mapping of chunks to pieces that need repair.
type repairMap map[uint64][]uint64

// subtract deletes the pieces in r that also appear in m, and returns r. If a
// key contains no pieces, it is removed from the map.
func (r repairMap) subtract(m repairMap) repairMap {
	for chunk, mPieces := range m {
		if _, ok := r[chunk]; !ok {
			continue
		}
		for _, mPieceIndex := range mPieces {
			for i, rPieceIndex := range r[chunk] {
				if rPieceIndex == mPieceIndex {
					// remove i'th element
					r[chunk] = append(r[chunk][:i], r[chunk][i+1:]...)
					break
				}
			}
			// delete empty keys
			if len(r[chunk]) == 0 {
				delete(r, chunk)
				break
			}
		}
	}
	return r
}

// incompleteChunks returns a map of chunks containing pieces that have not
// been uploaded.
func (f *file) incompleteChunks() repairMap {
	present := make([][]bool, f.numChunks())
	for i := range present {
		present[i] = make([]bool, f.erasureCode.NumPieces())
	}
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			present[p.Chunk][p.Piece] = true
		}
	}
	incomplete := make(repairMap)
	for chunkIndex, pieceBools := range present {
		for pieceIndex, ok := range pieceBools {
			if !ok {
				incomplete[uint64(chunkIndex)] = append(incomplete[uint64(chunkIndex)], uint64(pieceIndex))
			}
		}
	}
	return incomplete
}

// chunksBelow returns a map of chunks whose pieces will expire before the
// desired endHeight. Importantly, it will not return pieces that have
// non-expiring duplicates; this prevents the renter from repairing the same
// chunks over and over.
func (f *file) chunksBelow(endHeight types.BlockHeight) repairMap {
	expiring := make(repairMap)
	nonexpiring := make(repairMap)
	for _, fc := range f.contracts {
		// mark every piece in the chunk
		for _, p := range fc.Pieces {
			if fc.WindowStart < endHeight {
				expiring[p.Chunk] = append(expiring[p.Chunk], p.Piece)
			} else {
				nonexpiring[p.Chunk] = append(nonexpiring[p.Chunk], p.Piece)
			}
		}
	}
	return expiring.subtract(nonexpiring)
}

// offlineChunks returns a map of chunks whose pieces are not
// immediately available for download.
func (f *file) offlineChunks() repairMap {
	offline := make(repairMap)
	online := make(repairMap)
	var mapLock sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(f.contracts))
	for _, fc := range f.contracts {
		go func(fc fileContract) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", string(fc.IP), hostTimeout)
			if err == nil {
				conn.Close()
			}
			mapLock.Lock()
			for _, p := range fc.Pieces {
				if err == nil {
					online[p.Chunk] = append(online[p.Chunk], p.Piece)
				} else {
					offline[p.Chunk] = append(offline[p.Chunk], p.Piece)
				}
			}
			mapLock.Unlock()
		}(fc)
	}
	wg.Wait()
	return offline.subtract(online)
}

// repair attempts to repair a file by uploading missing pieces to more hosts.
func (f *file) repair(r io.ReaderAt, pieceMap repairMap, hdb hostDB) error {
	// For each chunk with missing pieces, re-encode the chunk and upload each
	// missing piece.
	var wg sync.WaitGroup
	for chunkIndex, missingPieces := range pieceMap {
		hosts := hdb.UniqueHosts(len(missingPieces), nil)
		if len(hosts) == 0 {
			// TODO: consider repairing one chunk at a time
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
		// encrypt pieces
		for i := range pieces {
			key := deriveKey(f.masterKey, chunkIndex, uint64(i))
			pieces[i], err = key.EncryptBytes(pieces[i])
			if err != nil {
				return err
			}
		}

		// upload one piece per host
		wg.Add(len(hosts))
		for i, host := range hosts {
			go func(host hostdb.Uploader, pieceIndex uint64, piece []byte) {
				offset, err := host.Upload(piece)
				if err != nil {
					return
				}

				// create contract entry, if necessary
				f.mu.Lock()
				defer f.mu.Unlock()
				contract, ok := f.contracts[host.ContractID()]
				if !ok {
					contract = fileContract{
						ID:          host.ContractID(),
						IP:          host.Address(),
						WindowStart: host.EndHeight(),
					}
				}

				// update contract
				contract.Pieces = append(contract.Pieces, pieceData{
					Chunk:  chunkIndex,
					Piece:  pieceIndex,
					Offset: offset,
				})
				f.contracts[host.ContractID()] = contract

				wg.Done()
			}(host, uint64(i), pieces[missingPieces[i]])
		}
		wg.Wait()
	}

	return nil
}

// threadedRepairUploads improves the health of files tracked by the renter by
// reuploading their missing pieces. Multiple repair attempts may be necessary
// before the file reaches full redundancy.
func (r *Renter) threadedRepairUploads() {
	for {
		time.Sleep(5 * time.Second)

		if !r.wallet.Unlocked() {
			continue
		}

		// make copy of repair set under lock
		repairing := make(map[string]trackedFile)
		id := r.mu.RLock()
		for name, meta := range r.tracking {
			repairing[name] = meta
		}
		r.mu.RUnlock(id)

		for name, meta := range repairing {
			// retrieve file object and get current height
			id = r.mu.RLock()
			f, ok := r.files[name]
			r.mu.RUnlock(id)
			if !ok {
				r.log.Printf("failed to repair %v: no longer tracking that file", name)
				id = r.mu.Lock()
				delete(r.tracking, name)
				r.mu.Unlock(id)
				continue
			}

			// check for expiration
			height := r.cs.Height()
			if meta.EndHeight != 0 && meta.EndHeight < height {
				r.log.Printf("removing %v from repair set: storage period has ended", name)
				id = r.mu.Lock()
				delete(r.tracking, name)
				r.mu.Unlock(id)
				continue
			}

			// check for un-uploaded pieces
			badChunks := f.incompleteChunks()
			if len(badChunks) == 0 {
				// check for expiring contracts
				if meta.EndHeight == 0 {
					// if auto-renewing, mark any chunks expiring soon
					badChunks = f.chunksBelow(height + renewThreshold)
				} else {
					// otherwise mark any chunks expiring before desired end
					badChunks = f.chunksBelow(meta.EndHeight)
				}
				if len(badChunks) == 0 {
					// check for offline hosts (slow)
					// TODO: reenable
					//badChunks = f.offlineChunks()
					continue
				}
			}

			r.log.Printf("repairing %v chunks of %v", len(badChunks), name)

			// open file handle
			handle, err := os.Open(meta.RepairPath)
			if err != nil {
				r.log.Printf("could not open %v for repair: %v", name, err)
				id = r.mu.Lock()
				delete(r.tracking, name)
				r.mu.Unlock(id)
				continue
			}

			err = f.repair(handle, badChunks, r.hostDB)
			handle.Close()
			if err != nil {
				// not fatal
				r.log.Printf("error while repairing %v: %v", name, err)
				continue
			}

			// save the repaired file data
			err = r.saveFile(f)
			if err != nil {
				// definitely bad, but we probably shouldn't delete from the
				// repair set if this happens
				r.log.Printf("failed to save repaired file %v: %v", name, err)
			}
		}
	}
}
