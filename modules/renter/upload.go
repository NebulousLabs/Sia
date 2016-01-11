package renter

import (
	"errors"
	"os"
	"strings"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// piece sizes
	// NOTE: The encryption overhead is subtracted so that encrypted piece
	// will always be a multiple of 64 (i.e. crypto.SegmentSize). Without this
	// property, revisions break the file's Merkle root.
	defaultPieceSize = 1<<22 - crypto.TwofishOverhead // 4 MiB
	smallPieceSize   = 1<<16 - crypto.TwofishOverhead // 64 KiB
)

var (
	// defaultDuration is the contract length that the renter will use when the
	// uploader does not specify a duration.
	defaultDuration = func() types.BlockHeight {
		switch build.Release {
		case "testing":
			return 20
		case "dev":
			return 200
		default:
			return 504 // 3.5 days - RC ONLY!
		}
	}()

	// defaultDataPieces is the number of data pieces per erasure-coded chunk
	defaultDataPieces = func() int {
		if build.Release == "testing" {
			return 2
		}
		return 4
	}()

	// defaultParityPieces is the number of parity pieces per erasure-coded
	// chunk
	defaultParityPieces = func() int {
		if build.Release == "testing" {
			return 8
		}
		return 20
	}()
)

// checkWalletBalance looks at an upload and determines if there is enough
// money in the wallet to support such an upload. An error is returned if it is
// determined that there is not enough money.
func (r *Renter) checkWalletBalance(up modules.FileUploadParams) error {
	if !r.wallet.Unlocked() {
		return errors.New("wallet is locked")
	}
	// Get the size of the file.
	fileInfo, err := os.Stat(up.Source)
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
	// Enforce nickname rules.
	if strings.HasPrefix(up.SiaPath, "/") {
		return errors.New("nicknames cannot begin with /")
	}

	// Check for a nickname conflict.
	lockID := r.mu.RLock()
	_, exists := r.files[up.SiaPath]
	r.mu.RUnlock(lockID)
	if exists {
		return ErrPathOverload
	}

	// Fill in any missing upload params with sensible defaults.
	fileInfo, err := os.Stat(up.Source)
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
	f := newFile(up.SiaPath, up.ErasureCode, up.PieceSize, uint64(fileInfo.Size()))
	f.mode = uint32(fileInfo.Mode())

	// Add file to renter.
	lockID = r.mu.Lock()
	r.files[up.SiaPath] = f
	r.tracking[up.SiaPath] = trackedFile{
		RepairPath: up.Source,
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
