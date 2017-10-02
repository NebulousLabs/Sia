package renter

// TODO / NOTE: Once the filesystem is tree-based, instead of continually
// looping through the whole filesystem we can add values to the file metadata
// indicating the health of each folder + file, and the time of the last scan
// for each folder + file, where the folder scan time is the least recent time
// of any file in the folder, and the folder health is the lowest health of any
// file in the folder. This will allow us to go one folder at a time and focus
// on problem areas instead of doing everything all at once every iteration.
// This should boost scalability.

// TODO / NOTE: We need to upgrade the contractor before we can do this, but we
// need to be checking for every piece within a contract, and checking that the
// piece is still available in the contract that we have, that the host did not
// lose or nullify the piece.

// TODO: Need to make sure that we wait for all standing upload jobs to
// complete in some way before we send off the next round of upload jobs.
// Otherwise we might end up with simultenously overlapping repair jobs.

import (
	"container/heap"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
)

// ChunkHeap is a bunch of chunks sorted by percentage-completion for uploading.
// This is a temporary situation, once we have a filesystem we can do
// tree-diving instead to build out our chunk profile. This just simulates that.
type chunkHeap []*unfinishedChunk

// unfinishedChunk contains a chunk from the filesystem that has not finished
// uploading, including knowledge of the progress.
type unfinishedChunk struct {
	// Information about the file. localPath may be the empty string if the file
	// is known not to exist locally.
	renterFile *file
	localPath  string

	// Information about the chunk, namely where it exists within the file.
	//
	// TODO / NOTE: As we change the file mapper, we're probably going to have
	// to update these fields. Compatibility shouldn't be an issue because this
	// struct is not persisted anywhere, it's always built from other
	// structures.
	index          uint64
	length         uint64
	memoryNeeded   uint64 // memory needed in bytes
	memoryReleased uint64 // memory that has been returned of memoryNeeded
	minimumPieces  int    // number of pieces required to recover the file.
	offset         int64
	piecesNeeded   int // number of pieces to achieve a 100% complete upload

	// The logical data is the data that is presented to the user when the user
	// requests the chunk. The physical data is all of the pieces that get
	// stored across the network.
	logicalChunkData  []byte
	physicalChunkData [][]byte

	// Worker synchronization fields. The mutex only protects these fields.
	mu               sync.Mutex
	pieceUsage       []bool              // one per piece. 'false' = piece not uploaded. 'true' = piece uploaded.
	piecesCompleted  int                 // number of pieces that have been fully uploaded.
	piecesRegistered int                 // number of pieces that are being uploaded, but aren't finished yet.
	unusedHosts      map[string]struct{} // hosts that aren't yet storing any pieces
	workersRemaining int                 // number of workers who have received the chunk, but haven't finished processing it.
}

// Implementation of heap.Interface for chunkHeap.
func (ch chunkHeap) Len() int { return len(ch) }
func (ch chunkHeap) Less(i, j int) bool {
	return float64(ch[i].piecesCompleted)/float64(ch[i].piecesNeeded) < float64(ch[j].piecesCompleted)/float64(ch[j].piecesNeeded)
}
func (ch chunkHeap) Swap(i, j int)       { ch[i], ch[j] = ch[j], ch[i] }
func (ch *chunkHeap) Push(x interface{}) { *ch = append(*ch, x.(*unfinishedChunk)) }
func (ch *chunkHeap) Pop() interface{} {
	old := *ch
	n := len(old)
	x := old[n-1]
	*ch = old[0 : n-1]
	return x
}

