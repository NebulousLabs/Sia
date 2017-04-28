package renter

import (
	"errors"
	"sync/atomic"

	"fmt"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// DownloadSection performs a file download according to the download parameters passed.
func (r *Renter) DownloadSection(p *modules.RenterDownloadParameters) error {
	// Lookup the file associated with the nickname.
	lockID := r.mu.RLock()
	file, exists := r.files[p.Siapath]
	r.mu.RUnlock(lockID)
	if !exists {
		return errors.New(fmt.Sprintf("no file with that path: %s", p.Siapath))
	}

	// Build current contracts map.
	currentContracts := make(map[modules.NetAddress]types.FileContractID)
	for _, contract := range r.hostContractor.Contracts() {
		currentContracts[contract.NetAddress] = contract.ID
	}

	// Ensure that both offset and length were passed or neither.
	if (p.OffsetPassed || p.LengthPassed) && !(p.OffsetPassed && p.LengthPassed) {
		var missingfield = "offset"
		if p.LengthPassed {
			missingfield = "length"
		}
		return errors.New("either both \"offset\" and " +
			"\"length\" have to be specified or neither. " +
			missingfield + " has not been specified.")
	}

	// Determine if entire file is to be downloaded.
	if !p.OffsetPassed {
		p.Offset = 0
		p.Length = file.size
	}

	// Check whether offset and length is valid.
	if p.Offset < 0 || p.Offset+p.Length > file.size {
		emsg := fmt.Sprintf("offset and length combination invalid, max byte is at index %d", file.size-1)
		return errors.New(emsg)
	}
	if p.Length == 0 {
		return errors.New("the length parameter has to be greater than 0.")
	}

	// Create the download object and add it to the queue.
	d := r.newSectionDownload(file, p.DlWriter, currentContracts, p.Offset, p.Length)

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
			Filesize:    d.length,
			StartTime:   d.startTime,
		}
		downloads[i].Received = atomic.LoadUint64(&d.atomicDataReceived)

		if err := d.Err(); err != nil {
			downloads[i].Error = err.Error()
		}
	}
	return downloads
}
