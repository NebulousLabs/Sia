package renter

// scanAllFiles checks all files for pieces that are not yet active and then
// uploads them to the network.
func (r *Renter) scanAllFiles() {
	for _, file := range r.files {
		for i := range file.Pieces {
			if !file.Pieces[i].Active && !file.Pieces[i].Repairing {
				hosts := r.hostDB.RandomHosts(1)
				if len(hosts) == 1 {
					go r.threadedUploadPiece(hosts[0], file.UploadParams, &file.Pieces[i])
				}
			}
		}
	}
}
