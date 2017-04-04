package renter

import (
	"errors"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Download downloads a file, identified by its path, to the destination
// specified.
func (r *Renter) Download(path, destination string) error {
	// Lookup the file associated with the nickname.
	lockID := r.mu.RLock()
	file, exists := r.files[path]
	r.mu.RUnlock(lockID)
	if !exists {
		return errors.New("no file with that path")
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

	// Block until the download has completed.
	//
	// TODO: Eventually just return the channel to the error instead of the
	// error itself.
	select {
	case <-d.downloadFinished:
		return d.Err()
	case <-r.tg.StopChan():
		return errors.New("download interrupted by shutdown")
	}
}

// DownloadQueue returns the list of downloads in the queue.
func (r *Renter) DownloadQueue() []modules.DownloadInfo {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	// Order from most recent to least recent.
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

		if err := d.Err(); err != nil {
			downloads[i].Error = err.Error()
		}
	}
	return downloads
}
