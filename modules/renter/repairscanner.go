package renter

// TODO: Make sure the `newMemory` channel for the renter is buffered out to one
// element.

// TODO: When building the chunk, need to also include a list of pieces that
// aren't properly replicated yet.

// TODO / NOTE: Once the filesystem is tree-based, instead of continually
// looping through the whole filesystem we can add values to the file metadata
// indicating the health of each folder + file, and the time of the last scan
// for each folder + file, where the folder scan time is the least recent time
// of any file in the folder, and the folder health is the lowest health of any
// file in the folder. This will allow us to go one folder at a time and focus
// on problem areas instead of doing everything all at once every iteration.
// This should boost scalability.

import (
	"container/heap"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/Sia/modules"
)

// ChunkHeap is a bunch of chunks sorted by percentage-completion for uploading.
// This is a temporary situation, once we have a filesystem we can do
// tree-diving instead to build out our chunk profile. This just simulates that.
type chunkHeap []*unfinishedChunk

// unfinishedChunk contains a chunk from the filesystem that has not finished
// uploading, including knowledge of the progress.
type unfinishedChunk struct {
	renterFile *file

	localPath string

	index  uint64
	length uint64
	offset int64

	logicalChunkData  []byte
	physicalChunkData [][]byte

	// progress is used to sort the chunks in the chunkHeap. '0' indicates a
	// completely unfinished file, and '1' indicates a completely finished file,
	// meaning it's not actually an unfinished chunk.
	progress float64

	// unusedHosts is a list of hosts by pubkey that have not contributed to
	// this chunk yet. The key is the String() representation of a SiaPublicKey,
	// as a SiaPublicKey cannot be used as a map key directly.
	memoryNeeded     uint64
	piecesNeeded     int
	piecesCompleted  int
	piecesRegistered int
	pieceUsage       []bool // one element per piece. 'false' = piece not uploaded to a host. 'true' = piece uploaded to a host.
	unusedHosts      map[string]struct{}

	mu sync.Mutex
}

func (ch chunkHeap) Len() int           { return len(ch) }
func (ch chunkHeap) Less(i, j int) bool { return ch[i].progress < ch[j].progress }
func (ch chunkHeap) Swap(i, j int)      { ch[i], ch[j] = ch[j], ch[i] }
func (ch *chunkHeap) Push(x interface{}) { *ch = append(*ch, x.(*unfinishedChunk)) }
func (ch *chunkHeap) Pop() interface{} {
	old := *ch
	n := len(old)
	x := old[n-1]
	*ch = old[0 : n-1]
	return x
}

// managedBuildChunkHeap will iterate through all of the files in the renter and
// construct a chunk heap.
func (r *Renter) managedBuildChunkHeap(hosts map[string]struct{}, fcidToHPK map[types.FileContractID]types.SiaPublicKey) *chunkHeap {
	id := r.mu.Lock()
	defer r.mu.Unlock(id)

	// Loop through the whole set of files to build the chunk heap.
	var ch chunkHeap
	for _, file := range r.files {
		unfinishedChunks := r.buildUnfinishedChunks(file, hosts, fcidToHPK)
		ch = append(ch, unfinishedChunks...)
	}

	// Init the heap.
	heap.Init(&ch)
	return &ch
}

