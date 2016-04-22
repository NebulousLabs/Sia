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

var (
	// Erasure-coded piece size
	pieceSize = modules.SectorSize - crypto.TwofishOverhead

	// defaultDuration is the contract length that the renter will use when the
	// uploader does not specify a duration.
	defaultDuration = func() types.BlockHeight {
		switch build.Release {
		case "testing":
			return 20
		case "dev":
			return 200
		default:
			return 144 * 60 // 60 days - to soon be 6 months.
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

	// Create file object.
	f := newFile(up.SiaPath, up.ErasureCode, pieceSize, uint64(fileInfo.Size()))
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
