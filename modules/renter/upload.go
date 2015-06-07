package renter

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	maxUploadAttempts = 3
	parallelUploads   = 3
)

var (
	errUploadFailed = errors.New("failed to upload to the desired host")

	redundancy = 15
)

// checkWalletBalance looks at an upload and determines if there is enough
// money in the wallet to support such an upload. An error is returned if it is
// determined that there is not enough money.
func (r *Renter) checkWalletBalance(up modules.FileUploadParams) error {
	// Get the size of the file.
	fileInfo, err := os.Stat(up.Filename)
	if err != nil {
		return err
	}
	curSize := types.NewCurrency64(uint64(fileInfo.Size()))

	var averagePrice types.Currency
	sampleSize := redundancy * 2
	hosts := r.hostDB.RandomHosts(sampleSize)
	for _, host := range hosts {
		averagePrice = averagePrice.Add(host.Price)
	}
	averagePrice = averagePrice.Div(types.NewCurrency64(uint64(len(hosts))))
	estimatedCost := averagePrice.Mul(types.NewCurrency64(uint64(up.Duration))).Mul(curSize)
	bufferedCost := estimatedCost.Mul(types.NewCurrency64(2))

	if bufferedCost.Cmp(r.wallet.Balance(false)) > 0 {
		return errors.New("insufficient balance for upload")
	}
	return nil
}

// threadedUploadPiece will upload the piece of a file to a randomly chosen
// host. If the wallet has insufficient balance to support uploading,
// uploadPiece will give up. The file uploading can be continued using a repair
// tool. Upon completion, the memory containg the piece's information is
// updated.
func (r *Renter) threadedUploadPiece(host modules.HostSettings, up modules.FileUploadParams, piece *filePiece) error {
	// Set 'Repairing' for the piece to true.
	lockID := r.mu.Lock()
	piece.Repairing = true
	r.mu.Unlock(lockID)

	// Try 'maxUploadAttempts' hosts before giving up.
	for attempts := 0; attempts < maxUploadAttempts; attempts++ {
		// Negotiate the contract with the host. If the negotiation is
		// unsuccessful, we need to try again with a new host.
		err := r.negotiateContract(host, up, piece)
		if err == nil {
			return nil
		}

		// The previous attempt didn't work. We will try again after
		// sleeping for a randomized amount of time to increase our chances
		// of success. This will help spread things out if there are
		// problems with network congestion or other randomized issues.
		randSource := make([]byte, 1)
		rand.Read(randSource)
		time.Sleep(100 * time.Millisecond * time.Duration(randSource[0]))
	}

	// All attempts failed.
	return errors.New("failed to upload filePiece")
}

// Upload takes an upload parameters, which contain a file to upload, and then
// creates a redundant copy of the file on the Sia network.
func (r *Renter) Upload(up modules.FileUploadParams) error {
	// TODO: This type of restriction is something that should be handled by
	// the frontend, not the backend.
	if filepath.Ext(up.Filename) != filepath.Ext(up.Nickname) {
		return errors.New("nickname and file name must have the same extension")
	}

	err := r.checkWalletBalance(up)
	if err != nil {
		return err
	}

	// Check for a nickname conflict.
	lockID := r.mu.RLock()
	_, exists := r.files[up.Nickname]
	r.mu.RUnlock(lockID)
	if exists {
		return errors.New("file with that nickname already exists")
	}

	// Check that the file exists and is less than 500 MiB.
	fileInfo, err := os.Stat(up.Filename)
	if err != nil {
		return err
	}
	// NOTE: The upload max of 500 MiB is temporary and therefore does not have a
	// constant. This should be removed once micropayments + upload resuming
	// are in place. 500 MiB is chosen to prevent confusion - on anybody's
	// machine any file appearing to be under 500 MB will be below the hard
	// limit.
	if fileInfo.Size() > 500*1024*1024 {
		return errors.New("cannot upload a file larger than 500 MB")
	}

	// Check that the hostdb is sufficiently large to support an upload. Right
	// now that value is set to 3, but in the future the logic will be a bit
	// more complex; once there is erasure coding we'll want to hit the minimum
	// number of pieces plus some buffer before we decide that an upload is
	// okay.
	if len(r.hostDB.ActiveHosts()) < 1 {
		return errors.New("not enough hosts on the network to upload a file")
	}

	// Create file object.
	f := &file{
		Name: up.Nickname,

		PiecesRequired: 1,
		Pieces:         make([]filePiece, up.Pieces),
		UploadParams:   up,
		renter:         r,
	}
	for i := range f.Pieces {
		f.Pieces[i].Repairing = true
		f.Pieces[i].PieceSize = uint64(fileInfo.Size())
	}

	// Add file to renter.
	lockID = r.mu.Lock()
	r.files[up.Nickname] = f
	r.save()
	r.mu.Unlock(lockID)

	// Upload to hosts in parallel. To facilitate this, we create channels of
	// hosts and file pieces, and spawn goroutines that attempt to match each
	// piece to a host.
	hostPool := make(chan modules.HostSettings, 3*redundancy)
	for _, host := range r.hostDB.RandomHosts(3 * redundancy) {
		hostPool <- host
	}
	piecePool := make(chan *filePiece, len(f.Pieces))
	for i := range f.Pieces {
		piecePool <- &f.Pieces[i]
	}
	errChan := make(chan error, len(f.Pieces))
	for i := 0; i < parallelUploads; i++ {
		go func() {
			for piece := range piecePool {
				var err error
				for host := range hostPool {
					err = r.threadedUploadPiece(host, up, piece)
					if err == nil {
						break
					}
				}
				errChan <- err
			}
		}()
	}

	// Wait for success or failure. Since we are (currently) using full
	// replication, success means "one piece was uploaded," while failure
	// means "zero pieces were uploaded."
	reqPieces := f.PiecesRequired
	for i := 0; i < up.Pieces; i++ {
		if <-errChan == nil {
			reqPieces--
			if reqPieces <= 0 {
				return nil
			}
		}
	}

	// All uploads failed. Remove the file object.
	lockID = r.mu.Lock()
	delete(r.files, up.Nickname)
	r.save()
	r.mu.Unlock(lockID)

	return errors.New("failed to upload any file pieces")
}