// buildUnfinishedChunks will pull all of the unfinished chunks out of a file.
//
// TODO / NOTE: This code can be substantially simplified once the files store
// the HostPubKey instead of the FileContractID, and can be simplified even
// further once the layout is per-chunk instead of per-filecontract.
func (r *Renter) buildUnfinishedChunks(f *file, hosts map[string]struct{}, fcidToHPK map[types.FileContractID]types.SiaPublicKey) []*unfinishedChunk {
	// Files are not threadsafe.
	f.mu.Lock()
	defer f.mu.Unlock()

	// If the file is not being tracked, don't repair it.
	trackedFile, exists := r.tracking[f.name]
	if !exists {
		return nil
	}

	// Assemble the set of chunks.
	//
	// TODO / NOTE: Future files may have a different method for determining the
	// number of chunks. Changes will be made due to things like sparse files,
	// and the fact that chunks are going to be different sizes.
	chunkCount := f.numChunks()
	newUnfinishedChunks := make([]*unfinishedChunk, chunkCount)
	// Add a separate unusedHosts map for each chunk, as every chunk will have a
	// different set of unused hosts.
	for i := uint64(0); i < chunkCount; i++ {
		newUnfinishedChunks[i] = new(unfinishedChunk)
		newUnfinishedChunks[i].index = i
		newUnfinishedChunks[i].localPath = trackedFile.RepairPath

		// Mark the number of pieces needed for this chunk.
		newUnfinishedChunks[i].piecesNeeded = f.erasureCode.NumPieces()
		newUnfinishedChunks[i].memoryNeeded = f.pieceSize * uint64(f.erasureCode.NumPieces())

		// TODO / NOTE: Offset and length are going to have to be derived using
		// alternate means once chunks are no longer constant size within a
		// file. Likely the chunk metadata will contain this information, but we
		// also want to make sure that files are random-access, and don't
		// require seeking through a ton of chunk headers to get to an arbitrary
		// position. It's currently an open problem.
		newUnfinishedChunks[i].offset = int64(i * f.chunkSize())
		newUnfinishedChunks[i].length = f.chunkSize()

		// Fill out the set of unused hosts.
		newUnfinishedChunks[i].pieceUsage = make([]bool, f.erasureCode.NumPieces())
		newUnfinishedChunks[i].unusedHosts = make(map[string]struct{})
		for host := range hosts {
			newUnfinishedChunks[i].unusedHosts[host] = struct{}{}
		}
	}

	// Iterate through the contracts of the file and mark which hosts are
	// already in use for the chunk. As you delete hosts from the 'unusedHosts'
	// map, also increment the 'piecesCompleted' value.
	saveFile := false
	for fcid, fileContract := range f.contracts {
		// Convert the FileContractID into a host pubkey using the host pubkey
		// lookup.
		hpk, exists := fcidToHPK[fileContract.ID]
		if !exists {
			// File contract does not seem to be part of the host anymore.
			// Delete this contract and mark the file to be saved.
			delete(f.contracts, fcid)
			saveFile = true
			continue
		}

		// Mark the chunk set based on the pieces in this contract.
		for _, piece := range fileContract.Pieces {
			_, exists := newUnfinishedChunks[piece.Chunk].unusedHosts[hpk.String()]
			nonRedundantPiece := newUnfinishedChunks[piece.Chunk].pieceUsage[piece.Piece]
			if exists && nonRedundantPiece {
				newUnfinishedChunks[piece.Chunk].pieceUsage[piece.Piece] = true
				newUnfinishedChunks[piece.Chunk].piecesCompleted++
				delete(newUnfinishedChunks[piece.Chunk].unusedHosts, hpk.String())
			} else if exists {
				// TODO / NOTE: This host has a piece, but it's the same piece
				// that another host has. We may want to take action (such as
				// deleting this piece from this host) because of this
				// inefficiency.
			}
		}
	}
	// If 'saveFile' is marked, it means we deleted some dead contracts and
	// cleaned up the file a bit. Save the file to clean up some space on disk
	// and prevent the same work from being repeated after the next restart.
	//
	// TODO / NOTE: This process isn't going to make sense anymore once we
	// switch to chunk-based saving.
	if saveFile {
		err := r.saveFile(f)
		if err != nil {
			r.log.Println("error while saving a file after pruning some contracts from it:", err)
		}
	}

	// Iterate through the set of newUnfinishedChunks and remove any that are
	// completed.
	totalIncomplete := 0
	for i := 0; i < len(newUnfinishedChunks); i++ {
		if newUnfinishedChunks[i].piecesCompleted < newUnfinishedChunks[i].piecesNeeded {
			newUnfinishedChunks[totalIncomplete] = newUnfinishedChunks[i]
			newUnfinishedChunks[i].progress = float64(newUnfinishedChunks[i].piecesCompleted) / float64(newUnfinishedChunks[i].piecesNeeded)
			totalIncomplete++
		}
	}
	newUnfinishedChunks = newUnfinishedChunks[:totalIncomplete]
	return newUnfinishedChunks
}

