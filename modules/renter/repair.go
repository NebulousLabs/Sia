package renter

import (
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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

// removeExpiredContracts deletes contracts in the file object that have
// expired.
func (f *file) removeExpiredContracts(currentHeight types.BlockHeight) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var expired []types.FileContractID
	for id, fc := range f.contracts {
		if currentHeight >= fc.WindowStart {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		delete(f.contracts, id)
	}
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

// offlineChunks returns a map of chunks whose pieces are not
// immediately available for download.
func (f *file) offlineChunks() map[uint64][]uint64 {
	offline := make(map[uint64][]uint64)
	var mapLock sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(f.contracts))
	for _, fc := range f.contracts {
		go func(fc fileContract) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", string(fc.IP), hostTimeout)
			if err == nil {
				conn.Close()
				return
			}
			// host did not respond in time; mark all pieces as offline
			mapLock.Lock()
			for _, p := range fc.Pieces {
				offline[p.Chunk] = append(offline[p.Chunk], p.Piece)
			}
			mapLock.Unlock()
		}(fc)
	}
	wg.Wait()
	return offline
}

// repair attempts to repair a file by uploading missing pieces to more hosts.
func (f *file) repair(r io.ReaderAt, pieceMap map[uint64][]uint64, hosts []uploader) error {
	// For each chunk with missing pieces, re-encode the chunk and upload each
	// missing piece.
	var wg sync.WaitGroup
	for chunkIndex, missingPieces := range pieceMap {
		// can only upload to hosts that aren't already storing this chunk
		// TODO: what if we're renewing?
		// 	curHosts := f.chunkHosts(chunkIndex)
		// 	var newHosts []uploader
		// outer:
		// 	for _, h := range hosts {
		// 		for _, ip := range curHosts {
		// 			if ip == h.addr() {

		// 				continue outer
		// 			}
		// 		}
		// 		newHosts = append(newHosts, h)
		// 	}
		newHosts := hosts
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
				err := host.addPiece(up)
				if err == nil {
					// update contract
					f.mu.Lock()
					contract := host.fileContract()
					f.contracts[contract.ID] = contract
					f.mu.Unlock()
				}
				wg.Done()
			}(host, up)
		}
		wg.Wait()
	}

	return nil
}

// threadedRepairUploads improves the health of files tracked by the renter by
// reuploading their missing pieces. Multiple repair attempts may be necessary
// before the file reaches full redundancy.
func (r *Renter) threadedRepairUploads() {
	// a primitive blacklist is used to augment the hostdb's weights. Each
	// negotiation failure increments the integer, and the probability of
	// selecting the host for upload is 1/n.
	blacklist := make(map[modules.NetAddress]int)

	for {
		time.Sleep(5 * time.Second)

		if !r.wallet.Unlocked() {
			continue
		}

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
			//height := r.blockHeight
			r.mu.RUnlock(id)
			if !ok {
				r.log.Printf("failed to repair %v: no longer tracking that file", name)
				id = r.mu.Lock()
				delete(r.repairSet, name)
				r.mu.Unlock(id)
				continue
			}

			// delete any expired contracts
			//f.removeExpiredContracts(height)

			// determine file health
			badChunks := f.incompleteChunks()
			if len(badChunks) == 0 {
				//badChunks = f.expiringChunks(height)
				// if len(badChunks) == 0 {
				// 	// nothing to do
				// 	continue
				// }
				continue
			}

			r.log.Printf("repairing %v chunks of %v", len(badChunks), name)

			// defer is really convenient for cleaning up resources, so an
			// inline function is justified
			err := func() error {
				// open file handle
				handle, err := os.Open(path)
				if err != nil {
					return err
				}
				defer handle.Close()

				// build host list
				bytesPerHost := f.pieceSize * uint64(f.erasureCode.MinPieces()) * f.numChunks()
				var hosts []uploader
				randHosts := r.hostDB.RandomHosts(f.erasureCode.NumPieces() * 2)
				for _, h := range randHosts {
					// probabilistically filter out known bad hosts
					// unresponsive hosts will be selected with probability 1/(1+nFailures)
					nFailures, ok := blacklist[h.IPAddress]
					if n, _ := crypto.RandIntn(1 + nFailures); ok && n != 0 {
						continue
					}

					// TODO: use smarter duration
					hostUploader, err := r.newHostUploader(h, bytesPerHost, defaultDuration, f.masterKey)
					if err != nil {
						// penalize unresponsive hosts
						if strings.Contains(err.Error(), "timeout") {
							blacklist[h.IPAddress]++
						}
						continue
					}
					defer hostUploader.Close()

					hosts = append(hosts, hostUploader)
					if len(hosts) >= f.erasureCode.NumPieces() {
						break
					}
				}

				if len(hosts) < f.erasureCode.MinPieces() {
					// don't return an error in this case, since the file
					// should not be removed from the repair set
					r.log.Printf("failed to repair %v: not enough hosts", name)
					return nil
				}

				return f.repair(handle, badChunks, hosts)
			}()

			if err != nil {
				r.log.Printf("%v cannot be repaired: %v", name, err)
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
		}
	}
}
