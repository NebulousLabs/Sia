package renter

// TODO / NOTE: Once the filesystem is tree-based, instead of continually
// looping through the whole filesystem we can add values to the file metadata
// for each folder + file, where the folder scan time is the least recent time
// of any file in the folder, and the folder health is the lowest health of any
// file in the folder. This will allow us to go one folder at a time and focus
// on problem areas instead of doing everything all at once every iteration.
// This should boost scalability.

// TODO / NOTE: We need to upgrade the contractor before we can do this, but we
// need to be checking for every piece within a contract, and checking that the
// piece is still available in the contract that we have, that the host did not
// lose or nullify the piece.

// TODO: Renter will try to download to repair a piece even if there are not
// enough workers to make any progress on the repair.  This should be fixed.

import (
	"container/heap"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
)

// uploadHeap contains a priority-sorted heap of all the chunks being uploaded
// to the renter, along with some metadata.
type uploadHeap struct {
	// activeChunks contains a list of all the chunks actively being worked on.
	// These chunks will either be in the heap, or will be in the queues of some
	// of the workers. A chunk is added to the activeChunks map as soon as it is
	// added to the uploadHeap, and it is removed from the map as soon as the
	// last worker completes work on the chunk.
	activeChunks map[uploadChunkID]struct{}
	heap         uploadChunkHeap
	newUploads   chan struct{}
	mu           sync.Mutex
}

// uploadChunkHeap is a bunch of priority-sorted chunks that need to be either
// uploaded or repaired.
//
// TODO: When the file system is adjusted to have a tree structure, the
// filesystem itself will serve as the uploadChunkHeap, making this structure
// unnecessary. The repair loop might be moved to repair.go.
type uploadChunkHeap []*unfinishedUploadChunk

// Implementation of heap.Interface for uploadChunkHeap.
func (uch uploadChunkHeap) Len() int { return len(uch) }
func (uch uploadChunkHeap) Less(i, j int) bool {
	return float64(uch[i].piecesCompleted)/float64(uch[i].piecesNeeded) < float64(uch[j].piecesCompleted)/float64(uch[j].piecesNeeded)
}
func (uch uploadChunkHeap) Swap(i, j int)       { uch[i], uch[j] = uch[j], uch[i] }
func (uch *uploadChunkHeap) Push(x interface{}) { *uch = append(*uch, x.(*unfinishedUploadChunk)) }
func (uch *uploadChunkHeap) Pop() interface{} {
	old := *uch
	n := len(old)
	x := old[n-1]
	*uch = old[0 : n-1]
	return x
}

// managedAddChunkToUploadHeap will add a chunk to the upload heap.
func (uh *uploadHeap) managedPush(uuc *unfinishedUploadChunk) {
	// Create the unique chunk id.
	ucid := uploadChunkID{
		fileUID: uuc.renterFile.staticUID,
		index:   uuc.index,
	}
	// Sanity check: fileUID should not be the empty value.
	if uuc.renterFile.staticUID == "" {
		panic("empty string for file UID")
	}

	// Check whether this chunk is already being repaired. If not, add it to the
	// upload chunk heap.
	uh.mu.Lock()
	_, exists := uh.activeChunks[ucid]
	if !exists {
		uh.activeChunks[ucid] = struct{}{}
		uh.heap.Push(uuc)
	}
	uh.mu.Unlock()
}

// managedPop will pull a chunk off of the upload heap and return it.
func (uh *uploadHeap) managedPop() (uc *unfinishedUploadChunk) {
	uh.mu.Lock()
	if len(uh.heap) > 0 {
		uc = heap.Pop(&uh.heap).(*unfinishedUploadChunk)
	}
	uh.mu.Unlock()
	return uc
}

