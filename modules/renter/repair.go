package renter

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb"
	"github.com/NebulousLabs/Sia/types"
)

const (
	hostTimeout = 15 * time.Second
)

// When a file contract is within 'renewThreshold' blocks of expiring, the renter
// will attempt to renew the contract.
var renewThreshold = func() types.BlockHeight {
	switch build.Release {
	case "testing":
		return 20
	case "dev":
		return 200
	default:
		return 2000
	}
}()

// repair attempts to repair a file chunk by uploading its pieces to more
// hosts.
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
	numPieces := len(missingPieces)
	if len(hosts) < numPieces {
		numPieces = len(hosts)
	}
	var wg sync.WaitGroup
	wg.Add(numPieces)
	for i := 0; i < numPieces; i++ {
		// each goroutine gets a different host, index, and piece, so there
		// are no data race concerns
		pIndex := missingPieces[i]
		go func(host hostdb.Uploader, pieceIndex uint64, piece []byte) {
			defer wg.Done()

			// upload data to host
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
		}(hosts[i], pIndex, pieces[pIndex])
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

	// helper function for determining if a host is offline. A host is
	// considered offline if it is in AllHosts but not ActiveHosts. For good
	// order notation, we covert AllHosts and ActiveHosts to a single map. The
	// map lookup will return true if the host was in ActiveHosts.
	//
	// TODO: once the hostdb is persistent, checking for existence in
	// ActiveHosts should be sufficient.
	hostSet := make(map[modules.NetAddress]bool)
	for _, host := range hdb.AllHosts() {
		hostSet[host.IPAddress] = false
	}
	for _, host := range hdb.ActiveHosts() {
		hostSet[host.IPAddress] = true
	}
	isOffline := func(addr modules.NetAddress) bool {
		active, exists := hostSet[addr]
		return exists && !active
	}

	// mark all pieces belonging to offline hosts.
	offline := make(map[uint64][]uint64)
	for _, fc := range f.contracts {
		if isOffline(fc.IP) {
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
	if !meta.Renew && meta.EndHeight < height {
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

	// repair incomplete chunks
	if badChunks := f.incompleteChunks(); len(badChunks) != 0 {
		r.log.Printf("repairing %v chunks of %v", len(badChunks), f.name)
		var duration types.BlockHeight
		if meta.Renew {
			duration = defaultDuration
		} else {
			duration = meta.EndHeight - height
		}
		r.repairChunks(f, handle, badChunks, duration)
	}

	// repair offline chunks
	if badChunks := f.offlineChunks(r.hostDB); len(badChunks) != 0 {
		r.log.Printf("reuploading %v offline chunks of %v", len(badChunks), f.name)
		var duration types.BlockHeight
		if meta.Renew {
			duration = defaultDuration
		} else {
			duration = meta.EndHeight - height
		}
		r.repairChunks(f, handle, badChunks, duration)
	}

	// renew expiring contracts
	if meta.Renew {
		if badContracts := f.expiringContracts(height); len(badContracts) != 0 {
			r.log.Printf("renewing %v contracts of %v", len(badContracts), f.name)
			newHeight := height + defaultDuration
			r.renewContracts(f, badContracts, newHeight)
		}
	}

	// save the repaired file data
	f.mu.RLock()
	err = r.saveFile(f)
	f.mu.RUnlock()
	if err != nil {
		// definitely bad, but we probably shouldn't delete from the
		// repair set if this happens
		r.log.Printf("failed to save repaired file %v: %v", name, err)
	}
}

// repairChunks uploads missing chunks of f to new hosts.
func (r *Renter) repairChunks(f *file, handle io.ReaderAt, chunks map[uint64][]uint64, duration types.BlockHeight) {
	// create host pool
	contractSize := (f.pieceSize + crypto.TwofishOverhead) * f.numChunks() // each host gets one piece of each chunk
	pool, err := r.hostDB.NewPool(contractSize, duration)
	if err != nil {
		r.log.Printf("failed to repair %v: %v", f.name, err)
		return
	}
	defer pool.Close() // heh

	for chunk, pieces := range chunks {
		// determine host set
		old := f.chunkHosts(chunk)
		hosts := pool.UniqueHosts(f.erasureCode.NumPieces()-len(old), old)
		if len(hosts) == 0 {
			r.log.Printf("aborting repair of %v: not enough hosts", f.name)
			return
		}
		// upload to new hosts
		err = f.repair(chunk, pieces, handle, hosts)
		if err != nil {
			r.log.Printf("aborting repair of %v: %v", f.name, err)
			return
		}
	}
}

// renewContracts renews each of the supplied contracts, replacing their entry
// in f with the new contract.
func (r *Renter) renewContracts(f *file, contracts []fileContract, newHeight types.BlockHeight) {
	for _, c := range contracts {
		newID, err := r.hostDB.Renew(c.ID, newHeight)
		if err != nil {
			r.log.Printf("failed to renew contract %v: %v", c.ID, err)
			continue
		}
		f.mu.Lock()
		f.contracts[newID] = fileContract{
			ID:          newID,
			IP:          c.IP,
			Pieces:      c.Pieces,
			WindowStart: newHeight,
		}
		// need to delete the old contract; otherwise f.expiringContracts
		// will continue to return it
		delete(f.contracts, c.ID)
		f.mu.Unlock()
	}
}
