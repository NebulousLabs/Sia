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
func incompleteChunks(chunks [][]*types.FileContractID) repairMap {
	incomplete := make(repairMap)
	for chunkIndex, pieces := range chunks {
		for pieceIndex, id := range pieces {
			if id == nil {
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

// repair attempts to repair a chunk of f by uploading its pieces to more hosts.
func (f *file) repair(chunkIndex uint64, missingPieces []uint64, r io.ReaderAt, hosts []hostdb.Uploader) error {
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
	var wg sync.WaitGroup
	wg.Add(len(missingPieces))
	for i, pieceIndex := range missingPieces {
		go func(host hostdb.Uploader, pieceIndex uint64, piece []byte) {
			defer wg.Done()
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
		}(hosts[i%len(hosts)], uint64(i), pieces[pieceIndex])
	}
	wg.Wait()

	return nil
}

func (f *file) chunkFormat() [][]*types.FileContractID {
	ids := make([][]*types.FileContractID, f.numChunks())
	for i := range ids {
		ids[i] = make([]*types.FileContractID, f.erasureCode.NumPieces())
	}
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			ids[p.Chunk][p.Piece] = &fc.ID
		}
	}
	return ids
}

// threadedRepairLoop improves the health of files tracked by the renter by
// reuploading their missing pieces. Multiple repair attempts may be necessary
// before the file reaches full redundancy.
func (r *Renter) threadedRepairLoop() {
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
			r.threadedRepairFile(name, meta)
		}
	}
}

// threadedRepairFile repairs and saves an individual file.
func (r *Renter) threadedRepairFile(name string, meta trackedFile) {
	// helper function
	logAndRemove := func(fmt string, args ...interface{}) {
		r.log.Printf(fmt, args...)
		id := r.mu.Lock()
		delete(r.tracking, name)
		r.mu.Unlock(id)
	}

	id := r.mu.RLock()
	f, ok := r.files[name]
	r.mu.RUnlock(id)
	if !ok {
		logAndRemove("removing %v from repair set: no longer tracking that file", name)
		return
	}

	// check for expiration
	height := r.cs.Height()
	if meta.EndHeight != 0 && meta.EndHeight < height {
		logAndRemove("removing %v from repair set: storage period has ended", name)
		return
	}

	// open file handle
	handle, err := os.Open(meta.RepairPath)
	if err != nil {
		logAndRemove("removing %v from repair set: %v", name, err)
		return
	}
	defer handle.Close()

	// convert memory layout
	chunkIDs := f.chunkFormat()

	// check for un-uploaded pieces
	badChunks := incompleteChunks(chunkIDs)
	if len(badChunks) == 0 {
		return
		/*
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
				badChunks = f.offlineChunks()
			}
		*/
	}

	r.log.Printf("repairing %v chunks of %v", len(badChunks), name)

	// create host set
	hosts := r.hostDB.UniqueHosts(f.erasureCode.NumPieces(), nil)
	if len(hosts) == 0 {
		r.log.Printf("failed to repair %v: not enough hosts", name)
		return
	}
	for _, h := range hosts {
		defer h.Close()
	}

	for chunk, pieces := range badChunks {
		err = f.repair(chunk, pieces, handle, hosts)
		if err != nil {
			// not fatal
			r.log.Printf("error while repairing chunk %v of %v: %v", chunk, name, err)
		}
	}

	// save the repaired file data
	err = r.saveFile(f)
	if err != nil {
		// definitely bad, but we probably shouldn't delete from the
		// repair set if this happens
		r.log.Printf("failed to save repaired file %v: %v", name, err)
	}
}