// buildUnfinishedChunks will pull all of the unfinished chunks out of a file.
//
// TODO / NOTE: This code can be substantially simplified once the files store
// the HostPubKey instead of the FileContractID, and can be simplified even
// further once the layout is per-chunk instead of per-filecontract.
func (r *Renter) buildUnfinishedChunks(f *file, hosts map[string]struct{}) []*unfinishedUploadChunk {
	// Files are not threadsafe.
	f.mu.Lock()
	defer f.mu.Unlock()

	// If the file is not being tracked, don't repair it.
	trackedFile, exists := r.persist.Tracking[f.name]
	if !exists {
		return nil
	}

	// If we don't have enough workers for the file, don't repair it right now.
	if len(r.workerPool) < f.erasureCode.MinPieces() {
		return nil
	}

	// Assemble the set of chunks.
	//
	// TODO / NOTE: Future files may have a different method for determining the
	// number of chunks. Changes will be made due to things like sparse files,
	// and the fact that chunks are going to be different sizes.
	chunkCount := f.numChunks()
	newUnfinishedChunks := make([]*unfinishedUploadChunk, chunkCount)
	for i := uint64(0); i < chunkCount; i++ {
		newUnfinishedChunks[i] = &unfinishedUploadChunk{
			renterFile: f,
			localPath:  trackedFile.RepairPath,

			id: uploadChunkID{
				fileUID: f.staticUID,
				index:   i,
			},

			index:  i,
			length: f.staticChunkSize(),
			offset: int64(i * f.staticChunkSize()),

			// memoryNeeded has to also include the logical data, and also
			// include the overhead for encryption.
			//
			// TODO / NOTE: If we adjust the file to have a flexible encryption
			// scheme, we'll need to adjust the overhead stuff too.
			//
			// TODO: Currently we request memory for all of the pieces as well
			// as the minimum pieces, but we perhaps don't need to request all
			// of that.
			memoryNeeded:  f.pieceSize*uint64(f.erasureCode.NumPieces()+f.erasureCode.MinPieces()) + uint64(f.erasureCode.NumPieces()*crypto.TwofishOverhead),
			minimumPieces: f.erasureCode.MinPieces(),
			piecesNeeded:  f.erasureCode.NumPieces(),

			physicalChunkData: make([][]byte, f.erasureCode.NumPieces()),

			pieceUsage:  make([]bool, f.erasureCode.NumPieces()),
			unusedHosts: make(map[string]struct{}),
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
		pk := r.hostContractor.ResolveIDToPubKey(fcid)
		recentContract, exists := r.hostContractor.ContractByPublicKey(pk)
		contractUtility, exists2 := r.hostContractor.ContractUtility(pk)
		if exists != exists2 {
			build.Critical("got a contract without utility or vice versa which shouldn't happen",
				exists, exists2)
		}
		if !exists || !exists2 {
			// File contract does not seem to be part of the host anymore.
			// Delete this contract and mark the file to be saved.
			delete(f.contracts, fcid)
			saveFile = true
			continue
		}
		if !contractUtility.GoodForRenew {
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
				// This host has a piece, but it is the same piece another host
				// has. We should still remove the host from the unusedHosts
				// since one host having multiple pieces of a chunk might lead
				// to unexpected issues.
				delete(newUnfinishedChunks[piece.Chunk].unusedHosts, hpk.String())
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
	// TODO: Don't return chunks that can't be downloaded, uploaded or otherwise
	// helped by the upload process.
	return incompleteChunks
}

// managedBuildChunkHeap will iterate through all of the files in the renter and
// construct a chunk heap.
func (r *Renter) managedBuildChunkHeap(hosts map[string]struct{}) {
	// Loop through the whole set of files and get a list of chunks to add to
	// the heap.
	id := r.mu.Lock()
	for _, file := range r.files {
		unfinishedUploadChunks := r.buildUnfinishedChunks(file, hosts)
		for i := 0; i < len(unfinishedUploadChunks); i++ {
			r.uploadHeap.managedPush(unfinishedUploadChunks[i])
		}
	}
	r.mu.Unlock(id)
}

// managedPrepareNextChunk takes the next chunk from the chunk heap and prepares
// it for upload. Preparation includes blocking until enough memory is
// available, fetching the logical data for the chunk (either from the disk or
// from the network), erasure coding the logical data into the physical data,
// and then finally passing the work onto the workers.
func (r *Renter) managedPrepareNextChunk(uuc *unfinishedUploadChunk, hosts map[string]struct{}) {
	// Grab the next chunk, loop until we have enough memory, update the amount
	// of memory available, and then spin up a thread to asynchronously handle
	// the rest of the chunk tasks.
	if !r.memoryManager.Request(uuc.memoryNeeded, memoryPriorityLow) {
		return
	}
	// Fetch the chunk in a separate goroutine, as it can take a long time and
	// does not need to bottleneck the repair loop.
	go r.managedFetchAndRepairChunk(uuc)
}

// managedRefreshHostsAndWorkers will reset the set of hosts and the set of
// workers for the renter.
func (r *Renter) managedRefreshHostsAndWorkers() map[string]struct{} {
	// Grab the current set of contracts and use them to build a list of hosts
	// that are currently active. The hosts are assembled into a map where the
	// key is the String() representation of the host's SiaPublicKey.
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

// threadedUploadLoop is a background thread that checks on the health of files,
// tracking the least healthy files and queuing the worst ones for repair.
func (r *Renter) threadedUploadLoop() {
	err := r.tg.Add()
	if err != nil {
		return
	}
	defer r.tg.Done()

	for {
		// Wait until the renter is online to proceed.
		if !r.managedBlockUntilOnline() {
			// The renter shut down before the internet connection was restored.
			return
		}

		// Refresh the worker pool and get the set of hosts that are currently
		// useful for uploading.
		hosts := r.managedRefreshHostsAndWorkers()

		// Build a min-heap of chunks organized by upload progress.
		//
		// TODO: After replacing the filesystem to resemble a tree, we'll be
		// able to go through the filesystem piecewise instead of doing
		// everything all at once.
		r.managedBuildChunkHeap(hosts)
		r.uploadHeap.mu.Lock()
		heapLen := r.uploadHeap.heap.Len()
		r.uploadHeap.mu.Unlock()
		r.log.Println("Repairing", heapLen, "chunks")

		// Work through the heap. Chunks will be processed one at a time until
		// the heap is whittled down. When the heap is empty, we wait for new
		// files in a loop and then process those. When the rebuild signal is
		// received, we start over with the outer loop that rebuilds the heap
		// and re-checks the health of all the files.
		rebuildHeapSignal := time.After(rebuildChunkHeapInterval)
		for {
			// Return if the renter has shut down.
			select {
			case <-r.tg.StopChan():
				return
			default:
			}

			// Break to the outer loop if not online.
			if !r.g.Online() {
				break
			}

			// Check if there is work by trying to pop of the next chunk from
			// the heap.
			nextChunk := r.uploadHeap.managedPop()
			if nextChunk == nil {
				break
			}

			// Make sure we have enough workers for this chunk to reach minimum
			// redundancy. Otherwise we ignore this chunk for now and try again
			// the next time we rebuild the heap and refresh the workers.
			id := r.mu.RLock()
			availableWorkers := len(r.workerPool)
			r.mu.RUnlock(id)
			if availableWorkers < nextChunk.minimumPieces {
				continue
			}

			// Perform the work. managedPrepareNextChunk will block until
			// enough memory is available to perform the work, slowing this
			// thread down to using only the resources that are available.
			r.managedPrepareNextChunk(nextChunk, hosts)
			continue
		}

		// Block until new work is required.
		select {
		case <-r.uploadHeap.newUploads:
			// User has uploaded a new file.
		case <-rebuildHeapSignal:
			// Time to check the filesystem health again.
		case <-r.tg.StopChan():
			// Thre renter has shut down.
			return
		}
	}
}