// buildUnfinishedChunks will pull all of the unfinished chunks out of a file.
//
// TODO / NOTE: This code can be substantially simplified once the files store
// the HostPubKey instead of the FileContractID, and can be simplified even
// further once the layout is per-chunk instead of per-filecontract.
func (r *Renter) buildUnfinishedChunks(f *file, hosts map[string]struct{}) []*unfinishedChunk {
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
	for i := uint64(0); i < chunkCount; i++ {
		newUnfinishedChunks[i] = &unfinishedChunk{
			renterFile: f,
			localPath:  trackedFile.RepairPath,

			index:  i,
			length: f.chunkSize(),
			offset: int64(i * f.chunkSize()),

			// memoryNeeded has to also include the logical data, and also
			// include the overhead for encryption.
			//
			// TODO / NOTE: If we adjust the file to have a flexible encryption
			// scheme, we'll need to adjust the overhead stuff too.
			memoryNeeded:  f.pieceSize*uint64(f.erasureCode.NumPieces()+f.erasureCode.MinPieces()) + uint64(f.erasureCode.NumPieces()*crypto.TwofishOverhead),
			minimumPieces: f.erasureCode.MinPieces(),
			piecesNeeded:  f.erasureCode.NumPieces(),
			pieceUsage:    make([]bool, f.erasureCode.NumPieces()),
			unusedHosts:   make(map[string]struct{}),
		}
		// Every chunk can have a different set of unused hosts.
		for host := range hosts {
			newUnfinishedChunks[i].unusedHosts[host] = struct{}{}
		}
	}

	// Iterate through the contracts of the file and mark which hosts are
	// already in use for the chunk. As you delete hosts from the 'unusedHosts'
	// map, also increment the 'piecesCompleted' value.
	saveFile := false
	for fcid, fileContract := range f.contracts {
		recentContract, exists := r.hostContractor.ResolveContract(fcid)
		if !exists {
			// File contract does not seem to be part of the host anymore.
			// Delete this contract and mark the file to be saved.
			delete(f.contracts, fcid)
			saveFile = true
			continue
		}
		if !recentContract.GoodForUpload {
			// We are no longer renewing with this contract, so it does not
			// count for redundancy.
			continue
		}
		hpk := recentContract.HostPublicKey

		// Mark the chunk set based on the pieces in this contract.
		for _, piece := range fileContract.Pieces {
			_, exists := newUnfinishedChunks[piece.Chunk].unusedHosts[hpk.String()]
			redundantPiece := newUnfinishedChunks[piece.Chunk].pieceUsage[piece.Piece]
			if exists && !redundantPiece {
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
	incompleteChunks := newUnfinishedChunks[:0]
	for i := 0; i < len(newUnfinishedChunks); i++ {
		if newUnfinishedChunks[i].piecesCompleted < newUnfinishedChunks[i].piecesNeeded {
			incompleteChunks = append(incompleteChunks, newUnfinishedChunks[i])
		}
	}
	return incompleteChunks
}

// managedBuildChunkHeap will iterate through all of the files in the renter and
// construct a chunk heap.
func (r *Renter) managedBuildChunkHeap(hosts map[string]struct{}) *chunkHeap {
	// Loop through the whole set of files to build the chunk heap.
	ch := new(chunkHeap)
	heap.Init(ch)
	id := r.mu.Lock()
	for _, file := range r.files {
		unfinishedChunks := r.buildUnfinishedChunks(file, hosts)
		for i := 0; i < len(unfinishedChunks); i++ {
			heap.Push(ch, unfinishedChunks[i])
		}
	}
	r.mu.Unlock(id)

	// Init the heap.
	return ch
}

// managedInsertFileIntoChunkHeap will insert all of the chunks of a file into the
// chunk heap.
func (r *Renter) managedInsertFileIntoChunkHeap(f *file, ch *chunkHeap, hosts map[string]struct{}) {
	id := r.mu.Lock()
	unfinishedChunks := r.buildUnfinishedChunks(f, hosts)
	for i := 0; i < len(unfinishedChunks); i++ {
		heap.Push(ch, unfinishedChunks[i])
	}
	r.mu.Unlock(id)
}

// managedPrepareNextChunk takes the next chunk from the chunk heap and prepares
// it for upload. Preparation includes blocking until enough memory is
// available, fetching the logical data for the chunk (either from the disk or
// from the network), erasure coding the logical data into the physical data,
// and then finally passing the work onto the workers.
//
// TODO: Need to turn this into a smarter memory pool construction - this
// construction as it stands has a race condition. Instead of blocking until a
// memory refresh signal is received, it should just call 'AcquireMemory' on a
// pool object or something, and then that object can worry about breaking and
// stuff, and can also make sure that the memory goes to only one place.
func (r *Renter) managedPrepareNextChunk(ch *chunkHeap, hosts map[string]struct{}) {
	// Grab the next chunk, loop until we have enough memory, update the amount
	// of memory available, and then spin up a thread to asynchronously handle
	// the rest of the chunk tasks.
	memoryAvailable := r.managedMemoryAvailableGet()
	nextChunk := heap.Pop(ch).(*unfinishedChunk)
	for nextChunk.memoryNeeded > memoryAvailable {
		select {
		case newFile := <-r.newUploads:
			r.managedInsertFileIntoChunkHeap(newFile, ch, hosts)
		case <-r.newMemory:
			memoryAvailable = r.managedMemoryAvailableGet()
		case <-r.tg.StopChan():
		}
	}
	r.managedMemoryAvailableSub(nextChunk.memoryNeeded)
	// Add this thread to the waitgroup. This Add will be released once the
	// worker threads have been added to the wg.
	r.heapWG.Add(1)
	go func() {
		workDistributed := r.managedFetchAndRepairChunk(nextChunk)
		r.heapWG.Done()
		if !workDistributed {
			// Release any data that did not get distributed to workers.
			r.managedMemoryAvailableAdd(nextChunk.memoryNeeded - nextChunk.memoryReleased)
		} else {
			nextChunk.mu.Lock()
			nextChunk.mu.Unlock()
		}

		// Sanity check, make sure memory was returned properly.
		if nextChunk.logicalChunkData != nil {
			nextChunk.logicalChunkData = nil
			r.log.Critical("logical chunk data was not cleaned up correctly")
		}
	}()
}

// managedRefreshHostsAndWorkers will reset the set of hosts and the set of
// workers for the renter.
func (r *Renter) managedRefreshHostsAndWorkers() map[string]struct{} {
	// Grab the current set of contracts and use them to build a list of hosts
	// that are available for uploading. The hosts are assembled into a map
	// where the key is the String() representation of the host's SiaPublicKey.
	//
	// TODO / NOTE: This code can be removed once files store the HostPubKey
	// of the hosts they are using, instead of just the FileContractID.
	currentContracts := r.hostContractor.Contracts()
	hosts := make(map[string]struct{})
	for _, contract := range currentContracts {
		hosts[contract.HostPublicKey.String()] = struct{}{}
	}

	// Refresh the worker pool as well.
	r.managedUpdateWorkerPool()
	return hosts
}

// threadedRepairScan is a background thread that checks on the health of files,
// tracking the least healthy files and queuing the worst ones for repair.
//
// TODO / NOTE: Once we have upgraded the filesystem, we can replace this with
// the tree-diving technique discussed in Sprint 5. For now we just iterate
// through all of our in-memory files and chunks, and maintain a finite list of
// the worst ones, and then iterate through it again when we need to find more
// things to repair.
func (r *Renter) threadedRepairScan() {
	err := r.tg.Add()
	if err != nil {
		return
	}
	defer r.tg.Done()

	for {
		// Return if the renter has shut down.
		select {
		case <-r.tg.StopChan():
			return
		default:
		}

		// Refresh the worker pool and get the set of hosts that are currently
		// useful for uploading.
		hosts := r.managedRefreshHostsAndWorkers()

		// Build a min-heap of chunks organized by upload progress.
		chunkHeap := r.managedBuildChunkHeap(hosts)
		r.log.Println("Repairing", chunkHeap.Len(), "chunks")

		// Work through the heap. Chunks will be processed one at a time until
		// the heap is whittled down. When the heap is empty, we wait for new
		// files in a loop and then process those. When the rebuild signal is
		// received, we start over with the outer loop that rebuilds the heap
		// and re-checks the health of all the files.
		rebuildHeapSignal := time.After(rebuildChunkHeapInterval)
	LOOP:
		for {
			// Return if the renter has shut down.
			select {
			case <-r.tg.StopChan():
				return
			default:
			}

			if chunkHeap.Len() > 0 {
				r.managedPrepareNextChunk(chunkHeap, hosts)
			} else {
				// Block until the rebuild signal is received.
				select {
				case newFile := <-r.newUploads:
					// If a new file is received, add its chunks to the repair
					// heap and loop to start working through those chunks.
					// Update the worker pool before processing the file, as it
					// may have been a while since the previous update.
					hosts = r.managedRefreshHostsAndWorkers()
					r.managedInsertFileIntoChunkHeap(newFile, chunkHeap, hosts)
					continue
				case <-rebuildHeapSignal:
					// If the rebuild heap signal is received, break out to the
					// outer loop which will check the health of all filess
					// again and then rebuild the heap.
					r.heapWG.Wait()
					break LOOP
				case <-r.tg.StopChan():
					// If the stop signal is received, quit entirely.
					return
				}
			}
		}
	}
}
