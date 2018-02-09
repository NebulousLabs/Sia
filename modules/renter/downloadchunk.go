package renter

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// downloadPieceInfo contains all the information required to download and
// recover a piece of a chunk from a host. It is a value in a map where the key
// is the file contract id.
type downloadPieceInfo struct {
	index uint64
	root  crypto.Hash
}

// unfinishedDownloadChunk contains a chunk for a download that is in progress.
//
// TODO: The memory management is not perfect here. As we collect data pieces
// (instead of parity pieces), we don't need as much memory to recover the
// original data. But we do already allocate only as much as we potentially
// need, meaning that you can't naively release some memory to the renter each
// time a data piece completes, you have to check that the data piece was not
// already expected to be required for the download.
//
// TODO: Combine chunkMap and rootMap into single structure.
type unfinishedDownloadChunk struct {
	// Fetch + Write instructions - read only or otherwise thread safe.
	destination downloadDestination // Where to write the recovered logical chunk.
	erasureCode modules.ErasureCoder
	masterKey   crypto.TwofishKey

	// Fetch + Write instructions - read only or otherwise thread safe.
	staticChunkIndex  uint64                                     // Required for deriving the encryption keys for each piece.
	staticChunkMap    map[types.FileContractID]downloadPieceInfo // Maps from file contract ids to the info for the piece associated with that contract
	staticChunkSize   uint64
	staticFetchLength uint64 // Length within the logical chunk to fetch.
	staticFetchOffset uint64 // Offset within the logical chunk that is being downloaded.
	staticPieceSize   uint64
	staticWriteOffset int64 // Offet within the writer to write the completed data.

	// Fetch + Write instructions - read only or otherwise thread safe.
	staticLatencyTarget uint64
	staticNeedsMemory   bool // Set to true if memory was not pre-allocated for this chunk.
	staticOverdrive     int
	staticPriority      uint64

	// Download chunk state - need mutex to access.
	physicalChunkData [][]byte  // Used to recover the logical data.
	pieceUsage        []bool    // Which pieces are being actively fetched.
	piecesCompleted   int       // Number of pieces that have successfully completed.
	piecesRegistered  int       // Number of pieces that workers are actively fetching.
	workersRemaining  int       // Number of workers still able to fetch the chunk.
	workersStandby    []*worker // Set of workers that are able to work on this download, but are not needed unless other workers fail.

	// Memory management variables.
	memoryAllocated int

	// The download object, mostly to update download progress.
	download *download
	mu       sync.Mutex
}

// fail will mark the chunk as failed, and then fail the whole download as well.
//
// TODO: Error message can at least declare which chunk failed within the
// download.
//
// TODO: Should we have the ability to log this failure? Might that need to be
// done by the worker?
func (udc *unfinishedDownloadChunk) fail() {
	if udc.failed {
		// Failure code has already run, no need to run again.
		return
	}
	udc.failed = true
	// TODO: managed fail? Fail in a goroutine? Need to be careful with
	// concurrency here.
	udc.download.managedFail(errors.New("not enough working hosts to recover file"))
}

// recoverLogicalData
func (udc *unfinishedDownloadChunk) recoverLogicalData() error {
	// Assemble the chunk from the download.
	cd.download.mu.Lock()
	chunk := make([][]byte, cd.download.erasureCode.NumPieces())
	for pieceIndex, pieceData := range cd.completedPieces {
		chunk[pieceIndex] = pieceData
	}
	complete := cd.download.downloadComplete
	prevErr := cd.download.downloadErr
	cd.download.mu.Unlock()

	// Return early if the download has previously suffered an error.
	if complete {
		return build.ComposeErrors(errPrevErr, prevErr)
	}

	// Decrypt the chunk pieces.
	for i := range chunk {
		// Skip pieces that were not downloaded.
		if chunk[i] == nil {
			continue
		}

		// Decrypt the piece.
		key := deriveKey(cd.download.masterKey, cd.index, uint64(i))
		decryptedPiece, err := key.DecryptBytes(chunk[i])
		if err != nil {
			return build.ExtendErr("unable to decrypt piece", err)
		}
		chunk[i] = decryptedPiece
	}

	// Recover the chunk into a byte slice.
	recoverWriter := new(bytes.Buffer)
	recoverSize := cd.download.chunkSize
	if cd.index == cd.download.numChunks-1 && cd.download.fileSize%cd.download.chunkSize != 0 {
		recoverSize = cd.download.fileSize % cd.download.chunkSize
	}
	err := cd.download.erasureCode.Recover(chunk, recoverSize, recoverWriter)
	if err != nil {
		return build.ExtendErr("unable to recover chunk", err)
	}

	result := recoverWriter.Bytes()

	// Calculate the offset. If the offset is within the chunk, the
	// requested offset is passed, otherwise the offset of the chunk
	// within the overall file is passed.
	chunkBaseAddress := cd.index * cd.download.chunkSize
	chunkTopAddress := chunkBaseAddress + cd.download.chunkSize - 1
	off := chunkBaseAddress
	lowerBound := 0
	if cd.download.offset >= chunkBaseAddress && cd.download.offset <= chunkTopAddress {
		off = cd.download.offset
		offsetInBlock := off - chunkBaseAddress
		lowerBound = int(offsetInBlock) // If the offset is within the block, part of the block will be ignored
	}

	// Truncate b if writing the whole buffer at the specified offset would
	// exceed the maximum file size.
	upperBound := cd.download.chunkSize
	if chunkTopAddress > cd.download.length+cd.download.offset {
		diff := chunkTopAddress - (cd.download.length + cd.download.offset)
		upperBound -= diff + 1
	}
	if upperBound > uint64(len(result)) {
		upperBound = uint64(len(result))
	}

	result = result[lowerBound:upperBound]

	// Write the bytes to the requested output.
	_, err = cd.download.destination.WriteAt(result, int64(off))
	if err != nil {
		return build.ExtendErr("unable to write to download destination", err)
	}

	cd.download.mu.Lock()
	defer cd.download.mu.Unlock()

	// Update the download to signal that this chunk has completed. Only update
	// after the sync, so that durability is maintained.
	if cd.download.finishedChunks[cd.index] {
		build.Critical("recovering chunk when the chunk has already finished downloading")
	}
	cd.download.finishedChunks[cd.index] = true

	// Determine whether the download is complete.
	nowComplete := true
	for _, chunkComplete := range cd.download.finishedChunks {
		if !chunkComplete {
			nowComplete = false
			break
		}
	}
	if nowComplete {
		// Signal that the download is complete.
		cd.download.downloadComplete = true
		close(cd.download.downloadFinished)
		err = cd.download.destination.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
