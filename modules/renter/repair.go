package renter

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
	"github.com/NebulousLabs/Sia/types"
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
		return 144 * 7 * 6
	}
}()

// hostErr and hostErrs are helpers for reporting repair errors. The actual
// Error implementations aren't that important; we just need to be able to
// extract the NetAddress of the failed host.

type hostErr struct {
	host modules.NetAddress
	err  error
}

func (he hostErr) Error() string {
	return fmt.Sprintf("host %v failed: %v", he.host, he.err)
}

type hostErrs []*hostErr

func (hs hostErrs) Error() string {
	var errs []error
	for _, h := range hs {
		errs = append(errs, h)
	}
	return build.JoinErrors(errs, "\n").Error()
}

// repair attempts to repair a file chunk by uploading its pieces to more
// hosts.
func (f *file) repair(chunkIndex uint64, missingPieces []uint64, r io.ReaderAt, hosts []contractor.Editor) error {
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
	errChan := make(chan *hostErr)
	for i := 0; i < numPieces; i++ {
		go func(pieceIndex uint64, host contractor.Editor) {
			// upload data to host
			root, err := host.Upload(pieces[pieceIndex])
			if err != nil {
				errChan <- &hostErr{host.Address(), err}
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
				Chunk:      chunkIndex,
				Piece:      pieceIndex,
				MerkleRoot: root,
			})
			f.contracts[host.ContractID()] = contract
			f.mu.Unlock()
			errChan <- nil
		}(missingPieces[i], hosts[i])
	}
	var errs hostErrs
	for i := 0; i < numPieces; i++ {
		err := <-errChan
		if err != nil {
			errs = append(errs, err)
		}
	}
	if errs != nil {
		return errs
	}

	return nil
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

