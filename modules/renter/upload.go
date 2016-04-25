package renter

import (
	"errors"
	"os"
	"strings"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	errInsufficientContracts = errors.New("not enough contracts to upload file")

	// Erasure-coded piece size
	pieceSize = modules.SectorSize - crypto.TwofishOverhead

	// defaultDataPieces is the number of data pieces per erasure-coded chunk
	defaultDataPieces = func() int {
		if build.Release == "testing" {
			return 2
		}
		return 1 // RC; orig value: 4
	}()

	// defaultParityPieces is the number of parity pieces per erasure-coded
	// chunk
	defaultParityPieces = func() int {
		if build.Release == "testing" {
			return 8
		}
		return 6 // RC; orig value: 20
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
	if up.ErasureCode == nil {
		up.ErasureCode, _ = NewRSCode(defaultDataPieces, defaultParityPieces)
	}

	// Check that we have contracts to upload to.
	if len(r.hostContractor.Contracts()) < up.ErasureCode.NumPieces() && build.Release != "testing" {
		return errInsufficientContracts
	}

	// Create file object.
	f := newFile(up.SiaPath, up.ErasureCode, pieceSize, uint64(fileInfo.Size()))
	f.mode = uint32(fileInfo.Mode())

	// Add file to renter.
	lockID = r.mu.Lock()
	r.files[up.SiaPath] = f
	r.tracking[up.SiaPath] = trackedFile{
		RepairPath: up.Source,
	}
	r.save(true)
	r.mu.Unlock(lockID)

	// Save the .sia file to the renter directory.
	err = r.saveFile(f)
	if err != nil {
		return err
	}

	return nil
}
