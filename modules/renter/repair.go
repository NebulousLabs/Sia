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

// A repairMap is a mapping of chunks to pieces that need repair.
type repairMap map[uint64][]uint64

// subtract deletes the values in r that also appear in m, and returns r.
func (r repairMap) subtract(m repairMap) repairMap {
	for chunk, mPieces := range m {
		if _, ok := r[chunk]; !ok {
			continue
		}
		for _, mPieceIndex := range mPieces {
			for i, rPieceIndex := range r[chunk] {
				if rPieceIndex == mPieceIndex {
					// remove slice element
					r[chunk] = append(r[chunk][:i], r[chunk][i+1:]...)
					break
				}
			}
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
func (f *file) repair(r io.ReaderAt, pieceMap repairMap, hosts []uploader) error {
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
			newHosts = hosts
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
			height := r.blockHeight
			r.mu.RUnlock(id)
			if !ok {
				r.log.Printf("failed to repair %v: no longer tracking that file", name)
				id = r.mu.Lock()
				delete(r.tracking, name)
				r.mu.Unlock(id)
				continue
			}

			// calculate duration
			var duration types.BlockHeight
			if meta.EndHeight == 0 {
				duration = defaultDuration
			} else if meta.EndHeight > height {
				duration = meta.EndHeight - height
			} else {
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
					badChunks = f.offlineChunks()
					if len(badChunks) == 0 {
						// nothing to do
						continue
					}
				}
			}

			r.log.Printf("repairing %v chunks of %v", len(badChunks), name)

			// defer is really convenient for cleaning up resources, so an
			// inline function is justified
			err := func() error {
				// open file handle
				handle, err := os.Open(meta.RepairPath)
				if err != nil {
					return err
				}
				defer handle.Close()

				// build host list
				bytesPerHost := f.pieceSize * f.numChunks() * 2 // 2x buffer to prevent running out of money
				var hosts []uploader
				randHosts := r.hostDB.RandomHosts(f.erasureCode.NumPieces() * 2)
				for _, h := range randHosts {
					// probabilistically filter out known bad hosts
					// unresponsive hosts will be selected with probability 1/(1+nFailures)
					nFailures, ok := blacklist[h.IPAddress]
					if n, _ := crypto.RandIntn(1 + nFailures); ok && n != 0 {
						continue
					}

					hostUploader, err := r.newHostUploader(h, bytesPerHost, duration, f.masterKey)
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
				delete(r.tracking, name)
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
