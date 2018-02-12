package renter

// TODO: There are some magic numbers in this file, particularly with regards to
// latency and overdrive. They are all marked with respective TODOs.

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
)

type (
	// A download is a file download that has been queued by the renter.
	download struct {
		// Data progress variables.
		atomicDataReceived        uint64 // incremented as data completes, will stop at 100% file progress
		atomicTotalDataTransfered uint64 // incremented as data arrives, includes overdrive, contract negotitiaon, etc.

		// Other progress variables.
		chunksRemaining uint64        // Number of chunks whose downloads are incomplete.
		completeChan    chan struct{} // Closed once the download is complete.
		err             error         // Only set if there was an error which prevented the download from completing.

		// Timestamp information.
		endTime         time.Time
		staticStartTime time.Time

		// Basic information about the file.
		destination       downloadDestination
		destinationString string // The string reported to the user to indicate the download's destination.
		destinationType   string // "memory buffer", "http stream", "file", etc.
		staticLength      uint64 // Length to download starting from the offset.
		staticOffset      uint64 // Offset within the file to start the download.
		staticSiaPath     string

		// Retrieval settings for the file.
		staticLatencyTarget uint64 // In milliseconds. Lower latency results in lower total system throughput.
		staticOverdrive     int    // How many extra hosts to download from. Reduces latency at the cost of lower total system throughput.
		staticPriority      uint64 // Downloads with higher priority will complete first.

		// Utilities.
		log           *persist.Logger // Same log as the renter.
		memoryManager *memoryManager
		mu            sync.Mutex
	}

	// downloadParams is the set of parameters to use when downloading a file.
	downloadParams struct {
		destination       downloadDestination // The place to write the downloaded data.
		destinationType   string              // "file", "buffer", "http stream", etc.
		destinationString string              // The string to report to the user for the destination.
		file              *file               // The file to download.

		latencyTarget uint64 // Workers above this latency will be automatically put on standby initially.
		length        uint64 // Length of download. Cannot be 0.
		needsMemory   bool   // Whether new memory needs to be allocated to perform the download.
		offset        uint64 // Offset within the file to start the download. Must be less than the total filesize.
		overdrive     int    // Number of extra pieces to download as a race to improve latency at the cost of bandwidth efficiency.
		priority      uint64 // Files with a higher priority will be downloaded first.
	}
)

// complete is a helper function to indicate whether or not the download has
// completed.
func (d *download) complete() bool {
	select {
	case <-d.completeChan:
		return true
	default:
		return false
	}
}

// managedFail will mark the download as complete, but with the provided error.
// If the download has already failed, the error will be updated to be a
// concatenation of the previous error and the new error.
func (d *download) managedFail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// If the download is already complete, extend the error.
	if d.complete() && d.err != nil {
		d.err = errors.Compose(d.err, err)
		return
	} else if d.complete() && d.err == nil {
		d.log.Critical("download is marked as completed without error, but then managedFail was called with err:", err)
		return
	}

	// Mark the download as complete and set the error.
	d.err = err
	close(d.completeChan)
	err = d.destination.Close()
	if err != nil {
		d.log.Println("unable to close download destination:", err)
	}
}

// Err returns the error encountered by a download, if it exists.
func (d *download) Err() (err error) {
	d.mu.Lock()
	err = d.err
	d.mu.Unlock()
	return err
}