// repairChunks uploads missing chunks of f to new hosts.
func (r *Renter) repairChunks(f *file, handle io.ReaderAt, chunks map[uint64][]uint64, pool *hostPool) {
	for chunk, pieces := range chunks {
		// Determine host set. We want one host for each missing piece, and no
		// repeats of other hosts of this chunk.
		hosts := pool.uniqueHosts(len(pieces), f.chunkHosts(chunk))
		if len(hosts) == 0 {
			r.log.Debugf("aborting repair of %v: host pool is empty", f.name)
			return
		}
		// upload to new hosts
		err := f.repair(chunk, pieces, handle, hosts)
		if err != nil {
			if he, ok := err.(hostErrs); ok {
				// if a specific host failed, remove it from the pool
				for _, h := range he {
					// only log non-graceful errors
					if h.err != modules.ErrStopResponse {
						r.log.Printf("failed to upload to host %v: %v", h.host, h.err)
					}
					pool.remove(h.host)
				}
			} else {
				// any other type of error indicates a serious problem
				r.log.Printf("aborting repair of %v: %v", f.name, err)
				return
			}
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

		// check for download interruption
		id := r.mu.RLock()
		downloading := r.downloading
		r.mu.RUnlock(id)
		if downloading {
			return
		}
	}
}

type chunkGaps struct {
	contracts []types.FileContractID
	pieces    []uint64
	numGaps   int
}

func (r *Renter) addFileToRepairMatrix(file *file, availableWorkers map[types.FileContractID]struct{}, repairMatrix map[string]*chunkGaps, gapCounts map[int]int) {
	// Flatten availableWorkers into a list of contracts.
	contracts := make([]types.FileContractID, 0)
	for contract, _ := range availableWorkers {
		contracts = append(contracts, contract)
	}

	// If the file is not being tracked, don't add it to the repair matrix.
	_, exists := r.tracking[file.name]
	if exists {
		return
	}

	// For each chunk, create a map from the chunk to the pieces that it
	// has, and to the contracts that have that chunk.
	chunkCount := file.numChunks()
	contractMap := make(map[uint64][]types.FileContractID)
	pieceMap := make(map[uint64][]uint64)
	for i := uint64(0); i < chunkCount; i++ {
		contractMap[i] = make([]types.FileContractID, 0)
		pieceMap[i] = make([]uint64, 0)
	}

	// Iterate through each contract and figure out what's available.
	for _, contract := range file.contracts {
		for _, piece := range contract.Pieces {
			contractMap[piece.Chunk] = append(contractMap[piece.Chunk], contract.ID)
			pieceMap[piece.Chunk] = append(pieceMap[piece.Chunk], piece.Piece)
		}
	}

	// Iterate through each chunk and, if there are gaps, add the inverse
	// to the repair matrix.
	for i := uint64(0); i < chunkCount; i++ {
		if len(pieceMap[i]) < file.erasureCode.NumPieces() {
			// Find the gaps in the pieces and contracts.
			potentialPieceGaps := make(map[uint64]struct{})
			for j := 0; j < file.erasureCode.NumPieces(); i++ {
				potentialPieceGaps[i] = struct{}{}
			}
			potentialContractGaps := make(map[types.FileContractID]struct{})
			for _, contract := range contracts {
				potentialContractGaps[contract] = struct{}{}
			}

			// Delete every available piece from the potential piece gaps,
			// and every utilized contract form the potential contract
			// gaps.
			for _, fcid := range contractMap[i] {
				delete(potentialContractGaps, fcid)
			}
			for _, piece := range pieceMap[i] {
				delete(potentialPieceGaps, piece)
			}

			// Merge the gaps into a slice.
			var gaps chunkGaps
			for fcid, _ := range potentialContractGaps {
				gaps.contracts = append(gaps.contracts, fcid)
			}
			for piece, _ := range potentialPieceGaps {
				gaps.pieces = append(gaps.pieces, piece)
			}

			// Figure out the largest number of workers that could be
			// repairing this piece simultaneously.
			if len(potentialPieceGaps) < len(potentialContractGaps) {
				gaps.numGaps = len(potentialPieceGaps)
			} else {
				gaps.numGaps = len(potentialContractGaps)
			}

			// Record the number of gaps that this chunk has, which makes
			// blocking-related decisions easier.
			gapCounts[gaps.numGaps] = gapCounts[gaps.numGaps] + 1

			// Create a name for the incomplete chunk.
			chunkPrefix := make([]byte, 8)
			binary.LittleEndian.PutUint64(chunkPrefix, i)
			chunkName := string(chunkPrefix)+file.name

			// Add the chunk to the repair matrix.
			repairMatrix[chunkName] = &gaps
		}
	}
}

func (r *Renter) createRepairMatrix(availableWorkers map[types.FileContractID]struct{}) (map[string]*chunkGaps, map[int]int) {
	repairMatrix := make(map[string]*chunkGaps)
	gapCounts := make(map[int]int)

	// Add all of the files to the repair matrix.
	for _, file := range r.files {
		r.addFileToRepairMatrix(file, availableWorkers, repairMatrix, gapCounts)
	}
	return repairMatrix, gapCounts
}

// threadedRepairLoop improves the health of files tracked by the renter by
// reuploading their missing pieces. Multiple repair attempts may be necessary
// before the file reaches full redundancy.
func (r *Renter) threadedRepairLoop() {
	for {
		// Create the initial set of workers that are used to perform
		// uploading.
		availableWorkers := make(map[types.FileContractID]struct{})
		id := r.mu.RLock()
		for id, worker := range r.workerPool {
			// Ignore workers that have had an upload failure in the past two
			// hours.
			if worker.recentUploadFailure.Add(time.Minute * 120).Before(time.Now()) {
				availableWorkers[id] = struct{}{}
			}
		}
		r.mu.RUnlock(id)

		// Create the repair matrix. The repair matrix is a set of chunks,
		// linked from chunk id to the set of hosts that do not have that
		// chunk.
		repairMatrix, gapCounts := r.createRepairMatrix(availableWorkers)
		maxGaps := 0
		for i := 1; i < len(gapCounts); i++ {
			if gapCounts[i] > 0 {
				maxGaps = i
			}
		}
		if maxGaps == 0 {
			// There is no work to do. Sleep for 15 minutes, or until there has
			// been a new upload. Then iterate to make a new repair matrix and
			// check again.
			select{
				case <-time.After(time.Minute * 15):
					continue
				case <-r.newFiles:
					continue
			}
		}

		// Set up a loop that first waits for enough workers to become
		// available, and then iterates through the repair matrix to find a
		// chunk to repair. The loop will create a chunk if as few as 4 pieces
		// can be handed off to workers simultaneously.
		startTime := time.Now()
		activeWorkers := availableWorkers
		var retiredWorkers []types.FileContractID
		resultChan := make(chan finishedUpload)
		for {
			// Determine the maximum number of gaps that any chunk has.
			maxGaps := 0
			for i := 1; i < len(gapCounts); i++ {
				if gapCounts[i] != 0 {
					maxGaps = i
				}
			}
			if maxGaps == 0 {
				// None of the chunks have any more opportunity to upload.
				break
			}

			// Iterate through the chunks until a candidate chunk is found.
			for chunkID, chunkGaps := range repairMatrix {
				// Figure out how many pieces in this chunk could be repaired
				// by the current availableWorkers.
				var usefulWorkers []types.FileContractID
				for worker, _ := range availableWorkers {
					for _, contract := range chunkGaps.contracts {
						if worker == contract {
							usefulWorkers = append(usefulWorkers, worker)
						}
					}
				}

				if maxGaps >= 4 && len(usefulWorkers) < 4 {
					// These workers in particular are not useful for this
					// chunk - need a different or broader set of workers.
					// Update the gapCount for this chunk - retired workers may
					// have altered the number.

					// Remove the contract ids of any workers that have
					// retired.
					for _, retire := range retiredWorkers {
						for i := range chunkGaps.contracts {
							if chunkGaps.contracts[i] == retire {
								chunkGaps.contracts = append(chunkGaps.contracts[:i], chunkGaps.contracts[i+1:]...)
								break
							}
						}
					}
					// Update the gap counts if they have been affected in any
					// way.
					if len(chunkGaps.contracts) < len(chunkGaps.pieces) && len(chunkGaps.contracts) < chunkGaps.numGaps {
						oldNumGaps := chunkGaps.numGaps
						chunkGaps.numGaps = len(chunkGaps.contracts)
						gapCounts[oldNumGaps] = gapCounts[oldNumGaps] - 1
						gapCounts[chunkGaps.numGaps] = gapCounts[chunkGaps.numGaps] + 1
					}
					continue
				}

				// Parse the filename and chunk index from the repair
				// matrix key.
				chunkIndexBytes := chunkID[:8]
				filename := chunkID[8:]
				chunkIndex := binary.LittleEndian.Uint64([]byte(chunkIndexBytes))
				file, exists := r.files[filename]
				if !exists {
					// TODO: Should pull this chunk out of the repair
					// matrix. The other errors in this block should do the
					// same.
					continue
				}

				// Grab the chunk and code it into its separate pieces.
				meta, exists := r.tracking[filename]
				if !exists {
					continue
				}
				fHandle, err := os.Open(meta.RepairPath)
				if err != nil {
					// TODO: Perform a download-and-repair. Though, this
					// may block other uploads that are in progress. Not
					// sure how to do this cleanly in the background?
					//
					// TODO: Manage err
					continue
				}
				chunk := make([]byte, file.chunkSize())
				_, err = fHandle.ReadAt(chunk, int64(chunkIndex*file.chunkSize()))
				if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
					// TODO: Manage this error.
					continue
				}
				pieces, err := file.erasureCode.Encode(chunk)
				if err != nil {
					// TODO: Manage this error.
					continue
				}
				// encrypt pieces
				for i := range pieces {
					key := deriveKey(file.masterKey, chunkIndex, uint64(i))
					pieces[i], err = key.EncryptBytes(pieces[i])
					if err != nil {
						// TODO: Manage this error.
						continue
					}
				}

				// Give each piece to a worker, updating the chunkGaps and
				// availableWorkers along the way.
				var i int
				for i = 0; i < len(usefulWorkers) && i < len(chunkGaps.pieces); i++ {
					uw := uploadWork{
						chunkIndex: chunkGaps.pieces[i],
						data: pieces[chunkGaps.pieces[i]],
						file: file,
						pieceIndex: chunkGaps.pieces[i],

						resultChan: resultChan,
					}
					worker := r.workerPool[usefulWorkers[i]]
					worker.uploadChan <- uw

					delete(availableWorkers, usefulWorkers[i])
					for j := 0; j < len(chunkGaps.contracts); j++ {
						if chunkGaps.contracts[j] == usefulWorkers[i] {
							chunkGaps.contracts = append(chunkGaps.contracts[:j], chunkGaps.contracts[j+1:]...)
							break
						}
					}
				}
				chunkGaps.pieces = chunkGaps.pieces[i:]

				// Update the number of gaps.
				oldNumGaps := chunkGaps.numGaps
				if len(chunkGaps.contracts) < len(chunkGaps.pieces) {
					chunkGaps.numGaps = len(chunkGaps.contracts)
				} else {
					chunkGaps.numGaps = len(chunkGaps.pieces)
				}
				gapCounts[oldNumGaps] = gapCounts[oldNumGaps] - 1
				gapCounts[chunkGaps.numGaps] = gapCounts[chunkGaps.numGaps] + 1
				break
			}

			// Determine the number of workers we need in 'available'.
			exclude := maxGaps - 4
			if exclude < 0 {
				exclude = 0
			}
			need := len(activeWorkers) - exclude
			if need <= len(availableWorkers) {
				need = len(availableWorkers)+1
			}
			if need > len(activeWorkers) {
				need = len(activeWorkers)
			}
			newMatrix := false
			if time.Since(startTime) > time.Hour {
				newMatrix = true
				need = len(activeWorkers)
			}

			// Wait until 'need' workers are available.
			for len(availableWorkers) < need {
				finishedUpload := <-resultChan

				if finishedUpload.err != nil {
					r.log.Debugln("Error while performing upload to", finishedUpload.workerID, "::", finishedUpload.err)
					id := r.mu.RLock()
					worker, exists := r.workerPool[finishedUpload.workerID]
					if exists {
						worker.recentUploadFailure = time.Now()
						retiredWorkers = append(retiredWorkers, finishedUpload.workerID)
						delete(activeWorkers, finishedUpload.workerID)
						need--
					}
					r.mu.Unlock(id)
					continue
				}

				// Mark that the worker is available again.
				availableWorkers[finishedUpload.workerID] = struct{}{}
			}

			// Grab a new repair matrix if we've been using this repair matrix
			// for more than an hour.
			if newMatrix {
				break
			}

			// Receive all of the new files and add them to the repair matrix
			// before continuing.
			for {
				select {
					case file := <-r.newFiles:
						r.addFileToRepairMatrix(file, activeWorkers, repairMatrix, gapCounts)
					default:
						break
				}
			}
		}
	}
}
