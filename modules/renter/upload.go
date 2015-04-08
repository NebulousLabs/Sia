package renter

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

const (
	maxUploadAttempts = 8
)

// threadedUploadPiece will upload the piece of a file to a randomly chosen
// host. If the wallet has insufficient balance to support uploading,
// uploadPiece will give up. The file uploading can be continued using a repair
// tool. Upon completion, the memory containg the piece's information is
// updated.
func (r *Renter) threadedUploadPiece(up modules.UploadParams, piece *FilePiece) {
	// Set 'Repairing' for the piece to true.
	r.mu.Lock()
	piece.Repairing = true
	r.mu.Unlock()

	// Try 'maxUploadAttempts' hosts before giving up.
	for attempts := 0; attempts < maxUploadAttempts; attempts++ {
		// Select a host. An error here is unrecoverable.
		host, err := r.hostDB.RandomHost()
		if err != nil {
			return
		}

		// Negotiate the contract with the host. If the negotiation is
		// unsuccessful, we need to try again with a new host. Otherwise, the
		// file will be uploaded and we'll be done.
		contract, contractID, err := r.negotiateContract(host, up)
		if err != nil {
			// The previous attempt didn't work. We will try again after
			// sleeping for a randomized amount of time to increase our chances
			// of success. This will help spread things out if there are
			// problems with network congestion or other randomized issues.
			randSource := make([]byte, 1)
			rand.Read(randSource)
			time.Sleep(time.Duration(attempts) * time.Duration(attempts) * 250 * time.Millisecond * time.Duration(randSource[0]))
			continue
		}

		r.mu.Lock()
		*piece = FilePiece{
			Active:     true,
			Repairing:  false,
			Contract:   contract,
			ContractID: contractID,
			HostIP:     host.IPAddress,
		}
		r.save()
		r.mu.Unlock()
		return
	}
}

// Upload takes an upload parameters, which contain a file to upload, and then
// creates a redundant copy of the file on the Sia network.
func (r *Renter) Upload(up modules.UploadParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for a nickname conflict.
	_, exists := r.files[up.Nickname]
	if exists {
		return errors.New("file with that nickname already exists")
	}

	// Check that the hostdb is sufficiently large to support an upload. Right
	// now that value is set to 3, but in the future the logic will be a bit
	// more complex; once there is erasure coding we'll want to hit the minimum
	// number of pieces plus some buffer before we decide that an upload is
	// okay.
	if len(r.hostDB.ActiveHosts()) < 1 {
		return errors.New("not enough hosts on the network to upload a file :( - maybe you need to upgrade your software")
	}

	// Upload a piece to every host on the network.
	r.files[up.Nickname] = File{
		nickname:    up.Nickname,
		pieces:      make([]FilePiece, up.Pieces),
		startHeight: r.state.Height() + up.Duration,
		renter:      r,
	}
	for i := range r.files[up.Nickname].pieces {
		// threadedUploadPiece will change the memory that the piece points to,
		// which is useful because it means the file itself can be renamed but
		// will still point to the same underlying pieces.
		go r.threadedUploadPiece(up, &r.files[up.Nickname].pieces[i])
	}
	r.save()

	return nil
}
