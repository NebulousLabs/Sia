package renter

import (
	"crypto/rand"
	"errors"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	maxUploadAttempts = 8
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

	// TODO: Change average to median so that outliers are ignored.
	sampleSize := 12
	var averagePrice types.Currency
	for i := 0; i < sampleSize; i++ {
		potentialHost, err := r.hostDB.RandomHost()
		if err != nil {
			return err
		}
		averagePrice = averagePrice.Add(potentialHost.Price)
	}
	averagePrice = averagePrice.Div(types.NewCurrency64(uint64(sampleSize)))
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
func (r *Renter) threadedUploadPiece(up modules.FileUploadParams, piece *filePiece) {
	// Set 'Repairing' for the piece to true.
	lockID := r.mu.Lock()
	piece.Repairing = true
	r.mu.Unlock(lockID)

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

		lockID := r.mu.Lock()
		*piece = filePiece{
			Active:     true,
			Repairing:  false,
			Contract:   contract,
			ContractID: contractID,
			HostIP:     host.IPAddress,
		}
		r.save()
		r.mu.Unlock(lockID)
		return
	}
}

// Upload takes an upload parameters, which contain a file to upload, and then
// creates a redundant copy of the file on the Sia network.
func (r *Renter) Upload(up modules.FileUploadParams) error {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	err := r.checkWalletBalance(up)
	if err != nil {
		return err
	}

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
	r.files[up.Nickname] = &file{
		Name:         up.Nickname,
		Pieces:       make([]filePiece, up.Pieces),
		UploadParams: up,
		renter:       r,
	}
	for i := range r.files[up.Nickname].Pieces {
		// threadedUploadPiece will change the memory that the piece points to,
		// which is useful because it means the file itself can be renamed but
		// will still point to the same underlying pieces.
		go r.threadedUploadPiece(up, &r.files[up.Nickname].Pieces[i])
	}
	r.save()

	return nil
}