// newDownload creates and initializes a download based on the provided
// parameters.
func (r *Renter) newDownload(params downloadParams) (*download, error) {
	// Input validation.
	if params.file == nil {
		return nil, errors.New("no file provided when requesting download")
	}
	if params.length <= 0 {
		return nil, errors.New("download length must be a positive whole number")
	}
	if params.offset < 0 {
		return nil, errors.New("download offset cannot be a negative number")
	}
	if params.offset+params.length > params.file.size {
		return nil, errors.New("download is requesting data past the boundary of the file")
	}

	// Create the download object.
	d := &download{
		completeChan: make(chan struct{}),

		staticStartTime: time.Now(),

		destination:         params.destination,
		destinationString:   params.destinationString,
		staticLatencyTarget: params.latencyTarget,
		staticLength:        params.length,
		staticOffset:        params.offset,
		staticOverdrive:     params.overdrive,
		staticSiaPath:       params.file.name,
		staticPriority:      params.priority,

		log:           r.log,
		memoryManager: r.memoryManager,
	}

	// Determine which chunks to download.
	minChunk := params.offset / params.file.staticChunkSize()
	maxChunk := (params.offset + params.length - 1) / params.file.staticChunkSize()

	// For each chunk, assemble a mapping from the contract id to the index of
	// the piece within the chunk that the contract is responsible for.
	chunkMaps := make([]map[types.FileContractID]downloadPieceInfo, maxChunk-minChunk+1)
	for i := range chunkMaps {
		chunkMaps[i] = make(map[types.FileContractID]downloadPieceInfo)
	}
	params.file.mu.Lock()
	for id, contract := range params.file.contracts {
		resolvedID := r.hostContractor.ResolveID(id)
		for _, piece := range contract.Pieces {
			if piece.Chunk >= minChunk && piece.Chunk <= maxChunk {
				// Sanity check - the same worker should not have to pieces for
				// the same chunk.
				_, exists := chunkMaps[piece.Chunk-minChunk][resolvedID]
				if exists {
					r.log.Println("ERROR: Worker has multiple pieces uploaded for the same chunk.")
				}
				chunkMaps[piece.Chunk-minChunk][resolvedID] = downloadPieceInfo{
					index: piece.Piece,
					root:  piece.MerkleRoot,
				}
			}
		}
	}
	params.file.mu.Unlock()

	// Queue the downloads for each chunk.
	writeOffset := int64(0) // where to write a chunk within the download destination.
	for i := minChunk; i <= maxChunk; i++ {
		d.chunksRemaining++
		udc := &unfinishedDownloadChunk{
			destination: params.destination,
			erasureCode: params.file.erasureCode,
			masterKey:   params.file.masterKey,

			staticChunkIndex: i,
			staticChunkMap:   chunkMaps[i-minChunk],
			staticChunkSize:  params.file.staticChunkSize(),
			staticPieceSize:  params.file.pieceSize,

			// TODO: 25ms is just a guess for a good default. Really, we want to
			// set the latency target such that slower workers will pick up the
			// later chunks, but only if there's a very strong chance that
			// they'll finish before the earlier chunks finish, so that they do
			// no contribute to low latency.
			//
			// TODO: There is some sane minimum latency that should actually be
			// set based on the number of pieces 'n', and the 'n' fastest
			// workers that we have.
			staticLatencyTarget: params.latencyTarget + (25 * (i - minChunk)), // TODO: Latency target is dropped by 25ms for each following chunk.
			staticNeedsMemory:   params.needsMemory,
			staticPriority:      params.priority,

			physicalChunkData: make([][]byte, params.file.erasureCode.NumPieces()),
			pieceUsage:        make([]bool, params.file.erasureCode.NumPieces()),

			download: d,
		}

		// Set the fetchOffset - the offset within the chunk that we start
		// downloading from.
		if i == minChunk {
			udc.staticFetchOffset = params.offset % params.file.staticChunkSize()
		} else {
			udc.staticFetchOffset = 0
		}
		// Set the fetchLength - the number of bytes to fetch within the chunk
		// that we start downloading from.
		if i == maxChunk && (params.length+params.offset)%params.file.staticChunkSize() != 0 {
			udc.staticFetchLength = ((params.length + params.offset) % params.file.staticChunkSize()) - udc.staticFetchOffset
		} else {
			udc.staticFetchLength = params.file.staticChunkSize() - udc.staticFetchOffset
		}
		// Set the writeOffset within the destination for where the data should
		// be written.
		udc.staticWriteOffset = writeOffset
		writeOffset += int64(udc.staticFetchLength)

		// TODO: Pick a smarter value for the overdrive setting.
		if i < 2 {
			udc.staticOverdrive = params.overdrive
		}

		// Add this chunk to the chunk heap, and notify the download loop that
		// there is work to do.
		r.managedAddChunkToDownloadHeap(udc)
		select {
		case r.newDownloads <- struct{}{}:
		default:
		}
	}
	return d, nil
}

