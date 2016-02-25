package renter

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// repairThreads is the number of repairs that can run concurrently.
	repairThreads = 10
)

// When a file contract is within 'renewThreshold' blocks of expiring, the renter
// will attempt to renew the contract.
var renewThreshold = func() types.BlockHeight {
	switch build.Release {
	case "testing":
		return 10
	case "dev":
		return 200
	default:
		return 144 * 7 * 3 // 3 weeks - to soon be 6 weeks.
	}
}()

// repair attempts to repair a file chunk by uploading its pieces to more
// hosts.
func (f *file) repair(chunkIndex uint64, missingPieces []uint64, r io.ReaderAt, hosts []contractor.Uploader) error {
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
	numPieces := len(missingPieces)
	if len(hosts) < numPieces {
		numPieces = len(hosts)
	}
	var wg sync.WaitGroup
	wg.Add(numPieces)
	for i := 0; i < numPieces; i++ {
		go func(pieceIndex uint64, host contractor.Uploader) {
			defer wg.Done()
			// upload data to host
			offset, err := host.Upload(pieces[pieceIndex])
			if err != nil {
				return
			}

			// create contract entry, if necessary
			f.mu.Lock()
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
			f.mu.Unlock()
		}(missingPieces[i], hosts[i])
	}
	wg.Wait()

	return nil
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

// expiringContracts returns the contracts that will expire soon.
// TODO: what if contract has fully expired?
func (f *file) expiringContracts(height types.BlockHeight) []fileContract {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var expiring []fileContract
	for _, fc := range f.contracts {
		if height >= fc.WindowStart-renewThreshold {
			expiring = append(expiring, fc)
		}
	}
	return expiring
}

// offlineChunks returns the chunks belonging to "offline" hosts -- hosts that
// do not meet uptime requirements. Importantly, only chunks missing more than
// half their redundancy are returned.
func (f *file) offlineChunks(hdb hostDB) map[uint64][]uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// mark all pieces belonging to offline hosts.
	offline := make(map[uint64][]uint64)
	for _, fc := range f.contracts {
		if hdb.IsOffline(fc.IP) {
			for _, p := range fc.Pieces {
				offline[p.Chunk] = append(offline[p.Chunk], p.Piece)
			}
		}
	}
	// filter out chunks missing less than half of their redundancy
	filtered := make(map[uint64][]uint64)
	for chunk, pieces := range offline {
		if len(pieces) > f.erasureCode.NumPieces()/2 {
			filtered[chunk] = pieces
		}
	}
	return filtered
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

		if len(r.hostContractor.Contracts()) == 0 {
			// nothing to revise
			continue
		}

		// make copy of repair set under lock
		repairing := make(map[string]trackedFile)
		id := r.mu.RLock()
		for name, meta := range r.tracking {
			repairing[name] = meta
		}
		r.mu.RUnlock(id)

		// create host pool
		pool := r.newHostPool()
		for name, meta := range repairing {
			r.threadedRepairFile(name, meta, pool)
		}
		pool.Close() // heh
	}
}

// threadedRepairFile repairs and saves an individual file.
func (r *Renter) threadedRepairFile(name string, meta trackedFile, pool *hostPool) {
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
	if !meta.Renew && meta.EndHeight < height {
		logAndRemove("removing %v from repair set: storage period has ended", name)
		return
	}

	// determine if there is any work to do
	incChunks := f.incompleteChunks()
	if len(incChunks) == 0 {
		return
	}

	// open file handle
	handle, err := os.Open(meta.RepairPath)
	if err != nil {
		logAndRemove("removing %v from repair set: %v", name, err)
		return
	}
	defer handle.Close()

	// repair incomplete chunks
	if len(incChunks) != 0 {
		r.log.Printf("repairing %v chunks of %v", len(incChunks), f.name)
		r.repairChunks(f, handle, incChunks, pool)
	}
}

// repairChunks uploads missing chunks of f to new hosts.
func (r *Renter) repairChunks(f *file, handle io.ReaderAt, chunks map[uint64][]uint64, pool *hostPool) {
	for chunk, pieces := range chunks {
		// Determine host set. We want one host for each missing piece, and no
		// repeats of other hosts of this chunk.
		hosts := pool.uniqueHosts(len(pieces), f.chunkHosts(chunk))
		if len(hosts) == 0 {
			r.log.Printf("aborting repair of %v: not enough hosts", f.name)
			return
		}
		// upload to new hosts
		err := f.repair(chunk, pieces, handle, hosts)
		if err != nil {
			r.log.Printf("aborting repair of %v: %v", f.name, err)
			return
		}

		// save the new contract
		f.mu.RLock()
		err = r.saveFile(f)
		f.mu.RUnlock()
		if err != nil {
			// If saving failed for this chunk, it will probably fail for the
			// next chunk as well. Better to try again on the next cycle.
			r.log.Printf("failed to save repaired file %v: %v", f.name, err)
			return
		}
	}
}
