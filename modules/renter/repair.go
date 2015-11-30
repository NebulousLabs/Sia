package renter

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
)

const (
	// When a file contract is within this many blocks of expiring, the renter
	// will attempt to reupload the data covered by the contract.
	renewThreshold = 2000

	hostTimeout = 15 * time.Second
)

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

// incompleteChunks returns a map of chunks containing pieces that have not
// been uploaded.
func (f *file) incompleteChunks() map[uint64][]uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

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

// chunkHosts returns the hosts storing the given chunk.
func (f *file) chunkHosts(chunk uint64) []modules.NetAddress {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var old []modules.NetAddress
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			if p.Chunk == chunk {
				old = append(old, fc.IP)
				break
			}
		}
	}
	return old
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

	// check for un-uploaded pieces
	badChunks := f.incompleteChunks()
	if len(badChunks) == 0 {
		return
	}

	r.log.Printf("repairing %v chunks of %v", len(badChunks), name)

	// create host pool
	pool, err := r.hostDB.NewPool()
	if err != nil {
		r.log.Printf("failed to repair %v: %v", name, err)
		return
	}
	defer pool.Close() // heh

	for chunk, pieces := range badChunks {
		// determine host set
		old := f.chunkHosts(chunk)
		hosts := pool.UniqueHosts(f.erasureCode.NumPieces()-len(old), old)
		// upload to new hosts
		err = f.repair(chunk, pieces, handle, hosts)
		if err != nil {
			r.log.Printf("aborting repair of %v: %v", name, err)
			break
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
