package renter

import (
	"errors"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

func (r *Renter) DownloadSection(p *modules.RenterDownloadParameters) error {
	// Lookup the file associated with the nickname.
	lockID := r.mu.RLock()
	file, exists := r.files[p.Siapath]
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
	var d *download
	if p.Offset == maxUint64 {
		d = r.newDownload(file, p.DlWriter, currentContracts)
	} else {
		// Check whether the chunk index is valid.
		numChunks := file.numChunks()
		if p.Offset < 0 && p.Offset >= numChunks {
			emsg := "chunk index not in range of stored chunks. Max chunk index = " + string(numChunks-1)
			return errors.New(emsg)
		}
		d = r.newSectionDownload(file, p.DlWriter, currentContracts, p.Offset, p.Length)
	}

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

		// Calculate download size. If single chunk this value equals d.chunkSize, otherwise it is equal to
		// d.FileSize. TODO: Account for variable-size last chunk. Same in download.go
		dlsize := d.dlChunks * d.chunkSize

		downloads[i] = modules.DownloadInfo{
			SiaPath:     d.siapath,
			Destination: d.destination,
			Filesize:    dlsize,
			StartTime:   d.startTime,
		}
		downloads[i].Received = atomic.LoadUint64(&d.atomicDataReceived)

		if err := d.Err(); err != nil {
			downloads[i].Error = err.Error()
		}
	}
	return downloads
}
