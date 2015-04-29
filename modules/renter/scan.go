package renter

// scanAllFiles checks all files for pieces that are not yet active and then
// uploads them to the network.
func (r *Renter) scanAllFiles() {
	for _, file := range r.files {
		for i := range file.Pieces {
			if !file.Pieces[i].Active && !file.Pieces[i].Repairing {
				go r.threadedUploadPiece(file.UploadParams, &file.Pieces[i])
			}
		}
	}
}
