package renter

import (
	"errors"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Download downloads a file, identified by its path, to the destination
// specified.
func (r *Renter) Download(path, destination string) chan error {
	// Lookup the file associated with the nickname.
	lockID := r.mu.RLock()
	file, exists := r.files[path]
	r.mu.RUnlock(lockID)

	errchan := make(chan error, 1)

	if !exists {
		errchan <- errors.New("no file with that path")
		return errchan
	}

	// Build current contracts map.
	currentContracts := make(map[modules.NetAddress]types.FileContractID)
	for _, contract := range r.hostContractor.Contracts() {
		currentContracts[contract.NetAddress] = contract.ID
	}

	// Create the download object and add it to the queue.
	d := r.newDownload(file, destination, currentContracts)
	lockID = r.mu.Lock()
	r.downloadQueue = append(r.downloadQueue, d)
	r.mu.Unlock(lockID)
	r.newDownloads <- d

	go func() {
		errchan <- <-d.downloadFinished
	}()

	return errchan
}

// DownloadQueue returns the list of downloads in the queue.
func (r *Renter) DownloadQueue() []modules.DownloadInfo {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// order from most recent to least recent
	downloads := make([]modules.DownloadInfo, len(r.downloadQueue))
	for i := range r.downloadQueue {
		d := r.downloadQueue[len(r.downloadQueue)-i-1]

		downloads[i] = modules.DownloadInfo{
			SiaPath:     d.siapath,
			Destination: d.destination,
			Filesize:    d.fileSize,
			StartTime:   d.startTime,
		}
		downloads[i].Received = atomic.LoadUint64(&d.atomicDataReceived)

		// set the DownloadInfo's Error field if an error has occured.
		select {
		case err := <-d.downloadFinished:
			if err != nil {
				downloads[i].Error = err.Error()
			}
		default:
		}
	}
	return downloads
}
