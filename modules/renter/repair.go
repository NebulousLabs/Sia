package renter

import (
	"io"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/types"
)

// TODO: Move to a consts file.
const uploadFailureCooldown = time.Hour * 6
const maxUploadLoopIdleTime = time.Minute * 30
const minPiecesRepair = 4

type (
	// chunkGaps points to all of the missing pieces in a chunk, as well as all
	// of the hosts that are missing. The 'numGaps' value is the minimum of
	// len(contracts) and len(pieces).
	chunkGaps struct {
		contracts []types.FileContractID
		pieces    []uint64
	}

	// chunkID can be used to uniquely identify a chunk within the repair
	// matrix.
	chunkID struct {
		chunkIndex uint64
		filename   string
	}
)

// numGaps returns the number of gaps that a chunk has.
func (cg *chunkGaps) numGaps() int {
	if len(cg.contracts) <= len(cg.pieces) {
		return len(cg.contracts)
	}
	return len(cg.pieces)
}

// addFileToRepairMatrix will take a file and add each of the incomplete chunks
// to the repair matrix, along with data about which pieces need attention.
func (r *Renter) addFileToRepairMatrix(file *file, availableWorkers map[types.FileContractID]struct{}, repairMatrix map[chunkID]*chunkGaps, gapCounts map[int]int) {
	// Flatten availableWorkers into a list of contracts.
	contracts := make([]types.FileContractID, 0)
	for contract := range availableWorkers {
		contracts = append(contracts, contract)
	}

	// For each chunk, create a map from the chunk to the pieces that it
	// has, and to the contracts that have that chunk.
	chunkCount := file.numChunks()
	availablePieces := make(map[uint64][]uint64, chunkCount)
	utilizedContracts := make(map[uint64][]types.FileContractID, chunkCount)
	for i := uint64(0); i < chunkCount; i++ {
		availablePieces[i] = make([]uint64, 0)
		utilizedContracts[i] = make([]types.FileContractID, 0)
	}

	// Iterate through each contract and figure out which pieces are available.
	for _, contract := range file.contracts {
		// Check whether this contract is offline.
		offline := r.hostContractor.IsOffline(contract.ID)

		for _, piece := range contract.Pieces {
			utilizedContracts[piece.Chunk] = append(utilizedContracts[piece.Chunk], contract.ID)

			// Only mark the piece as complete if the piece can be recovered.
			//
			// TODO: Add an 'unavailable' flag to the piece that gets set to
			// true if the host loses the piece, and only add the piece to the
			// 'availablePieces' set if !unavailable.
			if !offline {
				availablePieces[piece.Chunk] = append(availablePieces[piece.Chunk], piece.Piece)
			}
		}
	}

	// Iterate through each chunk and add the list of gaps to the repair
	// matrix. The chunk will not be added if there are no gaps.
	for i := uint64(0); i < chunkCount; i++ {
		if len(availablePieces[i]) < file.erasureCode.NumPieces() {
			// Find the gaps in the pieces and contracts.
			potentialPieceGaps := make([]bool, file.erasureCode.NumPieces())
			potentialContractGaps := make(map[types.FileContractID]struct{})
			for _, contract := range contracts {
				potentialContractGaps[contract] = struct{}{}
			}

			// Delete every available piece from the potential piece gaps, and
			// every utilized contract from the potential contract gaps.
			for _, piece := range availablePieces[i] {
				potentialPieceGaps[piece] = true
			}
			for _, fcid := range utilizedContracts[i] {
				delete(potentialContractGaps, fcid)
			}

			// Merge the gaps into a slice.
			var gaps chunkGaps
			for j, piece := range potentialPieceGaps {
				if !piece {
					gaps.pieces = append(gaps.pieces, uint64(j))
				}
			}
			for fcid := range potentialContractGaps {
				gaps.contracts = append(gaps.contracts, fcid)
			}

			// Record the number of gaps that this chunk has and add the chunk
			// to the repair matrix.
			gapCounts[gaps.numGaps()]++
			cid := chunkID{i, file.name}
			repairMatrix[cid] = &gaps
		}
	}
}

// createRepairMatrix will fetch a list of incomplete chunks within the renter
// that are ready to be repaired, along with the information needed to repair
// them.
//
// The return values are a map of chunks to the set of gaps within those
// chunks, and map indicaating how many chunks are missing each quantitiy of
// pieces.
func (r *Renter) createRepairMatrix(availableWorkers map[types.FileContractID]struct{}) (map[chunkID]*chunkGaps, map[int]int) {
	repairMatrix := make(map[chunkID]*chunkGaps)
	gapCounts := make(map[int]int)

	// Add all of the files to the repair matrix.
	for _, file := range r.files {
		_, exists := r.tracking[file.name]
		if !exists {
			continue
		}
		file.mu.Lock()
		r.addFileToRepairMatrix(file, availableWorkers, repairMatrix, gapCounts)
		file.mu.Unlock()
	}
	return repairMatrix, gapCounts
}