// Download performs a file download using the passed parameters.
func (r *Renter) Download(p modules.RenterDownloadParameters) error {
	// Lookup the file associated with the nickname.
	lockID := r.mu.RLock()
	file, exists := r.files[p.SiaPath]
	r.mu.RUnlock(lockID)
	if !exists {
		return fmt.Errorf("no file with that path: %s", p.SiaPath)
	}

	// Validate download parameters.
	isHTTPResp := p.Httpwriter != nil
	if p.Async && isHTTPResp {
		return errors.New("cannot async download to http response")
	}
	if isHTTPResp && p.Destination != "" {
		return errors.New("destination cannot be specified when downloading to http response")
	}
	if !isHTTPResp && p.Destination == "" {
		return errors.New("destination not supplied")
	}
	if p.Destination != "" && !filepath.IsAbs(p.Destination) {
		return errors.New("destination must be an absolute path")
	}
	if p.Offset == file.size {
		return errors.New("offset equals filesize")
	}
	// Sentinel: if length == 0, download the entire file.
	if p.Length == 0 {
		p.Length = file.size - p.Offset
	}
	// Check whether offset and length is valid.
	if p.Offset < 0 || p.Offset+p.Length > file.size {
		return fmt.Errorf("offset and length combination invalid, max byte is at index %d", file.size-1)
	}

	// Instantiate the correct downloadWriter implementation.
	var dw downloadDestination
	var destinationType string
	if isHTTPResp {
		dw = newDownloadDestinationHTTPWriter(p.Httpwriter)
		destinationType = "http stream"
	} else {
		osFile, err := os.OpenFile(p.Destination, os.O_CREATE|os.O_WRONLY, defaultFilePerm)
		if err != nil {
			return err
		}
		dw = osFile
		destinationType = "file"
	}

	// Create the download object.
	d, err := r.newDownload(downloadParams{
		destination:       dw,
		destinationType:   destinationType,
		destinationString: p.Destination,
		file:              file,

		latencyTarget: 25e3, // TODO: high default until full latency support is added.
		length:        p.Length,
		needsMemory:   true,
		offset:        p.Offset,
		overdrive:     2, // TODO: moderate default until full overdrive support is added.
		priority:      5, // TODO: moderate default until full priority support is added.
	})
	if err != nil {
		return err
	}

	// Add the download object to the download queue.
	lockID = r.mu.Lock()
	r.downloadQueue = append(r.downloadQueue, d)
	r.mu.Unlock(lockID)

	// Block until the download has completed.
	select {
	case <-d.completeChan:
		return d.Err()
	case <-r.tg.StopChan():
		return errors.New("download interrupted by shutdown")
	}
}

// DownloadQueue returns the list of downloads in the queue.
func (r *Renter) DownloadQueue() []modules.DownloadInfo {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	downloads := make([]modules.DownloadInfo, len(r.downloadQueue))
	for i := range r.downloadQueue {
		// Order from most recent to least recent.
		d := r.downloadQueue[len(r.downloadQueue)-i-1]
		d.mu.Lock() // Lock required for d.endTime only.
		downloads[i] = modules.DownloadInfo{
			Destination:     d.destinationString,
			DestinationType: d.destinationType,
			Length:          d.staticLength,
			Offset:          d.staticOffset,
			SiaPath:         d.staticSiaPath,

			EndTime:             d.endTime,
			Received:            atomic.LoadUint64(&d.atomicDataReceived),
			StartTime:           d.staticStartTime,
			TotalDataTransfered: atomic.LoadUint64(&d.atomicTotalDataTransfered),
		}
		d.mu.Unlock() // Release lock before calling d.Err().
		// Need to listen on the completed channel to know if the download has
		// completed.
		select {
		case <-d.completeChan:
			downloads[i].Completed = true
		default:
			downloads[i].Completed = false
		}
		// Need to check if the error is nil before setting the error string.
		if d.Err() != nil {
			downloads[i].Error = d.Err().Error()
		} else {
			downloads[i].Error = ""
		}
	}
	return downloads
}
