package renter

// scanAllFiles checks all files for pieces that are not yet active and then
// uploads them to the network.
func (r *Renter) scanAllFiles() {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	for _, file := range r.files {
		for i := range file.pieces {
			if !file.pieces[i].Active && !file.pieces[i].Repairing {
				go r.threadedUploadPiece(file.uploadParams, &file.pieces[i])
			}
		}
	}
}