// managedRepairIteration does a full file repair iteration, which includes
// scanning all of the files for missing pieces and attempting repair them by
// uploading to chunks.
//
// TODO: Also include download + reupload strategies.
func (r *Renter) managedRepairIteration() {
	// Create the initial set of workers that are used to perform
	// uploading.
	availableWorkers := make(map[types.FileContractID]struct{})
	id := r.mu.RLock()
	for id, worker := range r.workerPool {
		// Ignore workers that have had an upload failure recently.
		if worker.recentUploadFailure.Add(uploadFailureCooldown).Before(time.Now()) {
			availableWorkers[id] = struct{}{}
		}
	}
	r.mu.RUnlock(id)

	// Create the repair matrix. The repair matrix is a set of chunks,
	// linked from chunk id to the set of hosts that do not have that
	// chunk.
	id = r.mu.Lock()
	repairMatrix, gapCounts := r.createRepairMatrix(availableWorkers)
	r.mu.Unlock(id)

	// Determine the maximum number of gaps of any chunk in the repair matrix.
	maxGaps := 0
	for i, gaps := range gapCounts {
		if i > maxGaps && gaps > 0 {
			maxGaps = i
		}
	}
	if maxGaps == 0 {
		// There is no work to do. Sleep for 15 minutes, or until there has
		// been a new upload. Then iterate to make a new repair matrix and
		// check again.
		select {
		case <-r.tg.StopChan():
			return
		case <-time.After(maxUploadLoopIdleTime):
			return
		case <-r.newFiles:
			return
		}
	}

	// Set up a loop that first waits for enough workers to become
	// available, and then iterates through the repair matrix to find a
	// chunk to repair.
	activeWorkers := make(map[types.FileContractID]struct{})
	for k, v := range availableWorkers {
		activeWorkers[k] = v
	}
	var retiredWorkers []types.FileContractID
	resultChan := make(chan finishedUpload)
	for {
		// Break if tg.Stop() has been called, to facilitate quick shutdown.
		select {
		case <-r.tg.StopChan():
			break
		default:
		}

		// TODO: If the number of retired workers has broken some threshold, or
		// now constitutes more than half of the total workers or something,
		// break here so that the worker pool can be reset.

		// TODO: If we've been using the same matrix for some long period of
		// time, break and get a new matrix.

		// TODO: If breaking, need to make sure that somehow we deal with
		// workers that are still in the middle of performing an upload.

		// Determine the maximum number of gaps that any chunk has.
		maxGaps := 0
		for i, gaps := range gapCounts {
			if i > maxGaps && gaps > 0 {
				maxGaps = i
			}
		}
		if maxGaps == 0 {
			// None of the chunks have any more opportunity to upload.
			break
		}

		// Iterate through the chunks until a candidate chunk is found.
		var chunksToDelete []chunkID
		for chunkID, chunkGaps := range repairMatrix {
			// Figure out how many pieces in this chunk could be repaired
			// by the current availableWorkers.
			var usefulWorkers []types.FileContractID
			for worker := range availableWorkers {
				for _, contract := range chunkGaps.contracts {
					if worker == contract {
						usefulWorkers = append(usefulWorkers, worker)
					}
				}
			}

			// Because it is expensive in terms of disk I/O and computational
			// resources to create a set of pieces to upload, a minimum number
			// of pieces are expected to be uploaded at a time so long as there
			// are chunks with at least that many gaps.
			//
			// If there are gaps with the minimum number of gaps and this chunk
			// cannot benefit sufficiently, iterate past it.
			if maxGaps >= minPiecesRepair && (len(usefulWorkers) < minPiecesRepair || chunkGaps.numGaps() < minPiecesRepair) {
				// The maxGaps for this chunk may have changed due to retired
				// workers, so it should be updated. Remove the contract ids of
				// any workers that have retired.
				gapCounts[chunkGaps.numGaps()]--
				for _, retire := range retiredWorkers {
					for i := range chunkGaps.contracts {
						if chunkGaps.contracts[i] == retire {
							chunkGaps.contracts = append(chunkGaps.contracts[:i], chunkGaps.contracts[i+1:]...)
							break
						}
					}
				}
				gapCounts[chunkGaps.numGaps()]++
				continue
			}

			// Create a repair job for this chunk.
			chunkIndex := chunkID.chunkIndex
			filename := chunkID.filename
			id := r.mu.RLock()
			file, exists1 := r.files[filename]
			meta, exists2 := r.tracking[filename]
			r.mu.RUnlock(id)

			// No need to repair a file that is no longer in the renter, or if
			// it is not being tracked for repair.
			if !exists1 || !exists2 {
				chunksToDelete = append(chunksToDelete, chunkID)
				continue
			}

			// Read the file data into memory.
			fHandle, err := os.Open(meta.RepairPath)
			if err != nil {
				// TODO: Perform a download-and-repair.
				continue
			}
			defer fHandle.Close()
			chunk := make([]byte, file.chunkSize())
			_, err = fHandle.ReadAt(chunk, int64(chunkIndex*file.chunkSize()))
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				// TODO: What's up with the ErrUnexpectedEOF? How do we know if
				// the file read was incomplete, and how do we compare that to
				// situations where we will be padding out the file?
				r.log.Println("Error reading file to perform chunk repair:", filename, meta.RepairPath, err)

				// TODO: Perform a download-and-repair.
				continue
			}
			// Erasure code the file data.
			pieces, err := file.erasureCode.Encode(chunk)
			if err != nil {
				r.log.Println("Error encoding data when trying to repair chunk:", filename, meta.RepairPath, err)
				chunksToDelete = append(chunksToDelete, chunkID)
				continue
			}
			// Encrypt the file data.
			for i := range pieces {
				key := deriveKey(file.masterKey, chunkIndex, uint64(i))
				pieces[i], err = key.EncryptBytes(pieces[i])
				if err != nil {
					r.log.Println("Error encrypting data when trying to repair chunk:", filename, meta.RepairPath, err)
					chunksToDelete = append(chunksToDelete, chunkID)
					continue
				}
			}

			// Give each piece to a worker, updating the chunkGaps and
			// availableWorkers along the way.
			for len(usefulWorkers) > 0 && len(chunkGaps.pieces) > 0 {
				uw := uploadWork{
					chunkIndex: chunkIndex,
					data:       pieces[chunkGaps.pieces[0]],
					file:       file,
					pieceIndex: chunkGaps.pieces[0],

					resultChan: resultChan,
				}
				worker := r.workerPool[usefulWorkers[0]]
				select {
				case worker.uploadChan <- uw:
				case <-r.tg.StopChan():
					return
				}

				delete(availableWorkers, usefulWorkers[0])
				for j := 0; j < len(chunkGaps.contracts); j++ {
					if chunkGaps.contracts[j] == usefulWorkers[0] {
						gapCounts[chunkGaps.numGaps()]--
						chunkGaps.contracts = append(chunkGaps.contracts[:j], chunkGaps.contracts[j+1:]...)
						chunkGaps.pieces = chunkGaps.pieces[1:]
						usefulWorkers = usefulWorkers[1:]
						gapCounts[chunkGaps.numGaps()]++
						break
					}
				}
			}

			// Only add one chunk per iteration of the outer loop.
			break
		}
		// Delete any chunks that do not need work.
		for _, ctd := range chunksToDelete {
			delete(repairMatrix, ctd)
		}

		// Determine the number of workers we need in 'available'.
		exclude := maxGaps - minPiecesRepair
		if exclude < 0 {
			exclude = 0
		}
		need := len(activeWorkers) - exclude
		if need <= len(availableWorkers) {
			need = len(availableWorkers) + 1
		}
		if need > len(activeWorkers) {
			need = len(activeWorkers)
		}

		// Wait until 'need' workers are available.
		for len(availableWorkers) < need {
			var finishedUpload finishedUpload
			select {
			case finishedUpload = <-resultChan:
			case <-r.tg.StopChan():
				return
			}

			// If there was no error, add the worker back to the set of
			// available workers and wait for the next worker.
			if finishedUpload.err == nil {
				availableWorkers[finishedUpload.workerID] = struct{}{}
				continue
			}

			// Log the error.
			r.log.Debugln("Error while performing upload to", finishedUpload.workerID, "::", finishedUpload.err)

			// Retire the worker that failed to perform the upload.
			id := r.mu.Lock()
			worker, exists := r.workerPool[finishedUpload.workerID]
			if exists {
				worker.recentUploadFailure = time.Now()
				retiredWorkers = append(retiredWorkers, finishedUpload.workerID)
				delete(activeWorkers, finishedUpload.workerID)
				need--
			}
			r.mu.Unlock(id)

			// Piece will not be added back into the repair matrix, this chunk
			// is doomed to wait until the next iteration.
		}

		// Receive all of the new files and add them to the repair matrix
		// before continuing.
		var done bool
		for !done {
			select {
			case file := <-r.newFiles:
				r.addFileToRepairMatrix(file, activeWorkers, repairMatrix, gapCounts)
			default:
				done = true
			}
		}
	}
}

// threadedRepairLoop improves the health of files tracked by the renter by
// reuploading their missing pieces. Multiple repair attempts may be necessary
// before the file reaches full redundancy.
func (r *Renter) threadedRepairLoop() {
	for {
		if r.tg.Add() != nil {
			return
		}
		r.managedRepairIteration()
		r.tg.Done()
	}
}