// managedInsertFileIntoChunkHeap will insert all of the chunks of a file into the
// chunk heap.
func (r *Renter) managedInsertFileIntoChunkHeap(f *file, hosts map[string]struct{}, fcidToHPK map[types.FileContractID]types.SiaPublicKey, ch *chunkHeap) {
	id := r.mu.Lock()
	unfinishedChunks := r.buildUnfinishedChunks(f, hosts, fcidToHPK)
	for i := 0; i < len(unfinishedChunks); i++ {
		heap.Push(ch, unfinishedChunks)
	}
	r.mu.Unlock(id)
}

// threadedRepairScan is a background thread that checks on the health of files,
// tracking the least healthy files and queuing the worst ones for repair.
//
// Once we have upgraded the filesystem, we can replace this with the
// tree-diving technique discussed in Sprint 5. For now we just iterate through
// all of our in-memory files and chunks, and maintain a finite list of the
// worst ones, and then iterate through it again when we need to find more
// things to repair.
func (r *Renter) threadedRepairScan() {
	err := r.tg.Add()
	if err != nil {
		return
	}
	defer r.tg.Done()

	for {
		// Grab the set of renewed ids and contracts. Then use them to create a
		// table that connects any historic FileContractID to the corresponding
		// HostPublicKey.
		//
		// TODO / NOTE: This code can be removed once files store the HostPubKey
		// of the hosts they are using, instead of just the FileContractID.
		renewedIDs, currentContracts := r.hostContractor.ContractLookups()
		fcidToHPK := make(map[types.FileContractID]types.SiaPublicKey)
		for oldID, newID := range renewedIDs {
			// First resolve the oldID into the most recent file contract id
			// available.
			finalID := newID
			nextID, exists := renewedIDs[newID]
			for exists {
				finalID = nextID
				nextID, exists = renewedIDs[nextID]
			}

			// Determine if the final id is available in the current set of
			// contracts. If it is available, add oldID to the fcidToHPK map.
			contract, exists := currentContracts[finalID]
			if exists {
				fcidToHPK[oldID] = contract.HostPublicKey
			}
		}

		// Pull together a list of hosts that are available for uploading. We
		// assemble them into a map where the key is the String() representation
		// of a types.SiaPublicKey (which cannot be used as a map key itself).
		hosts := make(map[string]struct{})
		sliceContracts := make([]modules.RenterContract, 0, len(currentContracts))
		for _, contract := range currentContracts {
			hosts[contract.HostPublicKey.String()] = struct{}{}
			sliceContracts = append(sliceContracts, contract)
		}
		// Build a min-heap of chunks organized by upload progress.
		chunkHeap := r.managedBuildChunkHeap(hosts, fcidToHPK)

		// Update the worker pool based on the current contracts.
		r.managedUpdateWorkerPool()

		// Work through the heap. As the heap is processed, frequent checks are
		// made for new files being uploaded. When the heap is fully processed,
		// sleep until until the next heap rebuild is required, though that
		// sleep should still be receiving and processing new chunks.
		rebuildHeapSignal := time.After(repairQueueInterval)
		for {
			// If there is a next chunk in the heap, grab it and block until
			// there's enough memory. Otherwise, block until a new file appears
			// or until we get the rebuild heap signal.
			if chunkHeap.Len() > 0 {
				// There is a chunk in the heap. Grab it, block until we have
				// enough memory to repair it, and then perform the repair.
				id := r.mu.RLock()
				memoryAvailable := r.uploadMemoryAvailable
				r.mu.RUnlock(id)
				nextChunk := chunkHeap.Pop().(*unfinishedChunk)
				for nextChunk.memoryNeeded > memoryAvailable {
					select {
					case newFile := <-r.newRepairs:
						r.managedInsertFileIntoChunkHeap(newFile, hosts, fcidToHPK, chunkHeap)
					case <-r.newMemory:
					case <-r.tg.StopChan():
						return
					}
				}
				id = r.mu.Lock()
				memoryAvailable -= nextChunk.memoryNeeded
				r.mu.Unlock(id)
				go r.managedFetchAndRepairChunk(nextChunk)
			} else {
				// The chunk heap is empty. Block until there's either a new
				// file, or until we
				select {
				case newFile := <-r.newRepairs:
					r.managedInsertFileIntoChunkHeap(newFile, hosts, fcidToHPK, chunkHeap)
					continue
				case <-rebuildHeapSignal:
					break
				case <-r.tg.StopChan():
					return
				}
			}
		}
	}
}
