package renter

import (
	"errors"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	defaultDuration     = 6000 // Duration that hosts will hold onto the file
	defaultDataPieces   = 2    // Data pieces per erasure-coded chunk
	defaultParityPieces = 8    // Parity pieces per erasure-coded chunk

	// piece sizes
	// NOTE: The encryption overhead is subtracted so that encrypted piece
	// will always be a multiple of 64 (i.e. crypto.SegmentSize). Without this
	// property, revisions break the file's Merkle root.
	defaultPieceSize = 1<<22 - crypto.TwofishOverhead // 4 MiB
	smallPieceSize   = 1<<16 - crypto.TwofishOverhead // 64 KiB
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

	averagePrice := r.hostDB.AveragePrice()
	estimatedCost := averagePrice.Mul(types.NewCurrency64(uint64(up.Duration))).Mul(curSize)
	bufferedCost := estimatedCost.Mul(types.NewCurrency64(2))

	siacoinBalance, _, _ := r.wallet.ConfirmedBalance()
	if bufferedCost.Cmp(siacoinBalance) > 0 {
		return errors.New("insufficient balance for upload")
	}
	return nil
}

// Upload instructs the renter to start tracking a file. The renter will
// automatically upload and repair tracked files using a background loop.
func (r *Renter) Upload(up modules.FileUploadParams) error {
	// Check for a nickname conflict.
	lockID := r.mu.RLock()
	_, exists := r.files[up.Nickname]
	r.mu.RUnlock(lockID)
	if exists {
		return errors.New("file with that nickname already exists")
	}

	// Fill in any missing upload params with sensible defaults.
	fileInfo, err := os.Stat(up.Filename)
	if err != nil {
		return err
	}
	if up.Duration == 0 {
		up.Duration = defaultDuration
	}
	endHeight := r.cs.Height() + up.Duration
	if up.ErasureCode == nil {
		up.ErasureCode, _ = NewRSCode(defaultDataPieces, defaultParityPieces)
	}
	if up.PieceSize == 0 {
		if fileInfo.Size() > defaultPieceSize {
			up.PieceSize = defaultPieceSize
		} else {
			up.PieceSize = smallPieceSize
		}
	}

	// Check that we have enough money to finance the upload.
	err = r.checkWalletBalance(up)
	if err != nil {
		return err
	}

	// Create file object.
	f := newFile(up.Nickname, up.ErasureCode, up.PieceSize, uint64(fileInfo.Size()))
	f.mode = uint32(fileInfo.Mode())

	// Add file to renter.
	lockID = r.mu.Lock()
	r.files[up.Nickname] = f
	r.tracking[up.Nickname] = trackedFile{
		RepairPath: up.Filename,
		EndHeight:  endHeight,
		Renew:      up.Renew,
	}
	r.save()
	r.mu.Unlock(lockID)

	// Save the .sia file to the renter directory.
	err = r.saveFile(f)
	if err != nil {
		return err
	}

	return nil
}
